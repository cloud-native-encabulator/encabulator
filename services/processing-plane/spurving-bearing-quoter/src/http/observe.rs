//! Request ids, server spans, HTTP server metrics, and the access log.
//!
//! Design section 7:
//!
//! > `X-Request-Id`: accept or generate; echo in the response; include in logs.
//! > `http_server_request_duration_seconds` histogram, labels `http_request_method`, `http_route`
//! > (the route template, never the raw path), `http_response_status_code`.
//! > Instrument inbound HTTP (server spans) [...] Do not trace `/healthz`, `/readyz`, `/metrics`.
//!
//! The health and metrics routes get [`request_id_only`]; everything else gets [`observe`]. That
//! split is how the "do not trace health" rule is enforced: those routes never enter a span at all,
//! so there is nothing for the OpenTelemetry layer to export.

use std::time::Instant;

use axum::extract::{MatchedPath, Request, State};
use axum::middleware::Next;
use axum::response::Response;
use http::{HeaderName, HeaderValue};
use tracing::Instrument;
use tracing_opentelemetry::OpenTelemetrySpanExt;

use crate::app::AppState;

pub const X_REQUEST_ID: HeaderName = HeaderName::from_static("x-request-id");
pub const X_ENCABULATOR_SUBJECT: HeaderName = HeaderName::from_static("x-encabulator-subject");
pub const X_ENCABULATOR_ROLES: HeaderName = HeaderName::from_static("x-encabulator-roles");

/// The `http_route` label used for requests that matched no route. A literal keeps the metric's
/// cardinality bounded no matter what a scanner throws at the service.
const UNMATCHED_ROUTE: &str = "unmatched";

/// The request id, put into request extensions so handlers can read it.
#[derive(Clone, Debug)]
pub struct RequestId(pub String);

/// Reads or mints a request id.
fn request_id(request: &Request) -> String {
    request
        .headers()
        .get(&X_REQUEST_ID)
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        // A client-supplied id goes into logs and metadata, so cap its length and reject anything
        // that is not printable ASCII rather than trusting it.
        .filter(|value| {
            !value.is_empty() && value.len() <= 128 && value.bytes().all(|b| b.is_ascii_graphic())
        })
        .map(str::to_owned)
        .unwrap_or_else(|| uuid::Uuid::now_v7().to_string())
}

/// The route template for this request, never the raw path.
fn route_template(request: &Request) -> String {
    request
        .extensions()
        .get::<MatchedPath>()
        .map(|matched| matched.as_str().to_owned())
        .unwrap_or_else(|| UNMATCHED_ROUTE.to_owned())
}

fn header_str<'a>(request: &'a Request, name: &HeaderName) -> Option<&'a str> {
    request
        .headers()
        .get(name)
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty())
}

/// Minimal middleware for `/healthz`, `/readyz`, and `/metrics`: accept or generate a request id
/// and echo it, and nothing else. No span, no metric, no log, no malaise.
pub async fn request_id_only(mut request: Request, next: Next) -> Response {
    let id = request_id(&request);
    request.extensions_mut().insert(RequestId(id.clone()));
    let mut response = next.run(request).await;
    if let Ok(value) = HeaderValue::from_str(&id) {
        response.headers_mut().insert(X_REQUEST_ID, value);
    }
    response
}

/// The full middleware for API routes: request id, W3C-parented server span, server metrics, and
/// one access log line per request.
pub async fn observe(State(state): State<AppState>, mut request: Request, next: Next) -> Response {
    let id = request_id(&request);
    request.extensions_mut().insert(RequestId(id.clone()));

    let method = request.method().clone();
    let route = route_template(&request);
    let path = request.uri().path().to_owned();
    let subject = header_str(&request, &X_ENCABULATOR_SUBJECT).map(str::to_owned);
    let roles = header_str(&request, &X_ENCABULATOR_ROLES).map(str::to_owned);
    let parent = crate::telemetry::extract_context(request.headers());

    // OpenTelemetry HTTP semantic conventions: the span name is "{method} {route}".
    let span = tracing::info_span!(
        "http.server",
        otel.name = %format!("{method} {route}"),
        otel.kind = "server",
        otel.status_code = tracing::field::Empty,
        request_id = %id,
        http.request.method = %method,
        http.route = %route,
        url.path = %path,
        http.response.status_code = tracing::field::Empty,
        enduser.id = subject.as_deref(),
        enduser.role = roles.as_deref(),
    );
    // Ignored deliberately: set_parent only fails when no OpenTelemetry layer is installed, which
    // is the supported no-op configuration.
    let _ = span.set_parent(parent);

    let started = Instant::now();
    let mut response = next.run(request).instrument(span.clone()).await;
    let elapsed = started.elapsed();
    let status = response.status();

    span.record("http.response.status_code", status.as_u16());
    if status.is_server_error() {
        span.record("otel.status_code", "ERROR");
    }

    state.metrics.observe_server_request(
        method.as_str(),
        &route,
        status.as_u16(),
        elapsed.as_secs_f64(),
    );

    if let Ok(value) = HeaderValue::from_str(&id) {
        response.headers_mut().insert(X_REQUEST_ID, value);
    }

    // One access log line, emitted inside the span so it carries trace_id, span_id, and request_id.
    span.in_scope(|| {
        let duration_ms = elapsed.as_secs_f64() * 1_000.0;
        if status.is_server_error() {
            tracing::error!(status = status.as_u16(), duration_ms, "request failed");
        } else if status.is_client_error() {
            tracing::warn!(status = status.as_u16(), duration_ms, "request rejected");
        } else {
            tracing::info!(status = status.as_u16(), duration_ms, "request served");
        }
    });

    response
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::body::Body;

    fn request_with(header: Option<&str>) -> Request {
        let mut builder = Request::builder().uri("/v1/quotes");
        if let Some(value) = header {
            builder = builder.header("x-request-id", value);
        }
        builder.body(Body::empty()).unwrap()
    }

    #[test]
    fn a_client_supplied_request_id_is_honoured() {
        assert_eq!(
            request_id(&request_with(Some("req-abc-123"))),
            "req-abc-123"
        );
    }

    #[test]
    fn an_absent_request_id_is_generated_as_a_uuid() {
        let id = request_id(&request_with(None));
        assert_eq!(id.len(), 36, "{id}");
        assert!(uuid::Uuid::parse_str(&id).is_ok());
    }

    #[test]
    fn a_hostile_request_id_is_replaced() {
        for hostile in [
            "",
            "   ",
            &"x".repeat(200),
            "with space",
            "semi;colon and space",
        ] {
            let id = request_id(&request_with(Some(hostile)));
            assert!(uuid::Uuid::parse_str(&id).is_ok(), "accepted {hostile:?}");
        }
    }

    #[test]
    fn an_unmatched_request_gets_a_bounded_route_label() {
        assert_eq!(route_template(&request_with(None)), UNMATCHED_ROUTE);
    }

    #[test]
    fn identity_headers_are_read_but_optional() {
        let request = Request::builder()
            .uri("/v1/quotes")
            .header("x-encabulator-subject", " customer-7 ")
            .header("x-encabulator-roles", "")
            .body(Body::empty())
            .unwrap();
        assert_eq!(
            header_str(&request, &X_ENCABULATOR_SUBJECT),
            Some("customer-7")
        );
        assert_eq!(header_str(&request, &X_ENCABULATOR_ROLES), None);
    }
}
