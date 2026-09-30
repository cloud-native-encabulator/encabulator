"""JetStream integration tests against a real NATS container.

Run with a container runtime available:

    source dev-env.sh && make -C libs/python/encabulator-kit test
"""

from __future__ import annotations

import asyncio

import pytest

from encabulator_kit.malaise import Directive, MalaiseController
from encabulator_kit.messaging import (
    STREAM_NAME,
    Envelope,
    PullConsumer,
    Subjects,
    ensure_stream,
    open_bus,
)
from encabulator_kit.testing import nats_container

pytestmark = pytest.mark.integration


@pytest.fixture(scope="module")
def nats_url():
    with nats_container() as url:
        yield url


@pytest.fixture
async def bus(nats_url: str):
    bus = await open_bus("kit-integration-test", nats_url)
    try:
        yield bus
    finally:
        await bus.close()


async def test_ensure_stream_is_idempotent(bus) -> None:
    await ensure_stream(bus.js)
    await ensure_stream(bus.js)
    info = await bus.js.stream_info(STREAM_NAME)
    assert info.config.subjects == ["encabulator.>"]
    assert info.config.max_age == 24 * 60 * 60


async def test_publish_and_consume_round_trip(bus) -> None:
    received: list[Envelope] = []
    done = asyncio.Event()

    async def handler(envelope: Envelope, _msg) -> None:
        received.append(envelope)
        done.set()

    consumer = PullConsumer(
        bus,
        durable="kit-round-trip",
        filter_subject=Subjects.FABRICATION_REQUESTED,
        handler=handler,
        fetch_timeout=1.0,
    )
    await consumer.start()
    try:
        published = await bus.publish(
            Subjects.FABRICATION_REQUESTED,
            {"orderId": "ord_round_trip", "attempt": 1, "model": "turbo"},
        )
        await asyncio.wait_for(done.wait(), timeout=15)
    finally:
        await consumer.stop()

    assert len(received) == 1
    assert received[0].id == published.id
    assert received[0].data["orderId"] == "ord_round_trip"
    assert received[0].source == "kit-integration-test"


async def test_nats_msg_id_deduplicates(bus) -> None:
    event_id = "evt_dedupe_test"
    await bus.publish(Subjects.ORDER_ACCEPTED, {"orderId": "ord_a"}, event_id=event_id)
    before = (await bus.js.stream_info(STREAM_NAME)).state.messages
    await bus.publish(Subjects.ORDER_ACCEPTED, {"orderId": "ord_a"}, event_id=event_id)
    after = (await bus.js.stream_info(STREAM_NAME)).state.messages
    assert after == before, "JetStream should dedupe on Nats-Msg-Id inside the duplicate window"


async def test_trace_headers_are_present_on_published_messages(bus) -> None:
    subscription = await bus.js.pull_subscribe(
        Subjects.INSPECTION_PASSED, durable="kit-headers", stream=STREAM_NAME
    )
    await bus.publish(Subjects.INSPECTION_PASSED, {"orderId": "ord_h", "serials": []})
    messages = await subscription.fetch(batch=1, timeout=5)
    try:
        headers = messages[0].headers or {}
        assert headers["Nats-Msg-Id"].startswith("evt_")
    finally:
        await messages[0].ack()
        await subscription.unsubscribe()


async def test_handler_failure_naks_and_is_redelivered(bus) -> None:
    attempts: list[int] = []
    done = asyncio.Event()

    async def handler(envelope: Envelope, _msg) -> None:
        attempts.append(1)
        if len(attempts) == 1:
            raise RuntimeError("side fumbling during handling")
        done.set()

    consumer = PullConsumer(
        bus,
        durable="kit-nak",
        filter_subject=Subjects.FABRICATION_COMPLETED,
        handler=handler,
        fetch_timeout=1.0,
        nak_delay=0.1,
    )
    await consumer.start()
    try:
        await bus.publish(Subjects.FABRICATION_COMPLETED, {"orderId": "ord_nak", "serials": []})
        await asyncio.wait_for(done.wait(), timeout=20)
    finally:
        await consumer.stop()
    assert len(attempts) >= 2


async def test_injected_malaise_naks_the_message(bus) -> None:
    handled = 0
    malaise = MalaiseController("kit-integration-test", "http://malaise:8080")
    malaise.set_directives([Directive(id="mal_x", kind="error", params={"status": 503})])

    async def handler(envelope: Envelope, _msg) -> None:
        nonlocal handled
        handled += 1

    consumer = PullConsumer(
        bus,
        durable="kit-malaise",
        filter_subject=Subjects.SHIPMENT_DISPATCHED,
        handler=handler,
        fetch_timeout=1.0,
        nak_delay=5.0,
        malaise=malaise,
    )
    await consumer.start()
    try:
        await bus.publish(Subjects.SHIPMENT_DISPATCHED, {"orderId": "ord_m"})
        await asyncio.sleep(2.0)
        assert handled == 0, "the handler must not run while an error directive is active"
    finally:
        await consumer.stop()


async def test_concurrent_consumer_handles_several_at_once(bus) -> None:
    peak = 0
    active = 0
    seen = asyncio.Semaphore(0)

    async def handler(envelope: Envelope, _msg) -> None:
        nonlocal peak, active
        active += 1
        peak = max(peak, active)
        await asyncio.sleep(0.3)
        active -= 1
        seen.release()

    consumer = PullConsumer(
        bus,
        durable="kit-concurrency",
        filter_subject=Subjects.ORDER_SCRAPPED,
        handler=handler,
        concurrency=3,
        fetch_timeout=1.0,
    )
    await consumer.start()
    try:
        for index in range(6):
            await bus.publish(Subjects.ORDER_SCRAPPED, {"orderId": f"ord_{index}"})
        for _ in range(6):
            await asyncio.wait_for(seen.acquire(), timeout=20)
    finally:
        await consumer.stop()
    assert peak > 1


async def test_open_bus_waits_for_a_nats_that_is_not_there_yet(nats_url: str) -> None:
    """A wrong port stands in for "not up yet": the wait elapses, then it fails loudly."""
    import time

    started = time.monotonic()
    with pytest.raises(Exception):  # noqa: B017 - any failure to reach NATS is fine
        await open_bus("kit-integration-test", "nats://127.0.0.1:14222", wait_seconds=1.0)
    assert time.monotonic() - started >= 1.0
