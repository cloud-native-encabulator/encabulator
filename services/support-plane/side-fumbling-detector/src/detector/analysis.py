"""Anomaly detection over unit telemetry, per design section 4.3.

The casing line never says which units are defective. It writes telemetry, and a defective unit's
telemetry contains a spike, a drift, or a phase slip in one metric during one stage. This module
has to find it in the numbers, from nothing but the documented contract:

* three metrics -- ``repleneration`` (around 1.0), ``marzlevane_alignment`` (around 0), and
  ``grammeter_drift`` (a cumulative random walk around 0)
* 20 samples per metric per stage, 10 ms apart
* the nominal per-sample noise scale of each metric (0.05, 0.2, and 0.5 per step)

The rule (design section 4.3): fail a unit if any metric in any stage has a rolling z-score above
**4.0** (window 10), or if ``repleneration`` leaves **[0.7, 1.3]**. Strict mode tightens both to
z > 3.0 and [0.8, 1.2], which catches subtler anomalies and also produces false positives.

Three implementation decisions the design leaves open, and why:

1. **The window is trailing and excludes the sample being scored.** If the sample were inside its
   own window of 10, the largest possible z would be 9/sqrt(10) = 2.85 -- neither threshold could
   ever fire. Scoring a sample against the 10 that came before it is the standard formulation and
   the only one under which 4.0 and 3.0 mean anything.
2. **``grammeter_drift`` is scored on its first differences.** It is a random walk by
   construction, so its level wanders freely and a z-score on the level would fire constantly.
   The drift *rate* is the quantity that is supposed to be stable, and a fault in the walk shows
   up as a step in the rate.
3. **The window's standard deviation has a floor** of ``SIGMA_FLOOR_FACTOR`` times the metric's
   nominal noise scale. Without it, ten unusually quiet samples manufacture enormous z-scores out
   of ordinary noise. With it, the false-positive rate at 4.0 is negligible and at 3.0 it is a
   visible few percent per unit -- which is exactly the behaviour the design asks strict mode for.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

import numpy as np

__all__ = [
    "DEFAULT_BAND",
    "DEFAULT_Z_THRESHOLD",
    "NOMINAL_SIGMA",
    "SIGMA_FLOOR_FACTOR",
    "STRICT_BAND",
    "STRICT_Z_THRESHOLD",
    "WINDOW",
    "Finding",
    "Thresholds",
    "inspect_unit",
    "rolling_z_scores",
]

REPLENERATION = "repleneration"
MARZLEVANE_ALIGNMENT = "marzlevane_alignment"
GRAMMETER_DRIFT = "grammeter_drift"

#: The contract from design section 4.3.
NOMINAL_SIGMA: Mapping[str, float] = {
    REPLENERATION: 0.05,
    MARZLEVANE_ALIGNMENT: 0.2,
    GRAMMETER_DRIFT: 0.5,  # per step: grammeter_drift is a random walk
}

#: Metrics whose level wanders by design and must be differenced before scoring.
CUMULATIVE_METRICS = frozenset({GRAMMETER_DRIFT})

WINDOW = 10
DEFAULT_Z_THRESHOLD = 4.0
STRICT_Z_THRESHOLD = 3.0
DEFAULT_BAND = (0.7, 1.3)
STRICT_BAND = (0.8, 1.2)

#: Calibrated so that z > 4.0 almost never fires on clean telemetry while z > 3.0 sometimes does.
SIGMA_FLOOR_FACTOR = 1.3


@dataclass(frozen=True, slots=True)
class Thresholds:
    z: float
    band: tuple[float, float]
    strict: bool

    @classmethod
    def for_mode(cls, *, strict: bool) -> Thresholds:
        if strict:
            return cls(STRICT_Z_THRESHOLD, STRICT_BAND, True)
        return cls(DEFAULT_Z_THRESHOLD, DEFAULT_BAND, False)


@dataclass(frozen=True, slots=True)
class Finding:
    """Why one unit failed inspection. Matches the ``inspection.failed`` event's findings[]."""

    serial: str
    stage: str
    metric: str
    score: float
    reason: str

    def to_json(self) -> dict[str, object]:
        return {
            "serial": self.serial,
            "stage": self.stage,
            "metric": self.metric,
            "score": round(self.score, 3),
            "reason": self.reason,
        }


def rolling_z_scores(values: Sequence[float] | np.ndarray, sigma_floor: float) -> np.ndarray:
    """Score each sample against the ``WINDOW`` samples immediately before it.

    Returns an array the same length as ``values``; the first ``WINDOW`` entries are 0 because
    there is nothing yet to compare them with.
    """
    series = np.asarray(values, dtype=float)
    scores = np.zeros(series.shape, dtype=float)
    if series.size <= WINDOW:
        return scores
    windows = np.lib.stride_tricks.sliding_window_view(series[:-1], WINDOW)[: series.size - WINDOW]
    means = windows.mean(axis=1)
    deviations = np.maximum(windows.std(axis=1, ddof=1), sigma_floor)
    scores[WINDOW:] = np.abs(series[WINDOW:] - means) / deviations
    return scores


