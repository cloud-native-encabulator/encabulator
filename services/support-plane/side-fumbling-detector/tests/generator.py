"""A small, independent implementation of the design section 4.3 signal contract.

This is a **test fixture**, not shared code. ``logarithmic-casing-line`` has its own generator;
the detector must work against anything that honours the contract, so duplicating the contract
here (rather than importing the line's code) is the point: if the two drift apart, these tests
notice.

Contract (design section 4.3), per stage, per metric: 20 samples, 10 ms apart.

* ``repleneration``: sinusoidal around 1.0, Gaussian noise sigma 0.05
* ``marzlevane_alignment``: degrees around 0, noise sigma 0.2
* ``grammeter_drift``: cumulative ppm around 0, random walk with step sigma 0.5
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Any, Literal

import numpy as np

SAMPLES = 20
INTERVAL_MS = 10
STAGES = (
    "prelaminate-base-plate",
    "surmount-logarithmic-casing",
    "align-spurving-bearings",
    "fit-marzlevanes",
    "wind-lotus-o-delta",
    "connect-tremie-pipes",
    "synchronize-cardinal-grammeters",
)
SIGMA = {"repleneration": 0.05, "marzlevane_alignment": 0.2, "grammeter_drift": 0.5}
AMPLITUDE = 0.03

Kind = Literal["spike", "drift", "phase-slip"]


@dataclass(frozen=True, slots=True)
class Fault:
    stage_index: int  # 1-based
    metric: str
    kind: Kind
    sigmas: float = 6.0
    start: int = 8


def unit_readings(
    serial: str,
    *,
    seed: int,
    fault: Fault | None = None,
    order_id: str = "ord_fixture",
    model: str = "turbo",
) -> list[dict[str, Any]]:
    """Generate one unit's worth of recorder rows: seven stages of three metrics."""
    rng = np.random.default_rng(seed)
    rows: list[dict[str, Any]] = []
    drift = 0.0
    phase_offset = 0.0
    base_time = datetime(2026, 9, 30, 12, 0, 0, tzinfo=UTC)

    for stage_index, stage in enumerate(STAGES, start=1):
        here = fault if (fault and fault.stage_index == stage_index) else None
        innovations = {metric: rng.normal(0.0, SIGMA[metric], SAMPLES) for metric in SIGMA}
        if here is not None and here.kind in {"spike", "drift"}:
            innovations[here.metric] = _inject(innovations[here.metric], here, SIGMA[here.metric])

        positions = (stage_index - 1) * SAMPLES + np.arange(SAMPLES)
        phase = 2.0 * np.pi * positions / SAMPLES + phase_offset
        amplitude = np.full(SAMPLES, AMPLITUDE)
        if here is not None and here.kind == "phase-slip":
            slip = np.zeros(SAMPLES)
            slip[here.start :] = np.pi
            phase = phase + slip
            amplitude[here.start :] = here.sigmas * SIGMA["repleneration"]
            phase_offset = (phase_offset + np.pi) % (2.0 * np.pi)

        series = {
            "repleneration": 1.0 + amplitude * np.sin(phase) + innovations["repleneration"],
            "marzlevane_alignment": innovations["marzlevane_alignment"],
            "grammeter_drift": drift + np.cumsum(innovations["grammeter_drift"]),
        }
        drift = float(series["grammeter_drift"][-1])

        stage_start = base_time + timedelta(milliseconds=(stage_index - 1) * SAMPLES * INTERVAL_MS)
        for metric, values in series.items():
            for index, value in enumerate(values):
                moment = stage_start + timedelta(milliseconds=index * INTERVAL_MS)
                rows.append(
                    {
                        "serial": serial,
                        "orderId": order_id,
                        "model": model,
                        "stage": stage,
                        "metric": metric,
                        "value": float(value),
                        "time": moment.isoformat(timespec="milliseconds").replace("+00:00", "Z"),
                    }
                )
    return rows


def _inject(values: np.ndarray, fault: Fault, sigma: float) -> np.ndarray:
    out = values.copy()
    magnitude = fault.sigmas * sigma
    sign = 1.0 if fault.start % 2 == 0 else -1.0
    if fault.kind == "spike":
        out[fault.start] += sign * magnitude
        if fault.start + 1 < len(out):
            out[fault.start + 1] += sign * magnitude * 0.5
    elif fault.kind == "drift":
        # A level shift held to the end of the stage.
        out[fault.start :] += sign * magnitude
    return out


def random_fault(rng: np.random.Generator) -> Fault:
    """A fault like the line would pick: random stage, metric, shape, and magnitude."""
    kind: Kind = str(rng.choice(["spike", "drift", "phase-slip"], p=[0.4, 0.35, 0.25]))  # type: ignore[assignment]
    metric = "repleneration" if kind == "phase-slip" else str(rng.choice(list(SIGMA)))
    return Fault(
        stage_index=int(rng.integers(1, len(STAGES) + 1)),
        metric=metric,
        kind=kind,
        sigmas=float(rng.uniform(4.0, 12.0)),
        start=int(rng.integers(2, SAMPLES - 3)),
    )
