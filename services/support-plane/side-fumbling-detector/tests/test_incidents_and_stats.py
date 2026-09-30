from __future__ import annotations

import json

from detector.analysis import Finding
from detector.config import INCIDENT_CAP
from detector.incidents import Incident, IncidentStore
from detector.stats import InspectionStats

from .fakes import FakeValkey


def _finding(serial: str) -> Finding:
    return Finding(
        serial=serial,
        stage="fit-marzlevanes",
        metric="marzlevane_alignment",
        score=7.1234,
        reason="rolling z-score 7.12 exceeds 4.0",
    )


async def test_incidents_are_newest_first_and_capped() -> None:
    valkey = FakeValkey()
    store = IncidentStore(valkey)
    for index in range(INCIDENT_CAP + 25):
        await store.record(
            [Incident.from_finding(_finding(f"ENC-{index}"), order_id="ord_1", strict=False)]
        )
    assert len(valkey.items) == INCIDENT_CAP
    recent = await store.recent(limit=3)
    assert [incident["serial"] for incident in recent] == [
        f"ENC-{INCIDENT_CAP + 24}",
        f"ENC-{INCIDENT_CAP + 23}",
        f"ENC-{INCIDENT_CAP + 22}",
    ]


async def test_a_batch_keeps_its_order() -> None:
    valkey = FakeValkey()
    store = IncidentStore(valkey)
    await store.record(
        [
            Incident.from_finding(_finding("ENC-1"), order_id="ord", strict=False),
            Incident.from_finding(_finding("ENC-2"), order_id="ord", strict=False),
        ]
    )
    assert [incident["serial"] for incident in await store.recent()] == ["ENC-1", "ENC-2"]


async def test_scores_are_rounded_for_the_wire() -> None:
    valkey = FakeValkey()
    await IncidentStore(valkey).record(
        [Incident.from_finding(_finding("ENC-1"), order_id="ord", strict=True)]
    )
    assert json.loads(valkey.items[0])["score"] == 7.123


async def test_a_disabled_store_is_harmless() -> None:
    store = IncidentStore(None)
    assert store.enabled is False
    assert (
        await store.record([Incident.from_finding(_finding("x"), order_id="o", strict=False)]) == 0
    )
    assert await store.recent() == []


async def test_byte_payloads_from_a_real_client_are_decoded() -> None:
    valkey = FakeValkey()
    store = IncidentStore(valkey)
    await store.record([Incident.from_finding(_finding("ENC-b"), order_id="ord", strict=False)])
    valkey.items = [item.encode() for item in valkey.items]
    assert (await store.recent())[0]["serial"] == "ENC-b"


def test_stats_counts_and_rolling_window() -> None:
    stats = InspectionStats()
    assert stats.to_json(now=0.0) == {
        "inspected": 0,
        "passed": 0,
        "failed": 0,
        "failRate5m": 0.0,
    }
    for index in range(10):
        stats.record(failed=index < 3, now=float(index))
    assert stats.inspected == 10
    assert stats.failed == 3
    assert stats.passed == 7
    assert stats.fail_rate_5m(now=10.0) == 0.3
    # Five minutes later the window has emptied.
    assert stats.fail_rate_5m(now=1000.0) == 0.0
    assert stats.inspected == 10  # the cumulative counters do not decay


def test_stats_window_drops_only_old_samples() -> None:
    stats = InspectionStats()
    stats.record(failed=True, now=0.0)
    stats.record(failed=False, now=299.0)
    assert stats.fail_rate_5m(now=299.0) == 0.5
    assert stats.fail_rate_5m(now=301.0) == 0.0
