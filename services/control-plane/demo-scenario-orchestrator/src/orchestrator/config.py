"""Configuration, per design Appendix A."""

from __future__ import annotations

from dataclasses import dataclass

from encabulator_kit import CommonSettings, env_int, env_str

SERVICE_NAME = "demo-scenario-orchestrator"

#: The orchestrator drives control-plane services on the presenter's behalf. It asserts an
#: operator identity, which is what girdlespring and the malaise engine require for writes.
IDENTITY_SUBJECT = SERVICE_NAME
IDENTITY_ROLES = ("operator",)


@dataclass(frozen=True, slots=True)
class Settings:
    common: CommonSettings
    grammeter_url: str | None
    malaise_engine_url: str | None
    girdlespring_url: str | None
    depot_url: str | None
    prometheus_url: str | None
    scenario_dir: str
    run_history: int

    @classmethod
    def from_env(cls) -> Settings:
        common = CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0")
        return cls(
            common=common,
            grammeter_url=env_str("CARDINAL_GRAMMETER_URL"),
            malaise_engine_url=common.malaise_engine_url,
            girdlespring_url=common.girdlespring_url,
            depot_url=env_str("PREFABULATED_AMULITE_DEPOT_URL"),
            prometheus_url=env_str("PROMETHEUS_URL"),
            scenario_dir=env_str("SCENARIO_DIR", "/etc/scenarios") or "/etc/scenarios",
            run_history=max(1, env_int("RUN_HISTORY", 50)),
        )
