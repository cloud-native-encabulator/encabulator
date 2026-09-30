"""Configuration, per design Appendix A."""

from __future__ import annotations

from dataclasses import dataclass

from encabulator_kit import CommonSettings, env_int, env_str

SERVICE_NAME = "logarithmic-casing-line"
DURABLE = "logarithmic-casing-line"

#: Design section 5: MaxAckPending for this durable.
MAX_ACK_PENDING = 64


@dataclass(frozen=True, slots=True)
class Settings:
    common: CommonSettings
    line_capacity: int
    recorder_url: str | None
    depot_url: str | None

    @classmethod
    def from_env(cls) -> Settings:
        return cls(
            common=CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0"),
            line_capacity=max(1, env_int("LINE_CAPACITY", 2)),
            recorder_url=env_str("SINUSOIDAL_RECORDER_URL"),
            depot_url=env_str("PREFABULATED_AMULITE_DEPOT_URL"),
        )
