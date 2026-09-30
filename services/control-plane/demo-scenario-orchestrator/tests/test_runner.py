"""The runner: step execution, one-run-at-a-time, cancellation, and restoration."""

from __future__ import annotations

import asyncio

import pytest
from encabulator_kit import EncabulatorError

from orchestrator.loader import parse_scenario
from orchestrator.models import RunStatus
from orchestrator.runner import Orchestrator

from .fakes import FakeUpstreams


def _catalog(*scenarios):
    return {scenario.id: scenario for scenario in scenarios}


def _scenario(steps, scenario_id="t", title="Test Scenario"):
    return parse_scenario({"id": scenario_id, "title": title, "steps": steps})


async def _finish(orchestrator: Orchestrator, run_id: str, timeout: float = 5.0) -> None:
    loop = asyncio.get_running_loop()
    deadline = loop.time() + timeout
    while loop.time() < deadline:
        if not orchestrator.run(run_id).active:
            return
        await asyncio.sleep(0.01)
    raise AssertionError("run did not finish")


async def test_a_run_executes_every_step_and_narrates() -> None:
    upstreams = FakeUpstreams()
    scenario = _scenario(
        [
            {"say": "The works is cold."},
            {"traffic": {"running": True, "rps": 5, "pattern": "steady"}},
            {"restock": {"sku": "HYD-MARZ", "quantity": 600}},
            {"say": "The works is warm."},
        ]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)

    assert run.status is RunStatus.COMPLETED
    assert run.current_step == 4
    assert run.total_steps == 4
    assert upstreams.traffic_calls == [{"running": True, "rps": 5, "pattern": "steady"}]
    assert upstreams.restocks == [("HYD-MARZ", 600)]
    narration = [entry.message for entry in run.log if entry.kind == "say"]
    assert narration == ["The works is cold.", "The works is warm."]
    assert run.finished_at is not None


async def test_only_one_run_at_a_time() -> None:
    scenario = _scenario([{"wait": "5s"}])
    orchestrator = Orchestrator(_catalog(scenario), FakeUpstreams())
    first = orchestrator.start("t")
    with pytest.raises(EncabulatorError) as caught:
        orchestrator.start("t")
    assert caught.value.status_code == 409
    assert caught.value.code == "run_in_progress"
    await orchestrator.cancel(first.id)
    # With the floor clear, another run may start.
    assert orchestrator.start("t").id != first.id
    await orchestrator.shutdown()


async def test_an_unknown_scenario_is_404() -> None:
    orchestrator = Orchestrator({}, FakeUpstreams())
    with pytest.raises(EncabulatorError) as caught:
        orchestrator.start("nope")
    assert caught.value.status_code == 404


async def test_flags_are_restored_at_the_end_of_a_run() -> None:
    upstreams = FakeUpstreams(flags={"strict-side-fumbling": False})
    scenario = _scenario([{"flag": {"key": "strict-side-fumbling", "value": True}}])
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert upstreams.flag_writes == [
        ("strict-side-fumbling", True),
        ("strict-side-fumbling", False),
    ]
    assert upstreams.flags["strict-side-fumbling"] is False


async def test_a_kept_flag_is_left_alone() -> None:
    upstreams = FakeUpstreams(flags={"turbo-encabulation": False})
    scenario = _scenario([{"flag": {"key": "turbo-encabulation", "value": True, "keep": True}}])
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert upstreams.flag_writes == [("turbo-encabulation", True)]
    assert upstreams.flags["turbo-encabulation"] is True


