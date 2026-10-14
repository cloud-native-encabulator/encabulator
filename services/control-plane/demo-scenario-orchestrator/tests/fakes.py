"""A stand-in for every control-plane service a scenario drives, plus Prometheus."""

from __future__ import annotations

import itertools
from dataclasses import dataclass, field
from typing import Any

from encabulator_kit import UpstreamError

from orchestrator.clients import Upstreams


@dataclass
class FakeUpstreams(Upstreams):
    """Records calls instead of making them, and serves scripted Prometheus answers."""

    traffic_calls: list[dict[str, Any]] = field(default_factory=list)
    malaises: list[dict[str, Any]] = field(default_factory=list)
    cleared: list[str] = field(default_factory=list)
    flag_writes: list[tuple[str, Any]] = field(default_factory=list)
    restocks: list[tuple[str, int]] = field(default_factory=list)
    queries: list[str] = field(default_factory=list)
    flags: dict[str, Any] = field(default_factory=dict)
    #: promql -> iterator of values the query returns in order (the last value repeats).
    samples: dict[str, list[float | None]] = field(default_factory=dict)
    fail_on: str | None = None
    prometheus_enabled: bool = True
    #: What GET /v1/traffic answers; every PUT updates it, like the real grammeter.
    traffic_state: dict[str, Any] | None = None
    #: Malaise targets the engine does not know (a managed store has no pod to delete).
    unknown_targets: set[str] = field(default_factory=set)
    _ids: itertools.count = field(default_factory=lambda: itertools.count(1))

    def __post_init__(self) -> None:
        self.prometheus = object() if self.prometheus_enabled else None  # type: ignore[assignment]

    async def aclose(self) -> None:
        return None

    async def get_traffic(self) -> dict[str, Any] | None:
        if self.fail_on == "get_traffic":
            raise RuntimeError("the grammeter is not listening")
        return None if self.traffic_state is None else dict(self.traffic_state)

    async def set_traffic(self, body: dict[str, Any]) -> dict[str, Any]:
        if self.fail_on == "traffic":
            raise RuntimeError("the grammeter is not listening")
        self.traffic_calls.append(dict(body))
        if self.traffic_state is not None:
            self.traffic_state.update(body)
        return dict(body)

    async def inject_malaise(self, body: dict[str, Any]) -> dict[str, Any]:
        if self.fail_on == "malaise":
            raise RuntimeError("the malaise engine is itself unwell")
        if self.fail_on == "malaise_400":
            raise UpstreamError(
                "malaise-injection-engine", 400, "invalid_params", "count must be > 0"
            )
        if body.get("target") in self.unknown_targets:
            raise UpstreamError(
                "malaise-injection-engine",
                400,
                "unknown_target",
                f"unknown malaise target: no malaise target with that name: {body['target']}",
            )
        self.malaises.append(dict(body))
        return {"id": f"mal_{next(self._ids)}", **body}

    async def clear_malaise(self, malaise_id: str) -> None:
        self.cleared.append(malaise_id)

    async def get_flag(self, key: str) -> dict[str, Any] | None:
        if key not in self.flags:
            return None
        return {"key": key, "value": self.flags[key], "type": "boolean"}

    async def set_flag(self, key: str, value: Any, **extra: Any) -> dict[str, Any]:
        if self.fail_on == "flag":
            raise RuntimeError("the girdlespring is stuck")
        self.flag_writes.append((key, value))
        self.flags[key] = value
        return {"key": key, "value": value}

    async def restock(self, sku: str, quantity: int) -> dict[str, Any]:
        self.restocks.append((sku, quantity))
        return {"sku": sku, "stock": quantity}

    async def query(self, promql: str) -> float | None:
        self.queries.append(promql)
        series = self.samples.get(promql)
        if not series:
            return None
        return series.pop(0) if len(series) > 1 else series[0]
