//! Handlers for the quoter's API, health, and metrics routes (design section 8.3).

use std::time::Instant;

use axum::Json;
use axum::extract::{Path, State};
use axum::response::{IntoResponse, Response};
use chrono::Utc;
use http::{StatusCode, header};
use serde::Serialize;

use crate::app::AppState;
use crate::cache;
use crate::error::ApiError;
use crate::http::observe::X_ENCABULATOR_SUBJECT;
use crate::metrics::Metrics;
use crate::model::{Currency, Model, Quote, QuoteRequest, new_quote_id, rfc3339_millis};
use crate::pricing::{MAX_QUANTITY, MIN_QUANTITY, PricingInputs, price};

#[derive(Serialize)]
pub struct Health {
    pub status: &'static str,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Readiness {
    pub status: &'static str,
    /// Informational. Valkey is a soft dependency and never gates readiness (design section 8.3),
    /// so this reports what the quoter knows without acting on it.
    pub quote_cache: crate::cache::CacheState,
    pub flags: &'static str,
}

/// `GET /healthz`: liveness. 200 as long as the process is running.
pub async fn healthz() -> Json<Health> {
    Json(Health { status: "ok" })
}

/// `GET /readyz`: readiness.
///
/// The quoter has no hard dependencies: Valkey is soft and the girdlespring has built-in defaults,
/// so nothing downstream can make this fail. It goes red only while draining after SIGTERM.
pub async fn readyz(State(state): State<AppState>) -> Response {
    let body = Readiness {
        status: if state.draining() { "draining" } else { "ok" },
        quote_cache: state.store.state(),
        flags: if state.flags.enabled() {
            if state.flags.healthy() {
                "live"
            } else {
                "defaults"
            }
        } else {
            "disabled"
        },
    };
    let status = if state.draining() {
        StatusCode::SERVICE_UNAVAILABLE
    } else {
        StatusCode::OK
    };
    (status, Json(body)).into_response()
}

/// `GET /metrics`: Prometheus text format.
pub async fn metrics(State(state): State<AppState>) -> Response {
    (
        [(header::CONTENT_TYPE, Metrics::content_type())],
        state.metrics.encode(),
    )
        .into_response()
}

/// `POST /v1/quotes`.
///
/// Returns 200 rather than 201: a quote is a computation the service happens to remember for an
/// hour, and a cache hit returning 201 Created would be a lie. The design does not fix the status.
pub async fn create_quote(
    State(state): State<AppState>,
    headers: http::HeaderMap,
    body: Result<Json<QuoteRequest>, axum::extract::rejection::JsonRejection>,
) -> Result<Response, ApiError> {
    let Json(request) = body?;

    let model =
        Model::from_id(&request.model).ok_or_else(|| ApiError::unknown_model(&request.model))?;
    if !(MIN_QUANTITY..=MAX_QUANTITY).contains(&request.quantity) {
        return Err(ApiError::invalid_quantity(request.quantity));
    }

    // Flags are evaluated per subject so a percentage rollout of turbo-encabulation behaves the
    // same here as it does in the casing line.
    let subject = headers
        .get(&X_ENCABULATOR_SUBJECT)
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty());
    let flags = state.flags.snapshot();
    let turbo = flags.turbo_encabulation(subject);
    let cache_enabled = flags.quote_cache(subject);

    let inputs = PricingInputs {
        model,
        quantity: request.quantity,
        options: request.options,
        turbo,
    };

    if cache_enabled && let Some(mut cached) = state.store.get_cached(&inputs).await {
        cached.cached = true;
        state.metrics.record_quote(model, true);
        tracing::debug!(
            quote_id = %cached.id,
            model = model.id(),
            quantity = request.quantity,
            "served a cached quote"
        );
        return Ok(Json(cached).into_response());
    }

    let started = Instant::now();
    let priced = price(inputs);
    // The pentametric harmonic: deliberate CPU, on a bounded blocking pool so the runtime keeps
    // answering /healthz while it burns.
    let harmonic = state
        .pentametric
        .compute(model, request.quantity)
        .await
        .ok_or_else(|| ApiError::internal("The pentametric harmonic failed to converge."))?;
    let compute_seconds = started.elapsed().as_secs_f64();
    state.metrics.quote_compute_seconds.observe(compute_seconds);

    let quote = Quote {
        id: new_quote_id(),
        model,
        quantity: request.quantity,
        options: request.options,
        unit_price: priced.unit_price,
        subtotal: priced.subtotal,
        surcharges: priced.surcharges,
        total: priced.total,
        currency: Currency::Usd,
        computed_at: rfc3339_millis(Utc::now()),
        cached: false,
    };

    // Always store by id so GET /v1/quotes/{id} works; only fill the input cache when the
    // quote-cache flag is on.
    let cache_key = cache_enabled.then(|| cache::input_key(&inputs));
    state.store.store(&quote, cache_key.as_deref()).await;

    state.metrics.record_quote(model, false);
    tracing::info!(
        quote_id = %quote.id,
        model = model.id(),
        quantity = request.quantity,
        dingle_arm = request.options.dingle_arm,
        turbo,
        total = %quote.total,
        harmonic = harmonic.value,
        harmonic_terms = harmonic.terms,
        compute_ms = compute_seconds * 1_000.0,
        "priced a quote"
    );

    Ok(Json(quote).into_response())
}

/// `GET /v1/quotes/{id}`.
///
/// 404 when the quote is not on file or the cache is disabled entirely; 503 when Valkey is
/// configured but unreachable, because answering "no such quote" while the store is down would be
/// a lie the processor could act on.
pub async fn get_quote(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Response, ApiError> {
    if !state.store.enabled() {
        return Err(ApiError::quote_not_found(&id));
    }
    match state.store.get_by_id(&id).await {
        Ok(Some(mut quote)) => {
            quote.cached = true;
            Ok(Json(quote).into_response())
        }
        Ok(None) => Err(ApiError::quote_not_found(&id)),
        Err(err) => {
            tracing::warn!(error = %err, quote_id = %id, "could not read a quote from Valkey");
            Err(ApiError::dependency_unavailable("Valkey"))
        }
    }
}

/// Fallback for unmatched paths.
pub async fn not_found() -> ApiError {
    ApiError::not_found()
}

/// Fallback for a known path with an unsupported method, so a 405 carries the standard error shape.
pub async fn method_not_allowed() -> ApiError {
    ApiError::method_not_allowed()
}