async def test_malaises_are_cleared_by_reference() -> None:
    upstreams = FakeUpstreams()
    scenario = _scenario(
        [
            {
                "malaise": {
                    "ref": "slow-line",
                    "target": "logarithmic-casing-line",
                    "kind": "latency",
                    "params": {"ms": 500},
                }
            },
            {
                "malaise": {
                    "ref": "busy-gate",
                    "target": "dingle-arm-gateway",
                    "kind": "congestion",
                    "params": {"maxConcurrent": 2},
                }
            },
            {"clearMalaise": "slow-line"},
        ]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    # slow-line cleared by the step, busy-gate swept up by the restore.
    assert upstreams.cleared == ["mal_1", "mal_2"]
    assert run.malaise_ids == {}


async def test_clear_malaise_all_clears_only_what_this_run_created() -> None:
    upstreams = FakeUpstreams()
    scenario = _scenario(
        [
            {
                "malaise": {
                    "ref": "a",
                    "target": "postgres",
                    "kind": "pod-kill",
                    "params": {"count": 1},
                }
            },
            {"clearMalaise": "all"},
        ]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert upstreams.cleared == ["mal_1"]


async def test_clearing_an_unknown_reference_warns_rather_than_fails() -> None:
    upstreams = FakeUpstreams()
    scenario = _scenario([{"clearMalaise": "never-created"}])
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert run.status is RunStatus.COMPLETED
    assert any(entry.level == "warn" for entry in run.log)


async def test_the_malaise_body_omits_orchestrator_only_fields() -> None:
    upstreams = FakeUpstreams()
    scenario = _scenario(
        [
            {
                "malaise": {
                    "ref": "a",
                    "narration": "custom words",
                    "target": "valkey",
                    "kind": "latency",
                    "params": {"ms": 10},
                    "durationSeconds": 30,
                }
            }
        ]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert upstreams.malaises == [
        {"target": "valkey", "kind": "latency", "params": {"ms": 10}, "durationSeconds": 30}
    ]
    assert any(entry.message == "custom words" for entry in run.log)


async def test_cancelling_reverts_flags_and_clears_malaises() -> None:
    upstreams = FakeUpstreams(flags={"amulite-foundry-rate": 20})
    scenario = _scenario(
        [
            {"flag": {"key": "amulite-foundry-rate", "value": 0}},
            {
                "malaise": {
                    "ref": "slow",
                    "target": "spurving-bearing-quoter",
                    "kind": "latency",
                    "params": {"ms": 900},
                }
            },
            {"wait": "30s"},
            {"say": "never reached"},
        ]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    # Let it get as far as the wait.
    for _ in range(200):
        if run.current_step >= 3:
            break
        await asyncio.sleep(0.01)
    cancelled = await orchestrator.cancel(run.id)

    assert cancelled.status is RunStatus.CANCELLED
    assert upstreams.flag_writes == [("amulite-foundry-rate", 0), ("amulite-foundry-rate", 20)]
    assert upstreams.cleared == ["mal_1"]
    assert "never reached" not in [entry.message for entry in run.log]
    assert cancelled.finished_at is not None


async def test_cancelling_a_finished_run_is_a_no_op() -> None:
    scenario = _scenario([{"say": "done"}])
    orchestrator = Orchestrator(_catalog(scenario), FakeUpstreams())
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert (await orchestrator.cancel(run.id)).status is RunStatus.COMPLETED


async def test_a_failing_step_fails_the_run_and_still_restores() -> None:
    upstreams = FakeUpstreams(flags={"quote-cache": True}, fail_on="traffic")
    scenario = _scenario(
        [
            {"flag": {"key": "quote-cache", "value": False}},
            {"traffic": {"rps": 5}},
            {"say": "never reached"},
        ]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert run.status is RunStatus.FAILED
    assert run.error is not None
    assert "not listening" in run.error
    assert upstreams.flag_writes[-1] == ("quote-cache", True)


# ---- waitFor -------------------------------------------------------------------------------


async def test_wait_for_returns_when_the_condition_holds() -> None:
    upstreams = FakeUpstreams(samples={"up": [0.0, 0.0, 5.0]})
    scenario = _scenario(
        [{"waitFor": {"promql": "up", "op": ">", "value": 1, "timeout": "5s", "interval": "0.01s"}}]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert run.status is RunStatus.COMPLETED
    assert len(upstreams.queries) == 3
    assert any("Condition met" in entry.message for entry in run.log)


async def test_wait_for_gives_up_and_carries_on() -> None:
    upstreams = FakeUpstreams(samples={"up": [0.0]})
    scenario = _scenario(
        [
            {
                "waitFor": {
                    "promql": "up",
                    "op": ">",
                    "value": 1,
                    "timeout": "0.05s",
                    "interval": "0.01s",
                }
            },
            {"say": "still got here"},
        ]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert run.status is RunStatus.COMPLETED
    assert any("Gave up" in entry.message and entry.level == "warn" for entry in run.log)
    assert any(entry.message == "still got here" for entry in run.log)


async def test_wait_for_is_skipped_without_prometheus() -> None:
    upstreams = FakeUpstreams(prometheus_enabled=False)
    scenario = _scenario([{"waitFor": {"promql": "up", "op": ">", "value": 1, "timeout": "60s"}}])
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    assert run.status is RunStatus.COMPLETED
    assert upstreams.queries == []
    assert any("PROMETHEUS_URL is unset" in entry.message for entry in run.log)


@pytest.mark.parametrize(
    ("op", "value", "sample", "met"),
    [
        (">", 1, 2.0, True),
        (">", 1, 0.5, False),
        ("<", 1, 0.5, True),
        (">=", 1, 1.0, True),
        ("<=", 1, 1.0, True),
        ("==", 1, 1.0, True),
        ("!=", 1, 1.0, False),
    ],
)
async def test_wait_for_comparisons(op: str, value: float, sample: float, met: bool) -> None:
    upstreams = FakeUpstreams(samples={"x": [sample]})
    scenario = _scenario(
        [
            {
                "waitFor": {
                    "promql": "x",
                    "op": op,
                    "value": value,
                    "timeout": "0.05s",
                    "interval": "0.01s",
                }
            }
        ]
    )
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    await _finish(orchestrator, run.id)
    messages = " ".join(entry.message for entry in run.log)
    assert ("Condition met" in messages) is met


# ---- history -------------------------------------------------------------------------------


async def test_run_history_is_newest_first_and_bounded() -> None:
    scenario = _scenario([{"say": "quick"}])
    orchestrator = Orchestrator(_catalog(scenario), FakeUpstreams(), history=3)
    ids = []
    for _ in range(5):
        run = orchestrator.start("t")
        ids.append(run.id)
        await _finish(orchestrator, run.id)
    assert [run.id for run in orchestrator.runs()] == list(reversed(ids[-3:]))
    with pytest.raises(EncabulatorError):
        orchestrator.run(ids[0])


async def test_shutdown_cancels_the_active_run() -> None:
    upstreams = FakeUpstreams(flags={"turbo-encabulation": False})
    scenario = _scenario([{"flag": {"key": "turbo-encabulation", "value": True}}, {"wait": "30s"}])
    orchestrator = Orchestrator(_catalog(scenario), upstreams)
    run = orchestrator.start("t")
    for _ in range(200):
        if run.current_step >= 2:
            break
        await asyncio.sleep(0.01)
    await orchestrator.shutdown()
    assert orchestrator.run(run.id).status is RunStatus.CANCELLED
    assert upstreams.flags["turbo-encabulation"] is False
