"""The line's behaviour, with the bus and the upstreams faked."""

from __future__ import annotations

import random
from typing import Any

import numpy as np
import pytest
from encabulator_kit import Envelope, FlagClient
from encabulator_kit.messaging import Subjects

from casing_line.domain import STAGES
from casing_line.line import FabricationLine, LineState
from casing_line.upstream import Depot, Recorder, readings_payload


class FakeBus:
    def __init__(self) -> None:
        self.published: list[tuple[str, dict[str, Any]]] = []
        self.source = "logarithmic-casing-line"

    async def publish(self, subject: str, data: dict[str, Any], **_: Any) -> Envelope:
        self.published.append((subject, data))
        return Envelope.create(subject, data, self.source)

    def subjects(self) -> list[str]:
        return [subject for subject, _ in self.published]

    def data_for(self, subject: str) -> list[dict[str, Any]]:
        return [data for published, data in self.published if published == subject]


class FakeRecorder(Recorder):
    def __init__(self, *, fail: bool = False) -> None:
        super().__init__(None)
        self.batches: list[dict[str, Any]] = []
        self.keys: list[str] = []
        self.fail = fail

    @property
    def enabled(self) -> bool:
        return True

    async def write(self, payload: dict[str, Any], *, idempotency_key: str) -> None:
        if self.fail:
            raise RuntimeError("the recorder is not recording")
        self.batches.append(payload)
        self.keys.append(idempotency_key)


class FakeDepot(Depot):
    def __init__(self) -> None:
        super().__init__(None)
        self.committed: list[str] = []

    @property
    def enabled(self) -> bool:
        return True

    async def commit(self, reservation_id: str) -> bool:
        self.committed.append(reservation_id)
        return True


async def _instant(_seconds: float) -> None:
    """Skip the stage durations: the timing rules are covered in test_domain.py."""
    return None


@pytest.fixture
def rig():
    bus = FakeBus()
    recorder = FakeRecorder()
    depot = FakeDepot()
    state = LineState(capacity=2)
    flags = FlagClient(None)
    line = FabricationLine(
        bus,  # type: ignore[arg-type]
        flags,
        recorder=recorder,
        depot=depot,
        state=state,
        rng=random.Random(11),
        np_rng=np.random.default_rng(11),
        sleep=_instant,
    )
    return line, bus, recorder, depot, state


def _request(**overrides: Any) -> Envelope:
    data = {
        "orderId": "ord_01ABC",
        "attempt": 1,
        "model": "turbo",
        "quantity": 1,
        "options": {"dingleArm": False},
        "reservationId": "rsv_01XYZ",
    }
    data.update(overrides)
    return Envelope.create(Subjects.FABRICATION_REQUESTED, data, "panametric-fam-processor")


async def test_one_unit_runs_seven_stages_in_order(rig) -> None:
    line, bus, recorder, depot, state = rig
    await line.handle(_request())

    stage_events = bus.data_for(Subjects.FABRICATION_STAGE)
    assert len(stage_events) == 7
    assert [event["stageIndex"] for event in stage_events] == [1, 2, 3, 4, 5, 6, 7]
    assert [event["stage"] for event in stage_events] == [stage.key for stage in STAGES]
    assert all(event["orderId"] == "ord_01ABC" for event in stage_events)
    assert all(event["attempt"] == 1 for event in stage_events)
    assert all(event["durationMs"] >= 0 for event in stage_events)
    assert len({event["serial"] for event in stage_events}) == 1


async def test_readings_are_written_once_per_stage(rig) -> None:
    line, bus, recorder, depot, state = rig
    await line.handle(_request())
    assert len(recorder.batches) == 7
    assert len(recorder.batches[0]["readings"]) == 60  # 20 samples x 3 metrics
    first = recorder.batches[0]["readings"][0]
    assert set(first) == {"serial", "orderId", "model", "stage", "metric", "value", "time"}
    assert first["time"].endswith("Z")
    # The idempotency key makes the POST safely retryable.
    assert len(set(recorder.keys)) == 7


