"""The fabrication line itself: design section 4.2, steps 1 and 2.

One order at a time per slot, ``LINE_CAPACITY`` slots per pod, units within an order built
sequentially. Each stage sleeps its duration, generates readings, writes them to the recorder in
one batch, and publishes ``encabulator.fabrication.stage``. When every unit is done the line
commits the depot reservation (first attempt only) and publishes
``encabulator.fabrication.completed``.
"""

from __future__ import annotations

import asyncio
import logging
import random
import time
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from typing import Any

import numpy as np
from encabulator_kit import Envelope, EventBus, FlagClient
from encabulator_kit.flags import DINGLE_ARM_DAMPING, TURBO_ENCABULATION
from encabulator_kit.messaging import Subjects
from prometheus_client import Counter, Gauge, Histogram

from .domain import MODELS, STAGES, Model
from .serials import new_serial
from .signals import UnitSignal, choose_anomaly
from .timing import stage_duration_seconds
from .upstream import Depot, Recorder, readings_payload

log = logging.getLogger(__name__)

UNITS_FABRICATED = Counter("encabulator_units_fabricated", "Units fabricated.", ["model"])
STAGE_DURATION = Histogram(
    "encabulator_stage_duration_seconds",
    "Wall-clock duration of one fabrication stage.",
    ["stage"],
    buckets=(0.05, 0.1, 0.2, 0.3, 0.5, 0.75, 1.0, 1.5, 2.0, 5.0),
)
ACTIVE_ORDERS = Gauge("encabulator_line_active_orders", "Orders in fabrication on this pod.")


@dataclass(slots=True)
class ActiveUnit:
    order_id: str
    serial: str
    stage: str

    def to_json(self) -> dict[str, str]:
        return {"orderId": self.order_id, "serial": self.serial, "stage": self.stage}


@dataclass(slots=True)
class LineState:
    """What this pod is doing right now, for ``GET /v1/line``."""

    capacity: int
    units_completed: int = 0
    active: dict[str, ActiveUnit] = field(default_factory=dict)

    def begin(self, order_id: str, serial: str, stage: str) -> None:
        self.active[serial] = ActiveUnit(order_id, serial, stage)

    def advance(self, serial: str, stage: str) -> None:
        unit = self.active.get(serial)
        if unit is not None:
            unit.stage = stage

    def finish(self, serial: str) -> None:
        self.active.pop(serial, None)
        self.units_completed += 1

    def abandon(self, serial: str) -> None:
        self.active.pop(serial, None)

    def to_json(self) -> dict[str, Any]:
        return {
            "capacity": self.capacity,
            "activeOrders": len({unit.order_id for unit in self.active.values()}),
            "activeUnits": [unit.to_json() for unit in self.active.values()],
            "unitsCompleted": self.units_completed,
        }


def defect_probability(model: Model, *, dingle_arm: bool, damping_percent: float) -> float:
    """The model's base defect rate, reduced by the dingle arm's damping when one is fitted."""
    rate = model.base_defect_rate
    if dingle_arm:
        rate *= max(0.0, 1.0 - max(0.0, min(100.0, damping_percent)) / 100.0)
    return max(0.0, min(1.0, rate))


class FabricationLine:
    """Handles one ``encabulator.fabrication.requested`` message from start to finish."""

    def __init__(
        self,
        bus: EventBus,
        flags: FlagClient,
        *,
        recorder: Recorder,
        depot: Depot,
        state: LineState,
        rng: random.Random | None = None,
        np_rng: np.random.Generator | None = None,
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
    ) -> None:
        self.bus = bus
        self.flags = flags
        self.recorder = recorder
        self.depot = depot
        self.state = state
        self._rng = rng or random.Random()
        self._np_rng = np_rng or np.random.default_rng()
        # Injectable so tests can run the whole stage sequence without waiting 3.2 seconds.
        self._sleep = sleep

    async def handle(self, envelope: Envelope, _message: Any = None) -> None:
        data = envelope.data
        order_id = str(data.get("orderId", ""))
        attempt = int(data.get("attempt", 1))
        model_key = str(data.get("model", "turbo"))
        quantity = max(1, int(data.get("quantity", 1)))
        options = dict(data.get("options") or {})
        reservation_id = str(data.get("reservationId") or "")

        model = MODELS.get(model_key)
        if model is None:
            log.error("unknown model, dropping", extra={"order_id": order_id, "model": model_key})
            return

        ACTIVE_ORDERS.inc()
        started = time.perf_counter()
        serials: list[str] = []
        try:
            log.info(
                "order on the line",
                extra={
                    "order_id": order_id,
                    "attempt": attempt,
                    "model": model_key,
                    "quantity": quantity,
                },
            )
            for _ in range(quantity):
                serials.append(await self._fabricate_unit(order_id, attempt, model, options))
        finally:
            ACTIVE_ORDERS.dec()

        if attempt == 1 and reservation_id:
            await self.depot.commit(reservation_id)

        duration_ms = int((time.perf_counter() - started) * 1000)
        await self.bus.publish(
            Subjects.FABRICATION_COMPLETED,
            {
                "orderId": order_id,
                "attempt": attempt,
                "model": model_key,
                "serials": serials,
                "durationMs": duration_ms,
            },
        )
        log.info(
            "order off the line",
            extra={"order_id": order_id, "attempt": attempt, "duration_ms": duration_ms},
        )

    async def _fabricate_unit(
        self, order_id: str, attempt: int, model: Model, options: dict[str, Any]
    ) -> str:
        serial = new_serial(model.key)
        turbo = self.flags.bool(TURBO_ENCABULATION, subject=order_id)
        damping = self.flags.number(DINGLE_ARM_DAMPING)
        probability = defect_probability(
            model, dingle_arm=bool(options.get("dingleArm")), damping_percent=damping
        )
        # The line does not label the unit. It only decides whether the telemetry will contain an
        # anomaly; the detector has to find it.
        anomaly = (
            choose_anomaly(self._np_rng, len(STAGES)) if self._rng.random() < probability else None
        )
        signal = UnitSignal(self._np_rng, anomaly)

        self.state.begin(order_id, serial, STAGES[0].key)
        try:
            for stage in STAGES:
                self.state.advance(serial, stage.key)
                duration = stage_duration_seconds(stage, model, turbo=turbo, rng=self._rng)
                await self._sleep(duration)
                STAGE_DURATION.labels(stage.key).observe(duration)

                samples = signal.generate_stage(stage.index)
                await self.recorder.write(
                    readings_payload(
                        serial=serial,
                        order_id=order_id,
                        model=model.key,
                        stage=stage.key,
                        samples=samples,
                    ),
                    idempotency_key=f"{serial}:{stage.key}",
                )
                await self.bus.publish(
                    Subjects.FABRICATION_STAGE,
                    {
                        "orderId": order_id,
                        "attempt": attempt,
                        "serial": serial,
                        "stage": stage.key,
                        "stageIndex": stage.index,
                        "durationMs": int(duration * 1000),
                    },
                )
        except Exception:
            self.state.abandon(serial)
            raise

        self.state.finish(serial)
        UNITS_FABRICATED.labels(model.key).inc()
        return serial
