//! The one error shape every Encabulator service returns (design section 7):
//!
//! ```json
//! {"error": {"code": "snake_case_code", "message": "Human readable"}}
//! ```

use axum::Json;
use axum::extract::rejection::JsonRejection;
use axum::response::{IntoResponse, Response};
use http::StatusCode;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorBody {
    pub code: String,
    pub message: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorEnvelope {
    pub error: ErrorBody,
}

/// An error on its way to the client. Carries the status and the snake_case code together so that
/// no handler can pair a 400 with a 500's code.
#[derive(Debug, Clone)]
pub struct ApiError {
    pub status: StatusCode,
    pub code: &'static str,
    pub message: String,
}

impl ApiError {
    pub fn new(status: StatusCode, code: &'static str, message: impl Into<String>) -> Self {
        Self {
            status,
            code,
            message: message.into(),
        }
    }

    /// `400 unknown_model` (design section 8.3).
    pub fn unknown_model(model: &str) -> Self {
        Self::new(
            StatusCode::BAD_REQUEST,
            "unknown_model",
            format!("Unknown encabulator model '{model}'. Known models: turbo, retro, hyper."),
        )
    }

    /// `400 invalid_quantity`. The design fixes the range at 1 to 10 per order (section 3) but
    /// names no code for a violation, so the quoter uses this one.
    pub fn invalid_quantity(quantity: u32) -> Self {
        Self::new(
            StatusCode::BAD_REQUEST,
            "invalid_quantity",
            format!(
                "Quantity {quantity} is outside the accepted range {}-{}.",
                crate::pricing::MIN_QUANTITY,
                crate::pricing::MAX_QUANTITY
            ),
        )
    }

    pub fn invalid_request(message: impl Into<String>) -> Self {
        Self::new(StatusCode::BAD_REQUEST, "invalid_request", message)
    }

    pub fn quote_not_found(id: &str) -> Self {
        Self::new(
            StatusCode::NOT_FOUND,
            "quote_not_found",
            format!("No quote '{id}' is on file. Quotes are kept for one hour."),
        )
    }

    pub fn not_found() -> Self {
        Self::new(
            StatusCode::NOT_FOUND,
            "not_found",
            "No such route on the spurving bearing quoter.",
        )
    }

    pub fn method_not_allowed() -> Self {
        Self::new(
            StatusCode::METHOD_NOT_ALLOWED,
            "method_not_allowed",
            "That method is not supported on this route.",
        )
    }

    /// Valkey is configured but unreachable, and the request cannot be answered without it.
    /// Pricing never returns this; only `GET /v1/quotes/{id}` does.
    pub fn dependency_unavailable(dependency: &str) -> Self {
        Self::new(
            StatusCode::SERVICE_UNAVAILABLE,
            "dependency_unavailable",
            format!("{dependency} is unavailable."),
        )
    }

    /// Injected by the malaise middleware's `error` directive (design section 7).
    pub fn malaise_injected(status: StatusCode) -> Self {
        Self::new(
            status,
            "malaise_injected",
            "The malaise injection engine is inducing malaise on this route.",
        )
    }

    /// Injected by the malaise middleware's `congestion` directive once the queue times out.
    pub fn congested() -> Self {
        Self::new(
            StatusCode::SERVICE_UNAVAILABLE,
            "congested",
            "Too many requests are in the handler; the queue timed out.",
        )
    }

    pub fn internal(message: impl Into<String>) -> Self {
        Self::new(StatusCode::INTERNAL_SERVER_ERROR, "internal_error", message)
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        let body = ErrorEnvelope {
            error: ErrorBody {
                code: self.code.to_owned(),
                message: self.message,
            },
        };
        (self.status, Json(body)).into_response()
    }
}

impl From<JsonRejection> for ApiError {
    fn from(rejection: JsonRejection) -> Self {
        // Deliberately does not echo the rejection's full text for payload-too-large or IO
        // failures; the body must never reach the log or the client.
        match rejection {
            JsonRejection::JsonDataError(err) => Self::invalid_request(err.body_text()),
            JsonRejection::JsonSyntaxError(_) => {
                Self::invalid_request("Request body is not valid JSON.")
            }
            JsonRejection::MissingJsonContentType(_) => Self::new(
                StatusCode::UNSUPPORTED_MEDIA_TYPE,
                "unsupported_media_type",
                "Expected Content-Type: application/json.",
            ),
            _ => Self::invalid_request("Request body could not be read."),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::body::to_bytes;

    async fn body_of(err: ApiError) -> (StatusCode, ErrorEnvelope) {
        let response = err.into_response();
        let status = response.status();
        let bytes = to_bytes(response.into_body(), 64 * 1024).await.unwrap();
        (status, serde_json::from_slice(&bytes).unwrap())
    }

    #[tokio::test]
    async fn renders_the_canonical_envelope() {
        let (status, envelope) = body_of(ApiError::unknown_model("pentametric")).await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert_eq!(envelope.error.code, "unknown_model");
        assert!(envelope.error.message.contains("pentametric"));
    }

    #[tokio::test]
    async fn malaise_error_uses_the_directive_status() {
        let (status, envelope) = body_of(ApiError::malaise_injected(StatusCode::IM_A_TEAPOT)).await;
        assert_eq!(status, StatusCode::IM_A_TEAPOT);
        assert_eq!(envelope.error.code, "malaise_injected");
    }

    #[tokio::test]
    async fn every_code_is_snake_case() {
        let errors = [
            ApiError::unknown_model("x"),
            ApiError::invalid_quantity(99),
            ApiError::invalid_request("x"),
            ApiError::quote_not_found("qte_1"),
            ApiError::not_found(),
            ApiError::method_not_allowed(),
            ApiError::dependency_unavailable("Valkey"),
            ApiError::malaise_injected(StatusCode::SERVICE_UNAVAILABLE),
            ApiError::congested(),
            ApiError::internal("x"),
        ];
        for err in errors {
            assert!(
                err.code.chars().all(|c| c.is_ascii_lowercase() || c == '_'),
                "{} is not snake_case",
                err.code
            );
        }
    }
}
