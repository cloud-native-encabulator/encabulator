"""The bits of the domain the casing line needs: models and fabrication stages.

Design section 3. The line owns no catalog of its own; these are the numbers from the contract.
"""

from __future__ import annotations

from dataclasses import dataclass

__all__ = ["MODELS", "STAGES", "Model", "Stage", "stage_by_index", "stage_keys"]


@dataclass(frozen=True, slots=True)
class Model:
    key: str
    name: str
    stage_time_factor: float
    base_defect_rate: float


MODELS: dict[str, Model] = {
    "turbo": Model("turbo", "Turbo Encabulator (1944)", 1.0, 0.04),
    "retro": Model("retro", "Retro Encabulator (Rockwell)", 1.3, 0.03),
    "hyper": Model("hyper", "HyperEncabulator (SANS ICS)", 1.8, 0.06),
}


@dataclass(frozen=True, slots=True)
class Stage:
    index: int
    key: str
    description: str
    base_ms: int


STAGES: tuple[Stage, ...] = (
    Stage(1, "prelaminate-base-plate", "Prelaminate the amulite base-plate", 300),
    Stage(2, "surmount-logarithmic-casing", "Surmount the malleable logarithmic casing", 400),
    Stage(
        3,
        "align-spurving-bearings",
        "Align both spurving bearings with the pentametric fan",
        500,
    ),
    Stage(
        4,
        "fit-marzlevanes",
        "Fit six hydrocoptic marzlevanes to the ambifacient lunar waneshaft",
        700,
    ),
    Stage(
        5,
        "wind-lotus-o-delta",
        "Wind the lotus-o-delta main winding into the panendermic semi-boloid slots",
        600,
    ),
    Stage(
        6,
        "connect-tremie-pipes",
        "Connect every seventh conductor by non-reversible tremie pipe",
        400,
    ),
    Stage(7, "synchronize-cardinal-grammeters", "Synchronize the cardinal grammeters", 300),
)


def stage_by_index(index: int) -> Stage:
    return STAGES[index - 1]


def stage_keys() -> tuple[str, ...]:
    return tuple(stage.key for stage in STAGES)
