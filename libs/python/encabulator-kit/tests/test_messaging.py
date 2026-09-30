"""Envelope and stream-config unit tests (no container needed)."""

from __future__ import annotations

from datetime import UTC, datetime

from encabulator_kit.messaging import (
    STREAM_CONFIG,
    STREAM_NAME,
    Envelope,
    Subjects,
)


def test_stream_config_matches_the_design() -> None:
    assert STREAM_CONFIG.name == STREAM_NAME == "ENCABULATOR"
    assert STREAM_CONFIG.subjects == ["encabulator.>"]
    assert STREAM_CONFIG.retention.value == "limits"
    assert STREAM_CONFIG.storage.value == "file"
    assert STREAM_CONFIG.max_age == 24 * 60 * 60
    assert STREAM_CONFIG.num_replicas == 1
    assert STREAM_CONFIG.duplicate_window == 120


def test_subjects_are_all_under_the_prefix() -> None:
    subjects = [
        value
        for name, value in vars(Subjects).items()
        if not name.startswith("_") and isinstance(value, str)
    ]
    assert len(subjects) == 10  # nine event subjects plus the wildcard
    assert all(subject.startswith("encabulator.") for subject in subjects)


def test_envelope_round_trip() -> None:
    envelope = Envelope.create(
        Subjects.FABRICATION_REQUESTED,
        {"orderId": "ord_1", "attempt": 1},
        "panametric-fam-processor",
    )
    decoded = Envelope.from_bytes(envelope.to_bytes())
    assert decoded.id == envelope.id
    assert decoded.type == "encabulator.fabrication.requested"
    assert decoded.source == "panametric-fam-processor"
    assert decoded.data == {"orderId": "ord_1", "attempt": 1}
    assert decoded.time.tzinfo is not None


def test_envelope_time_is_rfc3339_with_milliseconds() -> None:
    envelope = Envelope.create(
        Subjects.ORDER_ACCEPTED, {}, "x", time=datetime(2026, 9, 30, 12, 0, 0, 123456, tzinfo=UTC)
    )
    assert envelope.to_json()["time"] == "2026-09-30T12:00:00.123Z"


def test_envelope_id_is_prefixed() -> None:
    assert Envelope.create(Subjects.ORDER_ACCEPTED, {}, "x").id.startswith("evt_")
