from __future__ import annotations

from casing_line.domain import MODELS, STAGES, stage_by_index, stage_keys
from casing_line.serials import new_serial
from casing_line.timing import stage_duration_seconds


def test_seven_stages_in_order_with_the_design_durations() -> None:
    assert [stage.index for stage in STAGES] == [1, 2, 3, 4, 5, 6, 7]
    assert [stage.base_ms for stage in STAGES] == [300, 400, 500, 700, 600, 400, 300]
    assert stage_keys() == (
        "prelaminate-base-plate",
        "surmount-logarithmic-casing",
        "align-spurving-bearings",
        "fit-marzlevanes",
        "wind-lotus-o-delta",
        "connect-tremie-pipes",
        "synchronize-cardinal-grammeters",
    )
    assert stage_by_index(4).key == "fit-marzlevanes"


def test_models_match_the_design() -> None:
    assert MODELS["turbo"].stage_time_factor == 1.0
    assert MODELS["retro"].stage_time_factor == 1.3
    assert MODELS["hyper"].stage_time_factor == 1.8
    assert MODELS["turbo"].base_defect_rate == 0.04
    assert MODELS["retro"].base_defect_rate == 0.03
    assert MODELS["hyper"].base_defect_rate == 0.06


def test_total_base_time_is_3200ms() -> None:
    assert sum(stage.base_ms for stage in STAGES) == 3200


def test_stage_duration_applies_factor_turbo_and_jitter() -> None:
    import random

    stage = STAGES[3]  # 700 ms
    model = MODELS["hyper"]  # x1.8
    samples = [
        stage_duration_seconds(stage, model, turbo=False, rng=random.Random(seed))
        for seed in range(200)
    ]
    nominal = 0.700 * 1.8
    assert min(samples) >= nominal * 0.75 - 1e-9
    assert max(samples) <= nominal * 1.25 + 1e-9
    assert abs(sum(samples) / len(samples) - nominal) < nominal * 0.05

    turbo = [
        stage_duration_seconds(stage, model, turbo=True, rng=random.Random(seed))
        for seed in range(200)
    ]
    assert max(turbo) <= nominal * 0.5 * 1.25 + 1e-9


def test_serial_format() -> None:
    serial = new_serial("turbo", timestamp_ms=1_800_000_000_000)
    prefix, model, stamp, suffix = serial.split("-")
    assert prefix == "ENC"
    assert model == "turbo"
    assert set(stamp) <= set("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
    assert len(suffix) == 4
    assert all(char in "0123456789ABCDEF" for char in suffix)


def test_serials_are_unique() -> None:
    assert len({new_serial("retro") for _ in range(500)}) == 500
