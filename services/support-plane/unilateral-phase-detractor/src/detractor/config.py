"""Configuration, per design Appendix A."""

from __future__ import annotations

from dataclasses import dataclass

from encabulator_kit import CommonSettings, env_int, env_str

SERVICE_NAME = "unilateral-phase-detractor"

#: The JWT issuer and audience are fixed by the design; the gateway checks both.
ISSUER = "unilateral-phase-detractor"
AUDIENCE = "encabulator"


@dataclass(frozen=True, slots=True)
class Settings:
    common: CommonSettings
    database_url: str
    jwt_private_key_pem: str | None
    jwt_ttl_seconds: int
    bootstrap_admin_password: str | None
    bootstrap_operator_password: str | None
    register_rate_per_minute: int

    @classmethod
    def from_env(cls) -> Settings:
        return cls(
            common=CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0"),
            database_url=env_str("DATABASE_URL", "postgresql://detractor@postgres:5432/detractor")
            or "",
            jwt_private_key_pem=env_str("JWT_PRIVATE_KEY"),
            jwt_ttl_seconds=env_int("JWT_TTL_SECONDS", 3600),
            bootstrap_admin_password=env_str("BOOTSTRAP_ADMIN_PASSWORD"),
            bootstrap_operator_password=env_str("BOOTSTRAP_OPERATOR_PASSWORD"),
            register_rate_per_minute=env_int("REGISTER_RATE_PER_MINUTE", 10),
        )
