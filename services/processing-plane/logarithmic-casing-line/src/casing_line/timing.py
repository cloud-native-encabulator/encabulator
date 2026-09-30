"""Stage timing, per design section 3.

Base durations are milliseconds, multiplied by the model's stage time factor, halved when the
``turbo-encabulation`` flag is on, and jittered plus or minus 25 percent.
"""

from __future__ import annotations

import random

from .domain import Model, Stage

__all__ = ["JITTER", "TURBO_FACTOR", "stage_duration_seconds"]

JITTER = 0.25
TURBO_FACTOR = 0.5


def stage_duration_seconds(
    stage: Stage, model: Model, *, turbo: bool, rng: random.Random | None = None
) -> float:
    milliseconds = stage.base_ms * model.stage_time_factor
    if turbo:
        milliseconds *= TURBO_FACTOR
    jitter = (rng or random).uniform(-JITTER, JITTER)
    return max(0.0, milliseconds * (1.0 + jitter)) / 1000.0
