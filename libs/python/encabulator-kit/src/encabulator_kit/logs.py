"""JSON line logging, per design section 7.

Required fields: ``time`` (RFC 3339 with milliseconds), ``level``, ``msg``, ``service``.
``trace_id``/``span_id`` appear when a recording span is active and ``request_id`` when the
call arrived over HTTP. Anything passed through ``extra=`` is merged in as a top-level field,
so ``log.info("fabricated", extra={"order_id": oid})`` does the right thing.

Never log request bodies, tokens, or passwords.
"""

from __future__ import annotations

import json
import logging
import sys
from datetime import UTC, datetime
from typing import Any

from opentelemetry import trace

from .context import current_request_id

__all__ = ["JsonFormatter", "configure_logging", "get_logger"]

# LogRecord attributes we never copy into the payload; everything else in __dict__ is an extra.
_RESERVED = frozenset(
    {
        "args",
        "asctime",
        "color_message",  # uvicorn attaches an ANSI-coloured copy of its own message
        "created",
        "exc_info",
        "exc_text",
        "filename",
        "funcName",
        "getMessage",
        "levelname",
        "levelno",
        "lineno",
        "module",
        "msecs",
        "msg",
        "name",
        "pathname",
        "process",
        "processName",
        "relativeCreated",
        "stack_info",
        "taskName",
        "thread",
        "threadName",
    }
)


class JsonFormatter(logging.Formatter):
    """Renders a LogRecord as one JSON object on one line."""

    def __init__(self, service: str) -> None:
        super().__init__()
        self.service = service

    def format(self, record: logging.LogRecord) -> str:
        created = datetime.fromtimestamp(record.created, UTC)
        payload: dict[str, Any] = {
            "time": created.isoformat(timespec="milliseconds").replace("+00:00", "Z"),
            "level": _level_name(record.levelname),
            "msg": record.getMessage(),
            "service": self.service,
        }

        span_context = trace.get_current_span().get_span_context()
        if span_context.is_valid:
            payload["trace_id"] = trace.format_trace_id(span_context.trace_id)
            payload["span_id"] = trace.format_span_id(span_context.span_id)

        request_id = current_request_id()
        if request_id:
            payload["request_id"] = request_id

        if record.name and record.name != "root":
            payload["logger"] = record.name

        for key, value in record.__dict__.items():
            if key in _RESERVED or key.startswith("_") or key in payload:
                continue
            payload[key] = value

        if record.exc_info:
            payload["error"] = self.formatException(record.exc_info)
        elif record.exc_text:
            payload["error"] = record.exc_text
        if record.stack_info:
            payload["stack"] = record.stack_info

        return json.dumps(payload, default=_fallback, separators=(",", ":"))


def _fallback(value: Any) -> str:
    return repr(value)


def _level_name(levelname: str) -> str:
    lowered = levelname.lower()
    return "warn" if lowered == "warning" else lowered


def configure_logging(service: str, level: str = "info") -> None:
    """Install the JSON formatter on the root logger and tame uvicorn's own handlers."""
    numeric = logging.getLevelNamesMapping().get(
        {"warn": "WARNING"}.get(level.lower(), level.upper()), logging.INFO
    )

    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JsonFormatter(service))

    root = logging.getLogger()
    for existing in list(root.handlers):
        root.removeHandler(existing)
    root.addHandler(handler)
    root.setLevel(numeric)

    # uvicorn and asyncio ship their own handlers/levels; route them through ours.
    for name in ("uvicorn", "uvicorn.error", "uvicorn.access", "asyncio", "httpx", "httpcore"):
        logger = logging.getLogger(name)
        logger.handlers.clear()
        logger.propagate = True
    logging.getLogger("uvicorn.access").disabled = True
    logging.getLogger("httpx").setLevel(max(numeric, logging.WARNING))
    logging.getLogger("httpcore").setLevel(logging.WARNING)


def get_logger(name: str) -> logging.Logger:
    return logging.getLogger(name)
