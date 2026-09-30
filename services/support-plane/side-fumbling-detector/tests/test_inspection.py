"""The inspection flow: fabrication.completed in, inspection.passed/failed out."""

from __future__ import annotations

import pytest
from encabulator_kit import Envelope, FlagClient
from encabulator_kit.messaging import Subjects

from detector.incidents import IncidentStore
from detector.inspection import Inspector, MissingReadings
from detector.stats import InspectionStats

from .fakes import FakeBus, FakeValkey, recorder_client
from .generator import Fault, unit_readings


def _completed(serials: list[str], *, attempt: int = 1, order_id: str = "ord_fumble") -> Envelope:
    return Envelope.create(
        Subjects.FABRICATION_COMPLETED,
        {
            "orderId": order_id,
            "attempt": attempt,
            "model": "turbo",
            "serials": serials,
            "durationMs": 3200,
        },
        "logarithmic-casing-line",
    )


def _inspector(readings, *, valkey=None, flags=None):
    bus = FakeBus()
    store = IncidentStore(valkey or FakeValkey())
    stats = InspectionStats()
    inspector = Inspector(
        bus,  # type: ignore[arg-type]
        flags or FlagClient(None),
        recorder=recorder_client(readings),
        incidents=store,
        stats=stats,
    )
    return inspector, bus, store, stats


async def test_a_clean_order_passes_inspection() -> None:
    readings = {
        "ENC-a": unit_readings("ENC-a", seed=1),
        "ENC-b": unit_readings("ENC-b", seed=2),
    }
    inspector, bus, store, stats = _inspector(readings)
    await inspector.handle(_completed(["ENC-a", "ENC-b"]))

    passed = bus.data_for(Subjects.INSPECTION_PASSED)
    assert len(passed) == 1
    assert passed[0] == {"orderId": "ord_fumble", "attempt": 1, "serials": ["ENC-a", "ENC-b"]}
    assert bus.data_for(Subjects.INSPECTION_FAILED) == []
    assert await store.recent() == []
    assert stats.to_json() == {"inspected": 2, "passed": 2, "failed": 0, "failRate5m": 0.0}


async def test_a_fumbling_unit_fails_the_order() -> None:
    fault = Fault(stage_index=4, metric="marzlevane_alignment", kind="spike", sigmas=12.0, start=6)
    readings = {
        "ENC-good": unit_readings("ENC-good", seed=3),
        "ENC-bad": unit_readings("ENC-bad", seed=4, fault=fault),
    }
    inspector, bus, store, stats = _inspector(readings)
    await inspector.handle(_completed(["ENC-good", "ENC-bad"], attempt=2))

    failed = bus.data_for(Subjects.INSPECTION_FAILED)
    assert len(failed) == 1
    assert failed[0]["orderId"] == "ord_fumble"
    assert failed[0]["attempt"] == 2
    assert failed[0]["serials"] == ["ENC-bad"]
    finding = failed[0]["findings"][0]
    assert set(finding) == {"serial", "stage", "metric", "score", "reason"}
    assert finding["serial"] == "ENC-bad"
    assert finding["metric"] == "marzlevane_alignment"
    assert finding["stage"] == "fit-marzlevanes"
    assert bus.data_for(Subjects.INSPECTION_PASSED) == []
    assert stats.to_json()["failed"] == 1
    assert stats.to_json()["passed"] == 1


async def test_an_incident_is_recorded_for_each_failed_unit() -> None:
    fault = Fault(stage_index=2, metric="grammeter_drift", kind="drift", sigmas=12.0, start=7)
    readings = {
        "ENC-x": unit_readings("ENC-x", seed=5, fault=fault),
        "ENC-y": unit_readings("ENC-y", seed=6, fault=fault),
    }
    inspector, bus, store, stats = _inspector(readings)
    await inspector.handle(_completed(["ENC-x", "ENC-y"]))

    incidents = await store.recent()
    assert len(incidents) == 2
    assert {incident["serial"] for incident in incidents} == {"ENC-x", "ENC-y"}
    first = incidents[0]
    assert set(first) == {
        "id",
        "orderId",
        "serial",
        "stage",
        "metric",
        "score",
        "reason",
        "strict",
        "time",
    }
    assert first["id"].startswith("inc_")
    assert first["orderId"] == "ord_fumble"
    assert first["strict"] is False
    assert first["time"].endswith("Z")


async def test_strict_mode_is_recorded_on_the_incident() -> None:
    class StrictFlags(FlagClient):
        def bool(self, key: str, subject: str | None = None) -> bool:
            return key == "strict-side-fumbling"

    fault = Fault(stage_index=3, metric="repleneration", kind="spike", sigmas=12.0, start=6)
    readings = {"ENC-s": unit_readings("ENC-s", seed=7, fault=fault)}
    inspector, bus, store, _ = _inspector(readings, flags=StrictFlags(None))
    await inspector.handle(_completed(["ENC-s"]))
    assert (await store.recent())[0]["strict"] is True


async def test_missing_readings_nak_the_message() -> None:
    inspector, bus, store, stats = _inspector({})
    with pytest.raises(MissingReadings):
        await inspector.handle(_completed(["ENC-nothing"]))
    assert bus.published == []
    assert stats.inspected == 0


async def test_a_recorder_error_naks_the_message() -> None:
    bus = FakeBus()
    inspector = Inspector(
        bus,  # type: ignore[arg-type]
        FlagClient(None),
        recorder=recorder_client({}, status=503),
        incidents=IncidentStore(FakeValkey()),
        stats=InspectionStats(),
    )
    with pytest.raises(MissingReadings):
        await inspector.handle(_completed(["ENC-nothing"]))


async def test_an_event_with_no_serials_is_ignored() -> None:
    inspector, bus, store, stats = _inspector({})
    await inspector.handle(_completed([]))
    assert bus.published == []


async def test_a_valkey_outage_does_not_stop_the_verdict() -> None:
    fault = Fault(stage_index=5, metric="repleneration", kind="drift", sigmas=14.0, start=6)
    readings = {"ENC-v": unit_readings("ENC-v", seed=8, fault=fault)}
    inspector, bus, store, _ = _inspector(readings, valkey=FakeValkey(fail=True))
    await inspector.handle(_completed(["ENC-v"]))
    assert len(bus.data_for(Subjects.INSPECTION_FAILED)) == 1
    assert await store.recent() == []