def _scored_series(metric: str, values: np.ndarray) -> np.ndarray:
    if metric in CUMULATIVE_METRICS:
        # The rate, not the level: a random walk has no stable level to score against.
        return np.diff(values, prepend=values[:1])
    return values


@dataclass(frozen=True, slots=True)
class UnitVerdict:
    """The outcome for one unit: pass or fail, the worst score seen, and why it failed."""

    serial: str
    passed: bool
    score: float
    finding: Finding | None


@dataclass(frozen=True, slots=True)
class _MetricResult:
    metric: str
    peak: float
    peak_stage: str
    band_stage: str | None = None
    band_value: float = 0.0


def _series_for(
    metric: str, rows: Sequence[tuple[str, str, float]]
) -> tuple[np.ndarray, list[str]]:
    ordered = sorted(rows, key=lambda row: row[0])
    values = np.asarray([row[2] for row in ordered], dtype=float)
    stages = [row[1] for row in ordered]
    return values, stages


def _examine(
    metric: str, rows: Sequence[tuple[str, str, float]], thresholds: Thresholds
) -> _MetricResult:
    """Score one metric across the whole unit.

    The window runs over the unit's series rather than restarting each stage: a 20-sample stage
    would otherwise leave its first ten samples unscored, and a fault in them invisible.
    """
    values, stages = _series_for(metric, rows)
    if values.size == 0:
        return _MetricResult(metric, 0.0, "")

    sigma_floor = SIGMA_FLOOR_FACTOR * NOMINAL_SIGMA.get(metric, 1.0)
    scores = rolling_z_scores(_scored_series(metric, values), sigma_floor)
    peak_index = int(np.argmax(scores)) if scores.size else 0
    peak = float(scores[peak_index]) if scores.size else 0.0

    band_stage: str | None = None
    band_value = 0.0
    if metric == REPLENERATION:
        low, high = thresholds.band
        below = float(values.min())
        above = float(values.max())
        if below < low or above > high:
            worst_index = (
                int(np.argmin(values)) if (low - below) > (above - high) else int(np.argmax(values))
            )
            band_stage = stages[worst_index]
            band_value = float(values[worst_index])

    return _MetricResult(metric, peak, stages[peak_index], band_stage, band_value)


def inspect_unit(
    serial: str,
    readings: Sequence[Mapping[str, object]],
    *,
    strict: bool = False,
) -> UnitVerdict:
    """Inspect every metric of one unit and decide whether it fumbled.

    ``readings`` are the recorder's rows -- ``{stage, metric, value, time}`` -- in any order; they
    are sorted by time per metric. The score returned is the worst rolling z-score seen anywhere
    in the unit, whether or not the unit failed, so the anomaly-score histogram stays meaningful.
    """
    by_metric: dict[str, list[tuple[str, str, float]]] = {}
    for reading in readings:
        metric = str(reading.get("metric", ""))
        if not metric:
            continue
        try:
            value = float(reading["value"])  # type: ignore[arg-type]
        except KeyError, TypeError, ValueError:
            continue
        by_metric.setdefault(metric, []).append(
            (str(reading.get("time", "")), str(reading.get("stage", "")), value)
        )

    thresholds = Thresholds.for_mode(strict=strict)
    results = [_examine(metric, rows, thresholds) for metric, rows in sorted(by_metric.items())]
    worst_score = max((result.peak for result in results), default=0.0)

    # A value outside the acceptable band is the least arguable failure, so it is reported first.
    for result in results:
        if result.band_stage is not None:
            low, high = thresholds.band
            return UnitVerdict(
                serial=serial,
                passed=False,
                score=worst_score,
                finding=Finding(
                    serial=serial,
                    stage=result.band_stage,
                    metric=result.metric,
                    score=max(worst_score, thresholds.z),
                    reason=(
                        f"{result.metric} reached {result.band_value:.3f}, outside [{low}, {high}]"
                    ),
                ),
            )

    exceeded = [result for result in results if result.peak > thresholds.z]
    if exceeded:
        worst = max(exceeded, key=lambda result: result.peak)
        return UnitVerdict(
            serial=serial,
            passed=False,
            score=worst_score,
            finding=Finding(
                serial=serial,
                stage=worst.peak_stage,
                metric=worst.metric,
                score=worst.peak,
                reason=f"rolling z-score {worst.peak:.2f} exceeds {thresholds.z}",
            ),
        )

    return UnitVerdict(serial=serial, passed=True, score=worst_score, finding=None)
