"""The control-plane services a scenario drives, and Prometheus.

Every client is optional: an unset URL turns its steps into a logged no-op, so a scenario can be
rehearsed against a partial deployment without special-casing anything.
"""

from __future__ import annotations

import logging
import operator
from dataclasses import dataclass
from typing import Any

from encabulator_kit import Identity, ServiceClient, UpstreamError

from .config import IDENTITY_ROLES, IDENTITY_SUBJECT

log = logging.getLogger(__name__)

IDENTITY = Identity.of(IDENTITY_SUBJECT, IDENTITY_ROLES)

COMPARISONS = {
    ">": operator.gt,
    ">=": operator.ge,
    "<": operator.lt,
    "<=": operator.le,
    "==": operator.eq,
    "!=": operator.ne,
}


@dataclass(slots=True)
class Upstreams:
    """One client per service the orchestrator talks to."""

    grammeter: ServiceClient | None = None
    malaise: ServiceClient | None = None
    girdlespring: ServiceClient | None = None
    depot: ServiceClient | None = None
    prometheus: ServiceClient | None = None

    @classmethod
    def from_settings(cls, settings: Any) -> Upstreams:
        def client(url: str | None, *, timeout: float = 5.0) -> ServiceClient | None:
            return ServiceClient(url, timeout=timeout) if url else None

        return cls(
            grammeter=client(settings.grammeter_url),
            malaise=client(settings.malaise_engine_url),
            girdlespring=client(settings.girdlespring_url),
            depot=client(settings.depot_url),
            prometheus=client(settings.prometheus_url, timeout=10.0),
        )

    async def aclose(self) -> None:
        for client in (
            self.grammeter,
            self.malaise,
            self.girdlespring,
            self.depot,
            self.prometheus,
        ):
            if client is not None:
                await client.aclose()

    # ---- cardinal-grammeter ------------------------------------------------------------

    async def set_traffic(self, body: dict[str, Any]) -> dict[str, Any] | None:
        if self.grammeter is None:
            log.info("CARDINAL_GRAMMETER_URL unset; skipping traffic step")
            return None
        return await self.grammeter.json("PUT", "/v1/traffic", json=body, identity=IDENTITY)

    # ---- malaise-injection-engine ------------------------------------------------------

    async def inject_malaise(self, body: dict[str, Any]) -> dict[str, Any] | None:
        if self.malaise is None:
            log.info("MALAISE_ENGINE_URL unset; skipping malaise step")
            return None
        return await self.malaise.json("POST", "/v1/malaises", json=body, identity=IDENTITY)

    async def clear_malaise(self, malaise_id: str) -> None:
        if self.malaise is None:
            return
        try:
            await self.malaise.json("DELETE", f"/v1/malaises/{malaise_id}", identity=IDENTITY)
        except UpstreamError as exc:
            if exc.status_code != 404:
                raise

    # ---- differential-girdlespring -----------------------------------------------------

    async def get_flag(self, key: str) -> dict[str, Any] | None:
        if self.girdlespring is None:
            return None
        try:
            return await self.girdlespring.json("GET", f"/v1/flags/{key}", identity=IDENTITY)
        except UpstreamError as exc:
            if exc.status_code == 404:
                return None
            raise

    async def set_flag(self, key: str, value: Any, **extra: Any) -> dict[str, Any] | None:
        if self.girdlespring is None:
            log.info("DIFFERENTIAL_GIRDLESPRING_URL unset; skipping flag step")
            return None
        body: dict[str, Any] = {"value": value}
        body.update({k: v for k, v in extra.items() if v is not None})
        return await self.girdlespring.json("PUT", f"/v1/flags/{key}", json=body, identity=IDENTITY)

    # ---- prefabulated-amulite-depot ----------------------------------------------------

    async def restock(self, sku: str, quantity: int) -> dict[str, Any] | None:
        if self.depot is None:
            log.info("PREFABULATED_AMULITE_DEPOT_URL unset; skipping restock step")
            return None
        return await self.depot.json(
            "POST",
            f"/v1/parts/{sku}/restock",
            json={"quantity": quantity},
            identity=IDENTITY,
        )

    # ---- prometheus --------------------------------------------------------------------

    async def query(self, promql: str) -> float | None:
        """Run an instant query and return the first sample's value, or None."""
        if self.prometheus is None:
            return None
        body = await self.prometheus.json("GET", "/api/v1/query", params={"query": promql})
        return _first_sample(body)


def _first_sample(body: Any) -> float | None:
    if not isinstance(body, dict) or body.get("status") != "success":
        return None
    data = body.get("data") or {}
    result = data.get("result")
    if data.get("resultType") == "scalar" and isinstance(result, list) and len(result) == 2:
        return _to_float(result[1])
    if isinstance(result, list) and result:
        first = result[0]
        value = (first or {}).get("value")
        if isinstance(value, list) and len(value) == 2:
            return _to_float(value[1])
    return None


def _to_float(raw: Any) -> float | None:
    try:
        value = float(raw)
    except TypeError, ValueError:
        return None
    return None if value != value else value  # NaN means "no data", not zero
