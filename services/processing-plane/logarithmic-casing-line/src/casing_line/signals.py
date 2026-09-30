"""Telemetry generation, per design section 4.3.

Each stage produces 20 samples of three metrics, 10 ms apart in synthetic time:

* ``repleneration`` -- sinusoidal, around 1.0, Gaussian noise sigma 0.05
* ``marzlevane_alignment`` -- degrees of misalignment around 0, noise sigma 0.2
* ``grammeter_drift`` -- cumulative drift in ppm around 0, a random walk with step sigma 0.5

**The line never labels a unit defective.** A defective unit simply gets an anomaly -- a spike, a
drift, or a phase slip -- in one metric during one stage, and ``side-fumbling-detector`` has to
find it in the numbers. The two services share only this documented contract.

Anomaly magnitudes are drawn so that the population spans the detector's two thresholds: most
are unmistakable, some are subtle enough that only strict inspection catches them, and a few
escape. That is what makes ``strict-side-fumbling`` visibly change the refabrication rate.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

import numpy as np

__all__ = [
    "ALIGNMENT_SIGMA",
    "DRIFT_STEP_SIGMA",
    "METRICS",
    "NOMINAL_SIGMA",
    "REPLENERATION_AMPLITUDE",
    "REPLENERATION_BASE",
    "REPLENERATION_SIGMA",
    "SAMPLES_PER_STAGE",
    "SAMPLE_INTERVAL_MS",
    "SINE_PERIOD_SAMPLES",
    "Anomaly",
    "AnomalyKind",
    "Sample",
    "UnitSignal",
    "choose_anomaly",
]

SAMPLES_PER_STAGE = 20
SAMPLE_INTERVAL_MS = 10
SINE_PERIOD_SAMPLES = 20  # one full repleneration cycle per stage

REPLENERATION = "repleneration"
MARZLEVANE_ALIGNMENT = "marzlevane_alignment"
GRAMMETER_DRIFT = "grammeter_drift"
METRICS: tuple[str, ...] = (REPLENERATION, MARZLEVANE_ALIGNMENT, GRAMMETER_DRIFT)

REPLENERATION_BASE = 1.0
REPLENERATION_AMPLITUDE = 0.03
REPLENERATION_SIGMA = 0.05
ALIGNMENT_SIGMA = 0.2
DRIFT_STEP_SIGMA = 0.5

#: The per-sample noise scale of each metric. For ``grammeter_drift`` this is the *step* sigma of
#: the walk, which is the scale of its first differences.
NOMINAL_SIGMA: dict[str, float] = {
    REPLENERATION: REPLENERATION_SIGMA,
    MARZLEVANE_ALIGNMENT: ALIGNMENT_SIGMA,
    GRAMMETER_DRIFT: DRIFT_STEP_SIGMA,
}

AnomalyKind = Literal["spike", "drift", "phase-slip"]

#: How far, in units of the metric's nominal sigma, an injected anomaly pushes the signal away
#: from where the detector's trailing window expects it to be.
MIN_DEVIATION = 4.0
MAX_DEVIATION = 12.0


@dataclass(frozen=True, slots=True)
class Anomaly:
    """Where the side fumbling is, and how loud."""

    stage_index: int
    metric: str
    kind: AnomalyKind
    #: Target deviation in units of the metric's nominal sigma (unused by ``phase-slip``).
    deviation_sigmas: float
    #: Sample index within the stage at which the anomaly starts.
    start_sample: int


@dataclass(frozen=True, slots=True)
class Sample:
    metric: str
    value: float
    offset_ms: int


def choose_anomaly(rng: np.random.Generator, stage_count: int) -> Anomaly:
    """Pick one metric in one stage, and a shape for the fault."""
    stage_index = int(rng.integers(1, stage_count + 1))
    kind: AnomalyKind = str(rng.choice(["spike", "drift", "phase-slip"], p=[0.4, 0.35, 0.25]))  # type: ignore[assignment]
    # Only the sinusoidal metric has a phase to slip.
    metric = REPLENERATION if kind == "phase-slip" else str(rng.choice(METRICS))
    return Anomaly(
        stage_index=stage_index,
        metric=metric,
        kind=kind,
        deviation_sigmas=float(rng.uniform(MIN_DEVIATION, MAX_DEVIATION)),
        start_sample=int(rng.integers(2, SAMPLES_PER_STAGE - 3)),
    )


class UnitSignal:
    """Generates one unit's telemetry, stage by stage, carrying state between stages.

    ``grammeter_drift`` is cumulative over the whole unit and the repleneration sinusoid runs
    continuously, so stages must be generated in order -- which is exactly how the line builds.
    """

    def __init__(self, rng: np.random.Generator, anomaly: Anomaly | None = None) -> None:
        self._rng = rng
        self.anomaly = anomaly
        self._drift = 0.0
        self._phase_offset = 0.0
        self._sample_index = 0

    def generate_stage(self, stage_index: int) -> list[Sample]:
        """Return 20 samples of each metric for one stage, 10 ms apart in synthetic time.

        The fault is injected into each metric's *innovation* -- the per-sample noise term for
        repleneration and alignment, the per-sample step for the grammeter drift walk -- so the
        three metrics share one fault geometry despite one of them being cumulative.
        """
        anomaly = (
            self.anomaly if (self.anomaly and self.anomaly.stage_index == stage_index) else None
        )
        innovations = {
            metric: self._rng.normal(0.0, NOMINAL_SIGMA[metric], SAMPLES_PER_STAGE)
            for metric in METRICS
        }
        if anomaly is not None and anomaly.kind in {"spike", "drift"}:
            innovations[anomaly.metric] = _inject(
                innovations[anomaly.metric], anomaly, NOMINAL_SIGMA[anomaly.metric]
            )

        series = {
            REPLENERATION: self._repleneration(innovations[REPLENERATION], anomaly),
            MARZLEVANE_ALIGNMENT: innovations[MARZLEVANE_ALIGNMENT],
            GRAMMETER_DRIFT: self._drift_walk(innovations[GRAMMETER_DRIFT]),
        }

        self._sample_index += SAMPLES_PER_STAGE
        return [
            Sample(metric=metric, value=float(values[index]), offset_ms=index * SAMPLE_INTERVAL_MS)
            for metric, values in series.items()
            for index in range(SAMPLES_PER_STAGE)
        ]

    # ---- per-metric generation ---------------------------------------------------------

    def _repleneration(self, noise: np.ndarray, anomaly: Anomaly | None) -> np.ndarray:
        positions = self._sample_index + np.arange(SAMPLES_PER_STAGE)
        phase = 2.0 * np.pi * positions / SINE_PERIOD_SAMPLES + self._phase_offset
        amplitude = np.full(SAMPLES_PER_STAGE, REPLENERATION_AMPLITUDE)
        if anomaly is not None and anomaly.kind == "phase-slip":
            # The lotus-o-delta winding slips half a cycle partway through the stage and stays
            # slipped, and the repleneration starts swinging at the fault's magnitude.
            slip = np.zeros(SAMPLES_PER_STAGE)
            slip[anomaly.start_sample :] = np.pi
            phase = phase + slip
            amplitude[anomaly.start_sample :] = anomaly.deviation_sigmas * REPLENERATION_SIGMA
            self._phase_offset = (self._phase_offset + np.pi) % (2.0 * np.pi)
        return REPLENERATION_BASE + amplitude * np.sin(phase) + noise

    def _drift_walk(self, steps: np.ndarray) -> np.ndarray:
        walk = self._drift + np.cumsum(steps)
        self._drift = float(walk[-1])
        return walk


def _inject(values: np.ndarray, anomaly: Anomaly, sigma: float) -> np.ndarray:
    """Add the fault to a metric series."""
    out = values.copy()
    magnitude = anomaly.deviation_sigmas * sigma
    sign = 1.0 if anomaly.start_sample % 2 == 0 else -1.0
    start = anomaly.start_sample

    if anomaly.kind == "spike":
        out[start] += sign * magnitude
        if start + 1 < len(out):
            out[start + 1] += sign * magnitude * 0.5
    elif anomaly.kind == "drift":
        # The metric drifts off to a new level at `start` and stays there for the rest of the
        # stage. A ramp would be a worse fault to inject than it looks: a trailing window's
        # standard deviation grows with the slope, so a linear ramp partly hides itself.
        out[start:] += sign * magnitude
    return out
