from __future__ import annotations

from opentelemetry import context as otel_context

from encabulator_kit.telemetry import (
    EXCLUDED_URLS,
    instrument_app,
    setup_tracing,
    tracer,
    tracing_enabled,
    untraced,
)


def test_tracing_is_a_noop_without_an_endpoint(monkeypatch) -> None:
    monkeypatch.delenv("OTEL_EXPORTER_OTLP_ENDPOINT", raising=False)
    assert tracing_enabled() is False
    assert setup_tracing("kit-test") is False
    # Spans are created but never recorded, so the logger sees no trace id.
    with tracer("t").start_as_current_span("noop") as span:
        assert span.is_recording() is False


def test_health_and_metrics_are_excluded_from_tracing() -> None:
    assert set(EXCLUDED_URLS.split(",")) == {"healthz", "readyz", "metrics"}


def test_instrument_app_is_a_noop_when_disabled(monkeypatch) -> None:
    monkeypatch.delenv("OTEL_EXPORTER_OTLP_ENDPOINT", raising=False)
    from encabulator_kit.app import create_app

    app = create_app("kit-telemetry-test", configure_logs=False)
    before = len(app.user_middleware)
    instrument_app(app)
    assert len(app.user_middleware) == before


def test_untraced_sets_the_suppression_key() -> None:
    from encabulator_kit.telemetry import _SUPPRESS_KEY

    assert otel_context.get_value(_SUPPRESS_KEY) is None
    with untraced():
        assert otel_context.get_value(_SUPPRESS_KEY) is True
    assert otel_context.get_value(_SUPPRESS_KEY) is None
