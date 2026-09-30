"""Inspection counters and the rolling five-minute fail rate for ``GET /v1/stats``."""

from __future__ import annotations

import time
from collections import deque
from dataclasses import dataclass, field

WINDOW_SECONDS = 300.0


@dataclass(slots=True)
class InspectionStats:
    inspected: int = 0
    passed: int = 0
    failed: int = 0
    _recent: deque[tuple[float, bool]] = field(default_factory=deque)

    def record(self, *, failed: bool, now: float | None = None) -> None:
        moment = now if now is not None else time.monotonic()
        self.inspected += 1
        if failed:
            self.failed += 1
        else:
            self.passed += 1
        self._recent.append((moment, failed))
        self._trim(moment)

    def fail_rate_5m(self, now: float | None = None) -> float:
        moment = now if now is not None else time.monotonic()
        self._trim(moment)
        if not self._recent:
            return 0.0
        failures = sum(1 for _, failed in self._recent if failed)
        return failures / len(self._recent)

    def _trim(self, now: float) -> None:
        cutoff = now - WINDOW_SECONDS
        while self._recent and self._recent[0][0] < cutoff:
            self._recent.popleft()

    def to_json(self, now: float | None = None) -> dict[str, float | int]:
        return {
            "inspected": self.inspected,
            "passed": self.passed,
            "failed": self.failed,
            "failRate5m": round(self.fail_rate_5m(now), 4),
        }
