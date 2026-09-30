"""Prometheus metrics, per design section 7.

The default ``prometheus_client`` registry already carries process, platform, and GC
collectors, which satisfies "the runtime's default process and runtime metrics".
"""

from __future__ import annotations

from collections.abc import Sequence

from prometheus_client import CONTENT_TYPE_LATEST, REGISTRY, Counter, Histogram, generate_latest

__all__ = [
    "CONTENT_TYPE_LATEST",
    "DURATION_BUCKETS",
    "MALAISE_APPLIED",
    "REGISTRY",
    "http_client_duration",
    "http_server_duration",
    "render",
]

DURATION_BUCKETS: tuple[float, ...] = (
    0.005,
    0.01,
    0.025,
    0.05,
    0.1,
    0.25,
    0.5,
    1.0,
    2.5,
    5.0,
    10.0,
)


def _histogram(name: str, documentation: str, labels: Sequence[str]) -> Histogram:
    """Create a histogram, or return the existing one (module reload / multi-app tests)."""
    existing = REGISTRY._names_to_collectors.get(name)
    if existing is not None:
        return existing  # type: ignore[return-value]
    return Histogram(name, documentation, list(labels), buckets=DURATION_BUCKETS)


def _counter(name: str, documentation: str, labels: Sequence[str]) -> Counter:
    existing = REGISTRY._names_to_collectors.get(name)
    if existing is not None:
        return existing  # type: ignore[return-value]
    return Counter(name, documentation, list(labels))


_SERVER = _histogram(
    "http_server_request_duration_seconds",
    "Duration of inbound HTTP requests, by route template.",
    ("http_request_method", "http_route", "http_response_status_code"),
)

_CLIENT = _histogram(
    "http_client_request_duration_seconds",
    "Duration of outbound HTTP requests, by target service.",
    ("http_request_method", "server_address", "http_response_status_code"),
)

MALAISE_APPLIED = _counter(
    "encabulator_malaise_applied",
    "Injected malaise directives applied, by kind.",
    ("kind",),
)


def http_server_duration(method: str, route: str, status_code: int, seconds: float) -> None:
    _SERVER.labels(method, route, str(status_code)).observe(seconds)


def http_client_duration(
    method: str, server_address: str, status_code: int, seconds: float
) -> None:
    _CLIENT.labels(method, server_address, str(status_code)).observe(seconds)


def render() -> bytes:
    return generate_latest(REGISTRY)
