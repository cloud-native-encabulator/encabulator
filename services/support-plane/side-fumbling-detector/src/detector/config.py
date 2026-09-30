"""Configuration, per design Appendix A."""

from __future__ import annotations

from dataclasses import dataclass

from encabulator_kit import CommonSettings, env_int, env_str

SERVICE_NAME = "side-fumbling-detector"
DURABLE = "side-fumbling-detector"

#: design section 8.11: the incident list is capped at 500.
INCIDENT_CAP = 500
INCIDENT_KEY = "incident:recent"


@dataclass(frozen=True, slots=True)
class Settings:
    common: CommonSettings
    recorder_url: str | None
    valkey_url: str | None
    concurrency: int

    @classmethod
    def from_env(cls) -> Settings:
        return cls(
            common=CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0"),
            recorder_url=env_str("SINUSOIDAL_RECORDER_URL"),
            valkey_url=env_str("VALKEY_URL"),
            concurrency=max(1, env_int("INSPECTION_CONCURRENCY", 2)),
        )
