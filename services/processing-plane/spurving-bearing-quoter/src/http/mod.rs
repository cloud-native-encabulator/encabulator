//! The HTTP surface. One port serves the API, health, and metrics (design section 7).

pub mod handlers;
pub mod observe;

use axum::Router;
use axum::routing::{get, post};

use crate::app::AppState;

/// Builds the router.
///
/// Two sub-routers, merged:
///
/// - the ops router (`/healthz`, `/readyz`, `/metrics`) carries only the request-id middleware, so
///   probes and scrapes are never traced, never metered, and never subject to injected malaise;
/// - the API router carries the full observe middleware and the malaise middleware.
pub fn router(state: AppState) -> Router {
    let ops = Router::new()
        .route("/healthz", get(handlers::healthz))
        .route("/readyz", get(handlers::readyz))
        .route("/metrics", get(handlers::metrics))
        .layer(axum::middleware::from_fn(observe::request_id_only))
        .with_state(state.clone());

    let api = Router::new()
        .route("/v1/quotes", post(handlers::create_quote))
        .route("/v1/quotes/{id}", get(handlers::get_quote))
        .method_not_allowed_fallback(handlers::method_not_allowed)
        .fallback(handlers::not_found)
        // Innermost: malaise sits closest to the handler, so injected latency and errors are
        // inside the server span and inside the latency histogram, exactly as a real fault would be.
        .layer(axum::middleware::from_fn_with_state(
            state.malaise.clone(),
            crate::malaise::middleware,
        ))
        .layer(axum::middleware::from_fn_with_state(
            state.clone(),
            observe::observe,
        ))
        .with_state(state);

    ops.merge(api)
}
