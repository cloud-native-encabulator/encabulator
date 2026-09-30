"""Environment-variable configuration, per design Appendix A.

Configuration is environment variables only. Nothing in the kit reads a file.
"""

from __future__ import annotations

import os
from dataclasses import dataclass

__all__ = [
    "CommonSettings",
    "env_bool",
    "env_float",
    "env_int",
    "env_str",
    "require_env",
]

_TRUE = frozenset({"1", "true", "yes", "on"})
_FALSE = frozenset({"0", "false", "no", "off", ""})


def env_str(name: str, default: str | None = None) -> str | None:
    """Return a stripped environment variable, or ``default`` when unset/blank."""
    raw = os.getenv(name)
    if raw is None:
        return default
    raw = raw.strip()
    return raw or default


def require_env(name: str) -> str:
    value = env_str(name)
    if value is None:
        raise RuntimeError(f"required environment variable {name} is not set")
    return value


def env_int(name: str, default: int) -> int:
    value = env_str(name)
    if value is None:
        return default
    try:
        return int(value)
    except ValueError:
        return default


def env_float(name: str, default: float) -> float:
    value = env_str(name)
    if value is None:
        return default
    try:
        return float(value)
    except ValueError:
        return default


def env_bool(name: str, default: bool = False) -> bool:
    value = env_str(name)
    if value is None:
        return default
    lowered = value.lower()
    if lowered in _TRUE:
        return True
    if lowered in _FALSE:
        return False
    return default


@dataclass(frozen=True, slots=True)
class CommonSettings:
    """The variables every Encabulator service shares (design Appendix A)."""

    service_name: str
    service_version: str
    port: int
    log_level: str
    otlp_endpoint: str | None
    malaise_engine_url: str | None
    girdlespring_url: str | None
    nats_url: str

    @classmethod
    def from_env(cls, service_name: str, *, service_version: str = "0.0.0") -> CommonSettings:
        return cls(
            service_name=env_str("OTEL_SERVICE_NAME", service_name) or service_name,
            service_version=env_str("SERVICE_VERSION", service_version) or service_version,
            port=env_int("PORT", 8080),
            log_level=(env_str("LOG_LEVEL", "info") or "info").lower(),
            otlp_endpoint=env_str("OTEL_EXPORTER_OTLP_ENDPOINT"),
            malaise_engine_url=env_str("MALAISE_ENGINE_URL"),
            girdlespring_url=env_str("DIFFERENTIAL_GIRDLESPRING_URL"),
            nats_url=env_str("NATS_URL", "nats://nats:4222") or "nats://nats:4222",
        )
