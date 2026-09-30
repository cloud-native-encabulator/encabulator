from __future__ import annotations

import re

from encabulator_kit.ids import new_event_id, new_id, new_ulid, new_uuid7

ULID = re.compile(r"^[0-9ABCDEFGHJKMNPQRSTVWXYZ]{26}$")


def test_ulid_shape_and_alphabet() -> None:
    value = new_ulid()
    assert ULID.match(value), value


def test_ulids_sort_by_time() -> None:
    earlier = new_ulid(timestamp_ms=1_700_000_000_000)
    later = new_ulid(timestamp_ms=1_700_000_001_000)
    assert earlier < later


def test_ulids_are_unique() -> None:
    assert len({new_ulid() for _ in range(2000)}) == 2000


def test_prefixed_ids() -> None:
    assert new_id("ord").startswith("ord_")
    assert new_event_id().startswith("evt_")
    prefix, _, ulid = new_id("inc").partition("_")
    assert prefix == "inc"
    assert ULID.match(ulid)


def test_uuid7_is_available() -> None:
    assert len(new_uuid7()) == 36
