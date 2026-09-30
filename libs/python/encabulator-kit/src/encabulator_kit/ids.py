"""Prefixed identifiers.

Design section 8: IDs are prefixed strings (``ord_``, ``evt_``, ``inc_`` ...) followed by a
UUIDv7 or a ULID. We mint ULIDs: 26 Crockford base32 characters, lexicographically sortable by
creation time, which keeps ledger and incident listings naturally ordered.
"""

from __future__ import annotations

import secrets
import time

__all__ = ["new_event_id", "new_id", "new_ulid", "new_uuid7"]

_CROCKFORD = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
_ULID_LEN = 26


def new_ulid(*, timestamp_ms: int | None = None) -> str:
    """Return a 26-character ULID: 48 bits of millisecond time, 80 bits of randomness."""
    ms = timestamp_ms if timestamp_ms is not None else time.time_ns() // 1_000_000
    value = ((ms & ((1 << 48) - 1)) << 80) | int.from_bytes(secrets.token_bytes(10), "big")
    out = bytearray(_ULID_LEN)
    for i in range(_ULID_LEN - 1, -1, -1):
        out[i] = ord(_CROCKFORD[value & 0x1F])
        value >>= 5
    return out.decode("ascii")


def new_id(prefix: str) -> str:
    """Return ``<prefix>_<ulid>``, e.g. ``ord_01K2E...``."""
    return f"{prefix}_{new_ulid()}"


def new_event_id() -> str:
    return new_id("evt")


def new_uuid7() -> str:
    """UUIDv7 as a string, for callers that prefer the RFC 9562 form."""
    import uuid

    return str(uuid.uuid7())
