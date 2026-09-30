"""Incident storage in Valkey: a capped list of the most recent findings.

Losing incidents is a cosmetic failure -- the inspection verdict itself travels over NATS -- so a
Valkey hiccup is logged and swallowed rather than allowed to fail a message.
"""

from __future__ import annotations

import json
import logging
from collections.abc import Sequence
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any, Protocol

from encabulator_kit import new_id

from .analysis import Finding
from .config import INCIDENT_CAP, INCIDENT_KEY

log = logging.getLogger(__name__)


class ValkeyLike(Protocol):
    """The three commands we need, so tests can supply a fake."""

    async def lpush(self, name: str, *values: str) -> int: ...
    async def ltrim(self, name: str, start: int, end: int) -> Any: ...
    async def lrange(self, name: str, start: int, end: int) -> list[Any]: ...


@dataclass(frozen=True, slots=True)
class Incident:
    id: str
    order_id: str
    serial: str
    stage: str
    metric: str
    score: float
    reason: str
    strict: bool
    time: datetime

    @classmethod
    def from_finding(
        cls, finding: Finding, *, order_id: str, strict: bool, when: datetime | None = None
    ) -> Incident:
        return cls(
            id=new_id("inc"),
            order_id=order_id,
            serial=finding.serial,
            stage=finding.stage,
            metric=finding.metric,
            score=finding.score,
            reason=finding.reason,
            strict=strict,
            time=when or datetime.now(UTC),
        )

    def to_json(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "orderId": self.order_id,
            "serial": self.serial,
            "stage": self.stage,
            "metric": self.metric,
            "score": round(self.score, 3),
            "reason": self.reason,
            "strict": self.strict,
            "time": self.time.astimezone(UTC)
            .isoformat(timespec="milliseconds")
            .replace("+00:00", "Z"),
        }


class IncidentStore:
    """A capped Valkey list of incidents, newest first."""

    def __init__(self, client: ValkeyLike | None, *, key: str = INCIDENT_KEY) -> None:
        self._client = client
        self.key = key

    @property
    def enabled(self) -> bool:
        return self._client is not None

    async def record(self, incidents: Sequence[Incident]) -> int:
        if self._client is None or not incidents:
            return 0
        payloads = [json.dumps(incident.to_json(), separators=(",", ":")) for incident in incidents]
        try:
            # Newest first: LPUSH in reverse so the last argument ends up at the head.
            await self._client.lpush(self.key, *reversed(payloads))
            await self._client.ltrim(self.key, 0, INCIDENT_CAP - 1)
        except Exception:
            log.warning("could not record incidents in valkey", exc_info=True)
            return 0
        return len(payloads)

    async def recent(self, limit: int = 50) -> list[dict[str, Any]]:
        if self._client is None:
            return []
        try:
            rows = await self._client.lrange(self.key, 0, max(0, limit - 1))
        except Exception:
            log.warning("could not read incidents from valkey", exc_info=True)
            return []
        out: list[dict[str, Any]] = []
        for row in rows:
            text = row.decode() if isinstance(row, bytes | bytearray) else str(row)
            try:
                out.append(json.loads(text))
            except ValueError:  # pragma: no cover - defensive
                continue
        return out
