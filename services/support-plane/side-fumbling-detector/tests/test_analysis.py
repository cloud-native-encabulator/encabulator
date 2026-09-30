"""The detector against generated clean and defective telemetry.

The series come from ``tests/generator.py``, an independent implementation of the design section
4.3 contract. The detector has never seen the casing line's code, and vice versa.
"""

from __future__ import annotations

import numpy as np
import pytest

from detector.analysis import (
    DEFAULT_BAND,
    DEFAULT_Z_THRESHOLD,
    STRICT_BAND,
    STRICT_Z_THRESHOLD,
    WINDOW,
    Thresholds,
    inspect_unit,
    rolling_z_scores,
)

from .generator import STAGES, Fault, random_fault, unit_readings

POPULATION = 400


def test_thresholds_match_the_design() -> None:
    default = Thresholds.for_mode(strict=False)
    strict = Thresholds.for_mode(strict=True)
    assert (default.z, default.band) == (4.0, (0.7, 1.3))
    assert (strict.z, strict.band) == (3.0, (0.8, 1.2))
    assert (DEFAULT_Z_THRESHOLD, STRICT_Z_THRESHOLD) == (4.0, 3.0)
    assert (DEFAULT_BAND, STRICT_BAND) == ((0.7, 1.3), (0.8, 1.2))
    assert WINDOW == 10


# ---- the rolling z-score itself ------------------------------------------------------------


def test_the_window_is_trailing_and_excludes_the_sample_it_scores() -> None:
    """If the sample were inside its own window of 10, max |z| would be 9/sqrt(10) = 2.85 and
    neither 4.0 nor 3.0 could ever fire."""
    series = [0.0] * 10 + [10.0]
    scores = rolling_z_scores(series, sigma_floor=1.0)
    assert scores[:WINDOW].tolist() == [0.0] * WINDOW
    assert scores[10] == pytest.approx(10.0)  # (10 - 0) / floor


def test_the_sigma_floor_stops_a_quiet_window_manufacturing_scores() -> None:
    series = [1.0] * 10 + [1.05]
    assert rolling_z_scores(series, sigma_floor=1.0)[10] == pytest.approx(0.05)
    # Without a floor the zero-variance window would divide by (almost) nothing.
    assert rolling_z_scores(series, sigma_floor=1e-9)[10] > 1000


def test_short_series_score_zero() -> None:
    assert rolling_z_scores([1.0, 2.0, 3.0], 1.0).tolist() == [0.0, 0.0, 0.0]


# ---- clean units ---------------------------------------------------------------------------


def _clean_verdicts(strict: bool) -> list:
    return [
        inspect_unit(
            f"ENC-clean-{seed}", unit_readings(f"ENC-clean-{seed}", seed=seed), strict=strict
        )
        for seed in range(POPULATION)
    ]


def test_clean_units_pass_default_inspection() -> None:
    verdicts = _clean_verdicts(strict=False)
    failures = [verdict for verdict in verdicts if not verdict.passed]
    rate = len(failures) / len(verdicts)
    assert rate < 0.01, (
        f"default false positive rate {rate:.2%}: {[f.finding for f in failures[:3]]}"
    )


def test_strict_inspection_produces_some_false_positives() -> None:
    """Design section 4.3: strict mode "also produces false positives, so the refabrication rate
    visibly rises". It must rise, and it must not swallow the factory."""
    strict_rate = sum(1 for v in _clean_verdicts(strict=True) if not v.passed) / POPULATION
    default_rate = sum(1 for v in _clean_verdicts(strict=False) if not v.passed) / POPULATION
    assert strict_rate > default_rate
    assert 0.02 < strict_rate < 0.35, f"strict false positive rate {strict_rate:.2%}"


def test_a_passing_unit_still_reports_a_score() -> None:
    verdict = inspect_unit("ENC-clean-1", unit_readings("ENC-clean-1", seed=1))
    assert verdict.passed
    assert verdict.finding is None
    assert 0.0 < verdict.score < DEFAULT_Z_THRESHOLD


# ---- defective units -----------------------------------------------------------------------


def _defective_rate(strict: bool, seed: int = 99) -> float:
    rng = np.random.default_rng(seed)
    caught = 0
    for index in range(POPULATION):
        fault = random_fault(rng)
        verdict = inspect_unit(
            "ENC-bad", unit_readings("ENC-bad", seed=50_000 + index, fault=fault), strict=strict
        )
        caught += 0 if verdict.passed else 1
    return caught / POPULATION


