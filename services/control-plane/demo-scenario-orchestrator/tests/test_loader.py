"""Scenario parsing, and the six built-ins the design requires."""

from __future__ import annotations

import pytest
import yaml

from orchestrator.loader import (
    ScenarioError,
    load_all,
    load_builtin,
    load_directory,
    parse_duration,
    parse_scenario,
)
from orchestrator.models import StepKind

BUILT_IN_IDS = {
    "morning-shift",
    "marzlevane-shortage",
    "hydrocoptic-outage",
    "side-fumbling-outbreak",
    "dingle-arm-congestion",
    "grand-tour",
}


def test_the_six_built_in_scenarios_are_present_and_valid() -> None:
    scenarios = {scenario.id: scenario for scenario in load_builtin()}
    assert set(scenarios) == BUILT_IN_IDS
    for scenario in scenarios.values():
        assert scenario.title
        assert scenario.description
        assert scenario.steps
        assert scenario.source == "built-in"


def test_every_built_in_narrates() -> None:
    for scenario in load_builtin():
        narration = [step for step in scenario.steps if step.kind is StepKind.SAY]
        assert len(narration) >= 4, f"{scenario.id} barely says anything"
        assert scenario.steps[0].kind is StepKind.SAY, f"{scenario.id} opens without narration"


def test_every_built_in_actually_exercises_the_system() -> None:
    """A scenario that only talks is a slide deck."""
    for scenario in load_builtin():
        kinds = {step.kind for step in scenario.steps}
        assert kinds & {
            StepKind.TRAFFIC,
            StepKind.MALAISE,
            StepKind.FLAG,
            StepKind.RESTOCK,
        }, f"{scenario.id} changes nothing"
        assert StepKind.WAIT_FOR in kinds, f"{scenario.id} never waits on a real metric"


def test_the_grand_tour_covers_the_other_five() -> None:
    tour = next(scenario for scenario in load_builtin() if scenario.id == "grand-tour")
    kinds = {step.kind for step in tour.steps}
    assert kinds >= {
        StepKind.SAY,
        StepKind.TRAFFIC,
        StepKind.MALAISE,
        StepKind.CLEAR_MALAISE,
        StepKind.FLAG,
        StepKind.WAIT,
        StepKind.WAIT_FOR,
        StepKind.RESTOCK,
    }
    assert len(tour.steps) > 40


def test_every_cleared_malaise_was_created_by_the_same_scenario() -> None:
    for scenario in load_builtin():
        created = {
            str(step.payload.get("ref") or f"step-{step.index}")
            for step in scenario.steps
            if step.kind is StepKind.MALAISE
        }
        for step in scenario.steps:
            if step.kind is StepKind.CLEAR_MALAISE and step.payload != "all":
                assert step.payload in created, f"{scenario.id} clears unknown {step.payload}"


def test_every_flag_touched_is_one_the_design_defines() -> None:
    known = {
        "turbo-encabulation",
        "quote-cache",
        "retro-encabulator-sales",
        "strict-side-fumbling",
        "amulite-foundry-rate",
        "gateway-rate-limit-rps",
        "dingle-arm-damping",
    }
    for scenario in load_builtin():
        for step in scenario.steps:
            if step.kind is StepKind.FLAG:
                assert step.payload["key"] in known, step.payload["key"]


def test_every_restocked_sku_is_in_the_catalog() -> None:
    catalog = {
        "AMU-BASE",
        "LOG-CASE",
        "SPV-BRG",
        "PNT-FAN",
        "HYD-MARZ",
        "LUN-WANE",
        "LOT-WIND",
        "TRM-PIPE",
        "GRD-SPRG",
        "DNG-ARM",
    }
    for scenario in load_builtin():
        for step in scenario.steps:
            if step.kind is StepKind.RESTOCK:
                assert step.payload["sku"] in catalog


def test_every_malaise_target_is_a_real_workload() -> None:
    services = {
        "encabulation-control-console",
        "cardinal-grammeter",
        "malaise-injection-engine",
        "demo-scenario-orchestrator",
        "dingle-arm-gateway",
        "panametric-fam-processor",
        "spurving-bearing-quoter",
        "logarithmic-casing-line",
        "nofer-trunnion-dispatch",
        "hydrocoptic-datastore",
        "prefabulated-amulite-depot",
        "sinusoidal-recorder",
        "unilateral-phase-detractor",
        "differential-girdlespring",
        "side-fumbling-detector",
        "postgres",
        "valkey",
        "nats",
        "influxdb",
    }
    for scenario in load_builtin():
        for step in scenario.steps:
            if step.kind is StepKind.MALAISE:
                assert step.payload["target"] in services, step.payload["target"]
                assert step.payload["kind"] in {
                    "latency",
                    "error",
                    "congestion",
                    "cpu-burn",
                    "pod-kill",
                    "partition",
                    "scale",
                }


# ---- durations ------------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("text", "seconds"),
    [("30s", 30.0), ("2m", 120.0), ("500ms", 0.5), ("1h", 3600.0), ("45", 45.0), (12, 12.0)],
)
def test_duration_parsing(text: object, seconds: float) -> None:
    assert parse_duration(text) == seconds


