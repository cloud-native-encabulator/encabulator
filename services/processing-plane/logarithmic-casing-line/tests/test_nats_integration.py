"""The casing line end to end over a real NATS JetStream, with the recorder and depot stubbed."""

from __future__ import annotations

import asyncio
from typing import Any

import pytest
from encabulator_kit import Envelope, FlagClient, PullConsumer, open_bus
from encabulator_kit.messaging import STREAM_NAME, Subjects
from encabulator_kit.testing import nats_container

from casing_line.config import DURABLE, MAX_ACK_PENDING
from casing_line.line import FabricationLine, LineState

from .test_line import FakeDepot, FakeRecorder

pytestmark = pytest.mark.integration


@pytest.fixture(scope="module")
def nats_url():
    with nats_container() as url:
        yield url


@pytest.fixture
async def bus(nats_url: str):
    connection = await open_bus("logarithmic-casing-line", nats_url)
    try:
        yield connection
    finally:
        await connection.close()


async def _instant(_seconds: float) -> None:
    """Keep the stage sequence but not the 3.2 seconds of it."""
    return None


async def test_requested_in_stage_and_completed_out(bus) -> None:
    recorder = FakeRecorder()
    depot = FakeDepot()
    state = LineState(capacity=2)
    line = FabricationLine(
        bus, FlagClient(None), recorder=recorder, depot=depot, state=state, sleep=_instant
    )

    observed: list[Envelope] = []
    completed = asyncio.Event()

    async def watch(envelope: Envelope, _msg: Any) -> None:
        observed.append(envelope)
        if envelope.type == Subjects.FABRICATION_COMPLETED:
            completed.set()

    watcher = PullConsumer(
        bus,
        durable="casing-line-test-watcher",
        filter_subject="encabulator.fabrication.*",
        handler=watch,
        concurrency=4,
        fetch_timeout=1.0,
    )
    worker = PullConsumer(
        bus,
        durable=DURABLE,
        filter_subject=Subjects.FABRICATION_REQUESTED,
        handler=line.handle,
        concurrency=2,
        max_ack_pending=MAX_ACK_PENDING,
        fetch_timeout=1.0,
    )
    await watcher.start()
    await worker.start()
    try:
        await bus.publish(
            Subjects.FABRICATION_REQUESTED,
            {
                "orderId": "ord_integration",
                "attempt": 1,
                "model": "retro",
                "quantity": 2,
                "options": {"dingleArm": True},
                "reservationId": "rsv_integration",
            },
        )
        await asyncio.wait_for(completed.wait(), timeout=30)
        await asyncio.sleep(1.0)  # let the tail of the stage events arrive
    finally:
        await worker.stop()
        await watcher.stop()

    stage_events = [e for e in observed if e.type == Subjects.FABRICATION_STAGE]
    finished = [e for e in observed if e.type == Subjects.FABRICATION_COMPLETED]
    assert len(stage_events) == 14  # two units, seven stages each
    assert len(finished) == 1
    assert len(finished[0].data["serials"]) == 2
    assert finished[0].source == "logarithmic-casing-line"
    assert len(recorder.batches) == 14
    assert depot.committed == ["rsv_integration"]
    assert state.units_completed == 2


async def test_the_durable_consumer_is_configured_as_the_design_says(bus) -> None:
    consumer = PullConsumer(
        bus,
        durable=DURABLE,
        filter_subject=Subjects.FABRICATION_REQUESTED,
        handler=lambda *_: None,  # type: ignore[arg-type]
        concurrency=2,
        max_ack_pending=MAX_ACK_PENDING,
    )
    await consumer.subscribe()
    info = await bus.js.consumer_info(STREAM_NAME, DURABLE)
    try:
        assert info.config.durable_name == DURABLE
        assert info.config.filter_subject == Subjects.FABRICATION_REQUESTED
        assert str(getattr(info.config.ack_policy, "value", info.config.ack_policy)) == "explicit"
        assert info.config.ack_wait == 30
        assert info.config.max_deliver == 5
        assert info.config.max_ack_pending == MAX_ACK_PENDING
    finally:
        await consumer.stop()
