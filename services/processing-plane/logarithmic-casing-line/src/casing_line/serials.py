"""Unit serial numbers: ``ENC-<model>-<base36 time>-<4 hex>`` (design section 4.2).

The four hex digits come from a per-process counter seeded randomly at import rather than from
four fresh random digits: 65,536 values collide often enough to matter inside a single
millisecond, and a counter makes a pod's serials unique by construction while different pods
still start from different places.
"""

from __future__ import annotations

import itertools
import secrets
import time

__all__ = ["new_serial"]

_BASE36 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
_SUFFIX_SPACE = 0x1_0000
_counter = itertools.count(secrets.randbelow(_SUFFIX_SPACE))


def _base36(value: int) -> str:
    if value == 0:
        return "0"
    digits: list[str] = []
    while value:
        value, remainder = divmod(value, 36)
        digits.append(_BASE36[remainder])
    return "".join(reversed(digits))


def new_serial(model: str, *, timestamp_ms: int | None = None) -> str:
    ms = timestamp_ms if timestamp_ms is not None else time.time_ns() // 1_000_000
    suffix = next(_counter) % _SUFFIX_SPACE
    return f"ENC-{model}-{_base36(ms)}-{suffix:04X}"