@pytest.mark.parametrize("bad", ["", "soon", "12 furlongs", True])
def test_bad_durations_are_rejected(bad: object) -> None:
    with pytest.raises(ScenarioError):
        parse_duration(bad)


# ---- validation -----------------------------------------------------------------------------


def _scenario(steps: list[dict]) -> dict:
    return {"id": "t", "title": "T", "description": "d", "steps": steps}


def test_a_step_must_name_exactly_one_kind() -> None:
    with pytest.raises(ScenarioError, match="exactly one"):
        parse_scenario(_scenario([{"say": "a", "wait": "1s"}]))
    with pytest.raises(ScenarioError, match="exactly one"):
        parse_scenario(_scenario([{"shout": "a"}]))


def test_required_fields() -> None:
    with pytest.raises(ScenarioError, match="needs id"):
        parse_scenario({"title": "T", "steps": [{"say": "a"}]})
    with pytest.raises(ScenarioError, match="needs title"):
        parse_scenario({"id": "t", "steps": [{"say": "a"}]})
    with pytest.raises(ScenarioError, match="needs steps"):
        parse_scenario({"id": "t", "title": "T"})
    with pytest.raises(ScenarioError, match="non-empty"):
        parse_scenario({"id": "t", "title": "T", "steps": {"not": "a list"}})


def test_step_specific_validation() -> None:
    with pytest.raises(ScenarioError, match="narration"):
        parse_scenario(_scenario([{"say": "   "}]))
    with pytest.raises(ScenarioError, match="malaise needs target"):
        parse_scenario(_scenario([{"malaise": {"kind": "latency"}}]))
    with pytest.raises(ScenarioError, match="flag needs key"):
        parse_scenario(_scenario([{"flag": {"key": "x"}}]))
    with pytest.raises(ScenarioError, match="waitFor needs promql"):
        parse_scenario(_scenario([{"waitFor": {"op": ">", "value": 1}}]))
    with pytest.raises(ScenarioError, match="op must be"):
        parse_scenario(_scenario([{"waitFor": {"promql": "x", "op": "~", "value": 1}}]))
    with pytest.raises(ScenarioError, match="restock needs a sku"):
        parse_scenario(_scenario([{"restock": {"quantity": 5}}]))


def test_wait_for_gets_defaults() -> None:
    scenario = parse_scenario(_scenario([{"waitFor": {"promql": "up", "op": ">", "value": 0}}]))
    payload = scenario.steps[0].payload
    assert payload["timeout"] == 120.0
    assert payload["interval"] == 2.0


def test_steps_are_numbered_from_one_and_summarised() -> None:
    scenario = parse_scenario(
        _scenario(
            [
                {"say": "hello"},
                {"flag": {"key": "turbo-encabulation", "value": True, "keep": True}},
                {"clearMalaise": "all"},
            ]
        )
    )
    assert [step.index for step in scenario.steps] == [1, 2, 3]
    assert scenario.steps[1].summary == "set flag turbo-encabulation = True (kept)"
    assert "every malaise" in scenario.steps[2].summary


def test_long_narration_is_truncated_in_the_summary() -> None:
    scenario = parse_scenario(_scenario([{"say": "x" * 200}]))
    assert len(scenario.steps[0].summary) == 80
    assert scenario.steps[0].summary.endswith("...")


# ---- mounted extras -------------------------------------------------------------------------


def test_mounted_scenarios_are_loaded_and_can_override_a_built_in(tmp_path) -> None:
    (tmp_path / "extra.yaml").write_text(
        yaml.safe_dump(_scenario([{"say": "a mounted scenario"}]) | {"id": "mounted"})
    )
    (tmp_path / "override.yaml").write_text(
        yaml.safe_dump(
            {
                "id": "morning-shift",
                "title": "Morning Shift (house rules)",
                "steps": [{"say": "different"}],
            }
        )
    )
    (tmp_path / "notes.txt").write_text("ignored")
    (tmp_path / ".hidden.yaml").write_text("ignored")

    mounted = {scenario.id: scenario for scenario in load_directory(tmp_path)}
    assert set(mounted) == {"mounted", "morning-shift"}
    assert mounted["mounted"].source.endswith("extra.yaml")

    combined = load_all(tmp_path)
    assert set(combined) == BUILT_IN_IDS | {"mounted"}
    assert combined["morning-shift"].title == "Morning Shift (house rules)"


def test_a_broken_mounted_scenario_is_skipped_not_fatal(tmp_path, caplog) -> None:
    (tmp_path / "bad.yaml").write_text("id: bad\ntitle: Bad\nsteps: [{shout: nope}]\n")
    (tmp_path / "worse.yaml").write_text("this: is: not: yaml:\n")
    (tmp_path / "good.yaml").write_text(yaml.safe_dump(_scenario([{"say": "fine"}])))
    loaded = load_directory(tmp_path)
    assert [scenario.id for scenario in loaded] == ["t"]
    assert "ignoring scenario" in caplog.text


def test_a_missing_scenario_directory_is_fine() -> None:
    assert load_directory("/no/such/place") == []
