"""The detector against a real NATS JetStream and a real Valkey."""

from __future__ import annotations

import asyncio
from typing import Any

import pytest
from encabulator_kit import Envelope, FlagClient, PullConsumer, open_bus
from encabulator_kit.messaging import Subjects
from encabulator_kit.testing import nats_container, valkey_container

from detector.config import INCIDENT_KEY
from detector.incidents import IncidentStore
from detector.inspection import Inspector
from detector.stats import InspectionStats

from .fakes import recorder_client
from .generator import Fault, unit_readings

pytestmark = pytest.mark.integration


@pytest.fixture(scope="module")
def nats_url():
    with nats_container() as url:
        yield url


@pytest.fixture(scope="module")
def valkey_url():
    with valkey_container() as url:
        yield url


@pytest.fixture
async def bus(nats_url: str):
    connection = await open_bus("side-fumbling-detector", nats_url)
    try:
        yield connection
    finally:
        await connection.close()


@pytest.fixture
async def valkey(valkey_url: str):
    from redis.asyncio import Redis

    client = Redis.from_url(valkey_url)
    await client.delete(INCIDENT_KEY)
    try:
        yield client
    finally:
        await client.aclose()


async def test_incidents_survive_a_round_trip_through_valkey(valkey) -> None:
    fault = Fault(stage_index=6, metric="repleneration", kind="spike", sigmas=12.0, start=6)
    store = IncidentStore(valkey)
    inspector = Inspector(
        _NullBus(),  # type: ignore[arg-type]
        FlagClient(None),
        recorder=recorder_client({"ENC-pg": unit_readings("ENC-pg", seed=21, fault=fault)}),
        incidents=store,
        stats=InspectionStats(),
    )
    await inspector.handle(
        Envelope.create(
            Subjects.FABRICATION_COMPLETED,
            {"orderId": "ord_valkey", "attempt": 1, "serials": ["ENC-pg"]},
            "logarithmic-casing-line",
        )
    )
    incidents = await store.recent(limit=10)
    assert len(incidents) == 1
    assert incidents[0]["serial"] == "ENC-pg"
    assert incidents[0]["orderId"] == "ord_valkey"
    assert await valkey.llen(INCIDENT_KEY) == 1


async def test_completed_in_verdict_out_over_jetstream(bus, valkey) -> None:
    fault = Fault(stage_index=2, metric="marzlevane_alignment", kind="drift", sigmas=14.0, start=7)
    readings = {
        "ENC-ok": unit_readings("ENC-ok", seed=22),
        "ENC-fumbled": unit_readings("ENC-fumbled", seed=23, fault=fault),
    }
    stats = InspectionStats()
    inspector = Inspector(
        bus,
        FlagClient(None),
        recorder=recorder_client(readings),
        incidents=IncidentStore(valkey),
        stats=stats,
    )

    verdicts: list[Envelope] = []
    seen = asyncio.Event()

    async def watch(envelope: Envelope, _msg: Any) -> None:
        verdicts.append(envelope)
        seen.set()

    watcher = PullConsumer(
        bus,
        durable="detector-test-watcher",
        filter_subject="encabulator.inspection.*",
        handler=watch,
        fetch_timeout=1.0,
    )
    worker = PullConsumer(
        bus,
        durable="side-fumbling-detector",
        filter_subject=Subjects.FABRICATION_COMPLETED,
        handler=inspector.handle,
        fetch_timeout=1.0,
    )
    await watcher.start()
    await worker.start()
    try:
        await bus.publish(
            Subjects.FABRICATION_COMPLETED,
            {
                "orderId": "ord_jetstream",
                "attempt": 1,
                "model": "turbo",
                "serials": ["ENC-ok", "ENC-fumbled"],
                "durationMs": 3200,
            },
        )
        await asyncio.wait_for(seen.wait(), timeout=30)
    finally:
        await worker.stop()
        await watcher.stop()

    assert len(verdicts) == 1
    assert verdicts[0].type == Subjects.INSPECTION_FAILED
    assert verdicts[0].data["serials"] == ["ENC-fumbled"]
    assert verdicts[0].data["findings"][0]["metric"] == "marzlevane_alignment"
    assert verdicts[0].source == "side-fumbling-detector"
    assert stats.to_json()["failed"] == 1


class _NullBus:
    source = "side-fumbling-detector"
    connected = True

    async def publish(self, subject: str, data: dict[str, Any], **_: Any) -> None:
        return None