async def test_completion_event_carries_every_serial(rig) -> None:
    line, bus, recorder, depot, state = rig
    await line.handle(_request(quantity=3))
    completed = bus.data_for(Subjects.FABRICATION_COMPLETED)
    assert len(completed) == 1
    assert len(completed[0]["serials"]) == 3
    assert completed[0]["model"] == "turbo"
    assert completed[0]["attempt"] == 1
    assert completed[0]["durationMs"] >= 0
    assert len(bus.data_for(Subjects.FABRICATION_STAGE)) == 21
    assert state.units_completed == 3


async def test_reservation_is_committed_on_the_first_attempt_only(rig) -> None:
    line, bus, recorder, depot, state = rig
    await line.handle(_request(attempt=1))
    assert depot.committed == ["rsv_01XYZ"]
    await line.handle(_request(attempt=2))
    assert depot.committed == ["rsv_01XYZ"]


async def test_units_within_an_order_get_distinct_serials(rig) -> None:
    line, bus, recorder, depot, state = rig
    await line.handle(_request(quantity=4))
    serials = bus.data_for(Subjects.FABRICATION_COMPLETED)[0]["serials"]
    assert len(set(serials)) == 4
    assert all(serial.startswith("ENC-turbo-") for serial in serials)


async def test_unknown_model_is_dropped_not_naked(rig) -> None:
    line, bus, recorder, depot, state = rig
    await line.handle(_request(model="omniscillator"))
    assert bus.published == []


async def test_a_failed_recorder_write_aborts_the_message(rig) -> None:
    line, bus, _, depot, state = rig
    line.recorder = FakeRecorder(fail=True)
    with pytest.raises(RuntimeError):
        await line.handle(_request())
    # Nothing was reported as completed, and the unit is no longer shown as active.
    assert Subjects.FABRICATION_COMPLETED not in bus.subjects()
    assert state.active == {}
    assert state.units_completed == 0


async def test_line_state_tracks_active_units(rig) -> None:
    line, bus, recorder, depot, state = rig
    seen: list[dict[str, Any]] = []
    original = line.recorder.write

    async def spy(payload: dict[str, Any], *, idempotency_key: str) -> None:
        seen.append(state.to_json())
        await original(payload, idempotency_key=idempotency_key)

    line.recorder.write = spy  # type: ignore[method-assign]
    await line.handle(_request())

    assert seen[0]["capacity"] == 2
    assert seen[0]["activeOrders"] == 1
    assert seen[0]["activeUnits"][0]["orderId"] == "ord_01ABC"
    assert seen[0]["activeUnits"][0]["stage"] == "prelaminate-base-plate"
    assert seen[-1]["activeUnits"][0]["stage"] == "synchronize-cardinal-grammeters"
    assert state.to_json() == {
        "capacity": 2,
        "activeOrders": 0,
        "activeUnits": [],
        "unitsCompleted": 1,
    }


def test_readings_payload_spaces_samples_ten_milliseconds_apart() -> None:
    from datetime import UTC, datetime

    from casing_line.signals import UnitSignal

    samples = UnitSignal(np.random.default_rng(0)).generate_stage(1)
    payload = readings_payload(
        serial="ENC-turbo-X-00AA",
        order_id="ord_1",
        model="turbo",
        stage="fit-marzlevanes",
        samples=samples,
        base_time=datetime(2026, 9, 30, 12, 0, 0, tzinfo=UTC),
    )
    times = [reading["time"] for reading in payload["readings"]]
    assert times[0] == "2026-09-30T12:00:00.000Z"
    assert times[1] == "2026-09-30T12:00:00.010Z"
    assert times[19] == "2026-09-30T12:00:00.190Z"
    # Each metric restarts at the top of the stage.
    assert times[20] == "2026-09-30T12:00:00.000Z"
