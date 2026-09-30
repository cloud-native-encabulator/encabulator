"""The generator half of the defect contract (design section 4.3)."""

from __future__ import annotations

import numpy as np
import pytest

from casing_line.domain import MODELS, STAGES
from casing_line.line import defect_probability
from casing_line.signals import (
    ALIGNMENT_SIGMA,
    DRIFT_STEP_SIGMA,
    METRICS,
    REPLENERATION_SIGMA,
    SAMPLE_INTERVAL_MS,
    SAMPLES_PER_STAGE,
    Anomaly,
    UnitSignal,
    choose_anomaly,
)


def _series(samples, metric: str) -> np.ndarray:
    return np.array([sample.value for sample in samples if sample.metric == metric])


def _clean_unit(seed: int = 0) -> list[list]:
    signal = UnitSignal(np.random.default_rng(seed))
    return [signal.generate_stage(stage.index) for stage in STAGES]


def test_shape_of_one_stage() -> None:
    samples = UnitSignal(np.random.default_rng(1)).generate_stage(1)
    assert len(samples) == 3 * SAMPLES_PER_STAGE
    assert {sample.metric for sample in samples} == set(METRICS)
    offsets = sorted({sample.offset_ms for sample in samples})
    assert offsets == [index * SAMPLE_INTERVAL_MS for index in range(SAMPLES_PER_STAGE)]
    assert offsets[-1] == 190


def test_repleneration_sits_around_one_inside_the_band() -> None:
    values = np.concatenate([_series(stage, "repleneration") for stage in _clean_unit(2)])
    assert abs(values.mean() - 1.0) < 0.02
    assert values.min() > 0.7
    assert values.max() < 1.3


def test_repleneration_is_actually_sinusoidal() -> None:
    values = _series(_clean_unit(3)[0], "repleneration")
    # One full cycle per stage: the first half sits above the second.
    assert values[:10].mean() > values[10:].mean()


def test_alignment_is_centred_noise() -> None:
    values = np.concatenate([_series(stage, "marzlevane_alignment") for stage in _clean_unit(4)])
    assert abs(values.mean()) < 0.1
    assert abs(values.std() - ALIGNMENT_SIGMA) < 0.05


def test_drift_is_a_cumulative_random_walk() -> None:
    stages = _clean_unit(5)
    values = np.concatenate([_series(stage, "grammeter_drift") for stage in stages])
    steps = np.diff(values)
    assert abs(steps.std() - DRIFT_STEP_SIGMA) < 0.12
    # Cumulative across stages: the walk continues rather than resetting to zero.
    first_stage_end = _series(stages[0], "grammeter_drift")[-1]
    second_stage_start = _series(stages[1], "grammeter_drift")[0]
    assert abs(second_stage_start - first_stage_end) < 6 * DRIFT_STEP_SIGMA


def test_noise_sigma_matches_the_contract() -> None:
    signal = UnitSignal(np.random.default_rng(6))
    values = np.concatenate(
        [_series(signal.generate_stage(stage.index), "repleneration") for stage in STAGES]
    )
    detrended = values - np.convolve(values, np.ones(5) / 5, mode="same")
    assert detrended.std() < 3 * REPLENERATION_SIGMA


@pytest.mark.parametrize("kind", ["spike", "drift"])
@pytest.mark.parametrize("metric", list(METRICS))
def test_injected_anomaly_shows_up_in_the_right_place(kind: str, metric: str) -> None:
    anomaly = Anomaly(stage_index=4, metric=metric, kind=kind, deviation_sigmas=9.0, start_sample=8)
    dirty = UnitSignal(np.random.default_rng(7), anomaly)
    clean = UnitSignal(np.random.default_rng(7))
    stages_dirty = [dirty.generate_stage(stage.index) for stage in STAGES]
    stages_clean = [clean.generate_stage(stage.index) for stage in STAGES]

    # Earlier stages are untouched.
    for index in range(3):
        assert np.allclose(
            _series(stages_dirty[index], metric), _series(stages_clean[index], metric)
        )
    # Stage 4 of that metric moved.
    difference = np.abs(_series(stages_dirty[3], metric) - _series(stages_clean[3], metric)).max()
    assert difference > 0
    # Other metrics in the same stage are untouched.
    for other in METRICS:
        if other != metric:
            assert np.allclose(_series(stages_dirty[3], other), _series(stages_clean[3], other))


def test_phase_slip_only_lands_on_repleneration() -> None:
    rng = np.random.default_rng(8)
    kinds = [choose_anomaly(rng, len(STAGES)) for _ in range(500)]
    slips = [anomaly for anomaly in kinds if anomaly.kind == "phase-slip"]
    assert slips, "phase slips should be drawn sometimes"
    assert {anomaly.metric for anomaly in slips} == {"repleneration"}
    assert all(1 <= anomaly.stage_index <= 7 for anomaly in kinds)


def test_phase_slip_inverts_the_sinusoid_and_swings_at_the_fault_magnitude() -> None:
    anomaly = Anomaly(
        stage_index=1,
        metric="repleneration",
        kind="phase-slip",
        deviation_sigmas=8.0,
        start_sample=10,
    )
    dirty = _series(
        UnitSignal(np.random.default_rng(9), anomaly).generate_stage(1), "repleneration"
    )
    clean = _series(UnitSignal(np.random.default_rng(9)).generate_stage(1), "repleneration")
    # Nothing before the slip changes, and after it the waveform is inverted and larger.
    assert np.allclose(dirty[:10], clean[:10])
    assert np.abs(dirty[10:] - 1.0).max() > 8.0 * REPLENERATION_SIGMA * 0.5
    assert np.abs(dirty[10:] - clean[10:]).max() > 0.1


def test_the_phase_slip_persists_into_later_stages() -> None:
    anomaly = Anomaly(
        stage_index=1,
        metric="repleneration",
        kind="phase-slip",
        deviation_sigmas=6.0,
        start_sample=10,
    )
    dirty = UnitSignal(np.random.default_rng(12), anomaly)
    clean = UnitSignal(np.random.default_rng(12))
    for stage in STAGES[:2]:
        dirty.generate_stage(stage.index)
        clean.generate_stage(stage.index)
    later_dirty = _series(dirty.generate_stage(3), "repleneration")
    later_clean = _series(clean.generate_stage(3), "repleneration")
    assert np.abs(later_dirty - later_clean).max() > 0.03


def test_defect_probability_uses_the_model_rate_and_damping() -> None:
    hyper = MODELS["hyper"]
    assert defect_probability(hyper, dingle_arm=False, damping_percent=50) == 0.06
    assert defect_probability(hyper, dingle_arm=True, damping_percent=50) == pytest.approx(0.03)
    assert defect_probability(hyper, dingle_arm=True, damping_percent=100) == 0.0
    assert defect_probability(hyper, dingle_arm=True, damping_percent=0) == 0.06
    # Out-of-range flag values are clamped rather than trusted.
    assert defect_probability(hyper, dingle_arm=True, damping_percent=500) == 0.0
    assert defect_probability(hyper, dingle_arm=True, damping_percent=-10) == 0.06
