"""The error shape from design section 7: ``{"error": {"code", "message"}}``."""

from __future__ import annotations

from typing import Any

__all__ = ["DEFAULT_ERROR_CODES", "EncabulatorError", "UpstreamError", "error_body"]

DEFAULT_ERROR_CODES: dict[int, str] = {
    400: "invalid_request",
    401: "unauthorized",
    403: "forbidden",
    404: "not_found",
    405: "method_not_allowed",
    409: "conflict",
    413: "payload_too_large",
    415: "unsupported_media_type",
    422: "invalid_request",
    429: "rate_limited",
    500: "internal_error",
    502: "bad_gateway",
    503: "service_unavailable",
    504: "gateway_timeout",
}


def error_body(code: str, message: str) -> dict[str, Any]:
    return {"error": {"code": code, "message": message}}


class EncabulatorError(Exception):
    """An error that renders as the documented error shape.

    Raise this from a handler instead of ``HTTPException`` when you want to control the code.
    """

    def __init__(
        self,
        status_code: int,
        code: str | None = None,
        message: str | None = None,
        *,
        headers: dict[str, str] | None = None,
        details: dict[str, Any] | None = None,
    ) -> None:
        self.status_code = status_code
        self.code = code or DEFAULT_ERROR_CODES.get(status_code, "error")
        self.message = message or self.code.replace("_", " ")
        self.headers = headers or {}
        self.details = details or {}
        super().__init__(f"{status_code} {self.code}: {self.message}")

    def body(self) -> dict[str, Any]:
        body = error_body(self.code, self.message)
        if self.details:
            body["error"].update(self.details)
        return body


class UpstreamError(Exception):
    """A non-2xx response (or transport failure) from a service call."""

    def __init__(
        self,
        service: str,
        status_code: int,
        code: str = "upstream_error",
        message: str = "",
        *,
        body: Any = None,
    ) -> None:
        self.service = service
        self.status_code = status_code
        self.code = code
        self.message = message
        self.body = body
        super().__init__(f"{service} responded {status_code} {code}: {message}")
