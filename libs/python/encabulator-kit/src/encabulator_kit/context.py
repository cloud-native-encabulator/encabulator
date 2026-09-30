"""Per-request context carried in context variables."""

from __future__ import annotations

from contextvars import ContextVar, Token

__all__ = ["current_request_id", "reset_request_id", "set_request_id"]

_request_id: ContextVar[str | None] = ContextVar("encabulator_request_id", default=None)


def current_request_id() -> str | None:
    return _request_id.get()


def set_request_id(request_id: str | None) -> Token[str | None]:
    return _request_id.set(request_id)


def reset_request_id(token: Token[str | None]) -> None:
    _request_id.reset(token)