def test_default_inspection_catches_most_defects() -> None:
    assert _defective_rate(strict=False) > 0.65


def test_strict_inspection_catches_subtler_defects() -> None:
    strict = _defective_rate(strict=True)
    default = _defective_rate(strict=False)
    assert strict > default + 0.05, f"strict {strict:.1%} vs default {default:.1%}"
    assert strict > 0.90


@pytest.mark.parametrize("metric", ["repleneration", "marzlevane_alignment", "grammeter_drift"])
@pytest.mark.parametrize("kind", ["spike", "drift"])
def test_a_loud_fault_in_any_metric_is_caught(metric: str, kind: str) -> None:
    fault = Fault(stage_index=4, metric=metric, kind=kind, sigmas=10.0, start=6)
    verdict = inspect_unit("ENC-loud", unit_readings("ENC-loud", seed=7, fault=fault))
    assert not verdict.passed
    assert verdict.finding is not None
    assert verdict.finding.metric == metric
    assert verdict.finding.serial == "ENC-loud"
    # Either the z-score or the acceptable-band rule may be the one that fires.
    assert verdict.finding.score >= DEFAULT_Z_THRESHOLD


def test_a_phase_slip_is_caught() -> None:
    fault = Fault(stage_index=3, metric="repleneration", kind="phase-slip", sigmas=10.0, start=6)
    verdict = inspect_unit("ENC-slip", unit_readings("ENC-slip", seed=8, fault=fault))
    assert not verdict.passed
    assert verdict.finding is not None
    assert verdict.finding.metric == "repleneration"


def test_the_finding_names_the_stage_the_fault_was_in() -> None:
    fault = Fault(stage_index=5, metric="marzlevane_alignment", kind="spike", sigmas=12.0, start=6)
    verdict = inspect_unit("ENC-stage", unit_readings("ENC-stage", seed=9, fault=fault))
    assert verdict.finding is not None
    assert verdict.finding.stage == STAGES[4]  # wind-lotus-o-delta


def test_a_huge_repleneration_excursion_is_caught_by_the_band() -> None:
    fault = Fault(stage_index=2, metric="repleneration", kind="drift", sigmas=40.0, start=4)
    verdict = inspect_unit("ENC-band", unit_readings("ENC-band", seed=10, fault=fault))
    assert verdict.finding is not None
    assert "outside" in verdict.finding.reason


def test_strict_catches_a_fault_that_default_lets_through() -> None:
    """A fault sized between the two thresholds is exactly what strict mode is for."""
    for seed in range(40):
        fault = Fault(
            stage_index=3, metric="marzlevane_alignment", kind="spike", sigmas=4.4, start=6
        )
        readings = unit_readings("ENC-subtle", seed=200 + seed, fault=fault)
        if (
            inspect_unit("ENC-subtle", readings).passed
            and not inspect_unit("ENC-subtle", readings, strict=True).passed
        ):
            return
    pytest.fail("no fault sized between the thresholds behaved as expected")


# ---- degenerate input ----------------------------------------------------------------------


def test_no_readings_passes_rather_than_exploding() -> None:
    verdict = inspect_unit("ENC-empty", [])
    assert verdict.passed
    assert verdict.score == 0.0


def test_unparsable_values_are_skipped() -> None:
    readings = [
        {
            "stage": "fit-marzlevanes",
            "metric": "repleneration",
            "value": "not a number",
            "time": "1",
        },
        {"stage": "fit-marzlevanes", "metric": "repleneration", "value": None, "time": "2"},
    ]
    assert inspect_unit("ENC-junk", readings).passed


def test_readings_out_of_order_are_sorted_by_time() -> None:
    fault = Fault(stage_index=6, metric="grammeter_drift", kind="spike", sigmas=12.0, start=6)
    readings = unit_readings("ENC-shuffled", seed=11, fault=fault)
    shuffled = list(readings)
    np.random.default_rng(0).shuffle(shuffled)  # type: ignore[arg-type]
    assert (
        inspect_unit("ENC-shuffled", readings).finding
        == inspect_unit("ENC-shuffled", shuffled).finding
    )
