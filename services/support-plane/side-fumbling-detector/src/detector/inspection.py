"""The inspection service: design section 4.2 step 3 and section 8.11.

Consume ``encabulator.fabrication.completed``, fetch each serial's readings from the recorder,
run the analysis, publish ``inspection.passed`` or ``inspection.failed``, and record an incident
for every unit that failed.
"""

from __future__ import annotations

import asyncio
import logging
from typing import Any

from encabulator_kit import Envelope, EventBus, FlagClient, ServiceClient, UpstreamError
from encabulator_kit.flags import STRICT_SIDE_FUMBLING
from encabulator_kit.messaging import Subjects
from prometheus_client import Counter, Histogram

from .analysis import UnitVerdict, inspect_unit
from .incidents import Incident, IncidentStore
from .stats import InspectionStats

log = logging.getLogger(__name__)

UNITS_INSPECTED = Counter("encabulator_units_inspected", "Units inspected.", ["result"])
ANOMALY_SCORE = Histogram(
    "encabulator_anomaly_score",
    "Worst rolling z-score seen in a unit's telemetry.",
    buckets=(0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 5.0, 6.0, 8.0, 12.0),
)


class MissingReadings(Exception):  # noqa: N818 - a retry signal, not a failure
    """The recorder has no telemetry for a serial yet. The message is naked and retried."""


class Inspector:
    def __init__(
        self,
        bus: EventBus,
        flags: FlagClient,
        *,
        recorder: ServiceClient | None,
        incidents: IncidentStore,
        stats: InspectionStats,
    ) -> None:
        self.bus = bus
        self.flags = flags
        self.recorder = recorder
        self.incidents = incidents
        self.stats = stats

    async def handle(self, envelope: Envelope, _message: Any = None) -> None:
        data = envelope.data
        order_id = str(data.get("orderId", ""))
        attempt = int(data.get("attempt", 1))
        serials = [str(serial) for serial in (data.get("serials") or [])]
        if not serials:
            log.warning("fabrication.completed with no serials", extra={"order_id": order_id})
            return

        strict = self.flags.bool(STRICT_SIDE_FUMBLING, subject=order_id)
        verdicts = await asyncio.gather(
            *(self._inspect(serial, strict=strict) for serial in serials)
        )

        failures = [verdict for verdict in verdicts if not verdict.passed]
        for verdict in verdicts:
            ANOMALY_SCORE.observe(verdict.score)
            UNITS_INSPECTED.labels("failed" if not verdict.passed else "passed").inc()
            self.stats.record(failed=not verdict.passed)

        if failures:
            await self.incidents.record(
                [
                    Incident.from_finding(verdict.finding, order_id=order_id, strict=strict)
                    for verdict in failures
                    if verdict.finding is not None
                ]
            )
            await self.bus.publish(
                Subjects.INSPECTION_FAILED,
                {
                    "orderId": order_id,
                    "attempt": attempt,
                    "serials": [verdict.serial for verdict in failures],
                    "findings": [
                        verdict.finding.to_json()
                        for verdict in failures
                        if verdict.finding is not None
                    ],
                },
            )
            log.info(
                "side fumbling detected",
                extra={
                    "order_id": order_id,
                    "attempt": attempt,
                    "failed": len(failures),
                    "inspected": len(serials),
                    "strict": strict,
                },
            )
            return

        await self.bus.publish(
            Subjects.INSPECTION_PASSED,
            {"orderId": order_id, "attempt": attempt, "serials": serials},
        )
        log.info(
            "no side fumbling found",
            extra={"order_id": order_id, "attempt": attempt, "inspected": len(serials)},
        )

    async def _inspect(self, serial: str, *, strict: bool) -> UnitVerdict:
        readings = await self._readings(serial)
        return inspect_unit(serial, readings, strict=strict)

    async def _readings(self, serial: str) -> list[dict[str, Any]]:
        if self.recorder is None:
            return []
        try:
            body = await self.recorder.json("GET", "/v1/readings", params={"serial": serial})
        except UpstreamError as exc:
            raise MissingReadings(f"recorder returned {exc.status_code} for {serial}") from exc
        readings = list((body or {}).get("readings") or [])
        if not readings:
            raise MissingReadings(f"no readings for {serial} yet")
        return readings
