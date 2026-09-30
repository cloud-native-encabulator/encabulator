"""A small in-process token bucket, used to rate limit registration per client IP.

Buckets live in a bounded LRU so a flood of distinct source addresses cannot grow the map without
limit. This is per pod on purpose: the detractor keeps no shared rate-limit state.
"""

from __future__ import annotations

import time
from collections import OrderedDict
from dataclasses import dataclass


@dataclass(slots=True)
class _Bucket:
    tokens: float
    updated: float


class RateLimiter:
    def __init__(self, per_minute: int, *, capacity: int = 10_000) -> None:
        self.rate = max(0.0, per_minute / 60.0)
        self.burst = float(max(1, per_minute))
        self.capacity = capacity
        self._buckets: OrderedDict[str, _Bucket] = OrderedDict()

    def allow(self, key: str, *, now: float | None = None) -> bool:
        if self.rate <= 0:
            return False
        moment = now if now is not None else time.monotonic()
        bucket = self._buckets.get(key)
        if bucket is None:
            bucket = _Bucket(tokens=self.burst, updated=moment)
            self._buckets[key] = bucket
            if len(self._buckets) > self.capacity:
                self._buckets.popitem(last=False)
        else:
            self._buckets.move_to_end(key)
            bucket.tokens = min(self.burst, bucket.tokens + (moment - bucket.updated) * self.rate)
            bucket.updated = moment
        if bucket.tokens < 1.0:
            return False
        bucket.tokens -= 1.0
        return True

    def retry_after(self, key: str) -> int:
        bucket = self._buckets.get(key)
        if bucket is None or self.rate <= 0:
            return 60
        deficit = max(0.0, 1.0 - bucket.tokens)
        return max(1, int(deficit / self.rate) + 1)
