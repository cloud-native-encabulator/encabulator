"""OpenTelemetry wiring, per design section 7.

OTLP over HTTP/protobuf to ``OTEL_EXPORTER_OTLP_ENDPOINT``. When that variable is unset,
tracing is a no-op: no provider is installed, no instrumentation is patched in, and
``trace.get_current_span()`` keeps returning the invalid span, so tests and local runs stay
quiet and the logger omits ``trace_id``.

Health, readiness, metrics, and the malaise/flag polling loops are never traced. The poller
exclusion is done with :func:`untraced`, which sets OpenTelemetry's suppression key so the
httpx instrumentation skips the call.
"""

from __future__ import annotations

import logging
from collections.abc import Iterator
from contextlib import contextmanager
from typing import TYPE_CHECKING, Any

from opentelemetry import context as otel_context
from opentelemetry import trace
from opentelemetry.trace import Tracer

if TYPE_CHECKING:  # pragma: no cover - typing only
    from fastapi import FastAPI

from .config import env_str

__all__ = [
    "EXCLUDED_URLS",
    "instrument_app",
    "instrument_sqlalchemy_engine",
    "setup_tracing",
    "shutdown_tracing",
    "tracer",
    "tracing_enabled",
    "untraced",
]

log = logging.getLogger(__name__)

#: Paths the FastAPI instrumentation must not create server spans for.
EXCLUDED_URLS = "healthz,readyz,metrics"

_SUPPRESS_KEY = "suppress_instrumentation"
try:  # the instrumentation packages use a UUID-suffixed key; prefer theirs when present
    from opentelemetry.instrumentation.utils import (  # type: ignore[attr-defined]
        _SUPPRESS_INSTRUMENTATION_KEY,
    )

    _SUPPRESS_KEY = _SUPPRESS_INSTRUMENTATION_KEY
except Exception:  # pragma: no cover - defensive
    pass

_provider: Any = None


def tracing_enabled() -> bool:
    return env_str("OTEL_EXPORTER_OTLP_ENDPOINT") is not None


def setup_tracing(service_name: str, service_version: str = "0.0.0") -> bool:
    """Install the tracer provider and client instrumentation. Returns False when disabled."""
    global _provider
    if not tracing_enabled():
        log.debug("tracing disabled: OTEL_EXPORTER_OTLP_ENDPOINT is unset")
        return False
    if _provider is not None:
        return True

    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor

    resource = Resource.create(
        {
            "service.name": service_name,
            "service.version": service_version,
            "service.namespace": "encabulator",
        }
    )
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    trace.set_tracer_provider(provider)
    _provider = provider

    _instrument_clients()
    log.info("tracing enabled", extra={"otlp_endpoint": env_str("OTEL_EXPORTER_OTLP_ENDPOINT")})
    return True


def _instrument_clients() -> None:
    """Patch the outbound clients we might be using. Missing libraries are simply skipped."""
    from importlib.util import find_spec

    attempts: list[tuple[str, str, str]] = [
        ("httpx", "opentelemetry.instrumentation.httpx", "HTTPXClientInstrumentor"),
        ("asyncpg", "opentelemetry.instrumentation.asyncpg", "AsyncPGInstrumentor"),
        ("redis", "opentelemetry.instrumentation.redis", "RedisInstrumentor"),
        ("sqlalchemy", "opentelemetry.instrumentation.sqlalchemy", "SQLAlchemyInstrumentor"),
    ]
    for library, module_name, class_name in attempts:
        if find_spec(library) is None:
            continue
        try:
            module = __import__(module_name, fromlist=[class_name])
            getattr(module, class_name)().instrument()
        except Exception:  # pragma: no cover - instrumentation is best effort
            log.warning("could not instrument %s", library, exc_info=True)


def instrument_sqlalchemy_engine(engine: Any) -> None:
    """Instrument one (async) SQLAlchemy engine. Safe to call when tracing is disabled."""
    if not tracing_enabled():
        return
    try:
        from opentelemetry.instrumentation.sqlalchemy import SQLAlchemyInstrumentor

        target = getattr(engine, "sync_engine", engine)
        SQLAlchemyInstrumentor().instrument(engine=target)
    except Exception:  # pragma: no cover
        log.warning("could not instrument SQLAlchemy engine", exc_info=True)


def instrument_app(app: FastAPI) -> None:
    """Add server-span instrumentation to a FastAPI app, excluding health and metrics."""
    if not tracing_enabled():
        return
    try:
        from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

        FastAPIInstrumentor.instrument_app(app, excluded_urls=EXCLUDED_URLS)
    except Exception:  # pragma: no cover
        log.warning("could not instrument FastAPI app", exc_info=True)


def tracer(name: str = "encabulator") -> Tracer:
    return trace.get_tracer(name)


@contextmanager
def untraced() -> Iterator[None]:
    """Suppress instrumentation inside the block (the 2s malaise and 5s flag pollers)."""
    token = otel_context.attach(otel_context.set_value(_SUPPRESS_KEY, True))
    try:
        yield
    finally:
        otel_context.detach(token)


def shutdown_tracing(timeout_millis: int = 5_000) -> None:
    """Flush and shut down the provider so the last spans of a run are not lost."""
    global _provider
    provider = _provider
    _provider = None
    if provider is None:
        return
    try:
        provider.force_flush(timeout_millis)
        provider.shutdown()
    except Exception:  # pragma: no cover
        log.warning("tracer provider shutdown failed", exc_info=True)
