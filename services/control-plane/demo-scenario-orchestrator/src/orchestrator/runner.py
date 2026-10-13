"""The scenario runner.

Runs execute as asyncio tasks, never inside a request (design section 8.14). One run at a time:
two scenarios fighting over the same flags and the same traffic generator would tell the audience
nothing. Whatever a run changed, it puts back -- flags to their previous values unless the step
said ``keep: true``, and any malaise it created cleared -- whether it finished, failed, or was
cancelled.
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
from collections import OrderedDict
from typing import Any

from encabulator_kit import EncabulatorError, UpstreamError, new_id
from prometheus_client import Counter, Gauge

from .clients import COMPARISONS, Upstreams
from .models import LogEntry, Run, RunStatus, Scenario, Step, StepKind

log = logging.getLogger(__name__)

RUNS = Counter("encabulator_scenario_runs", "Scenario runs started.", ["scenario", "result"])
ACTIVE = Gauge("encabulator_scenario_active", "1 while a scenario run is in flight.")
STEPS = Counter("encabulator_scenario_steps", "Scenario steps executed.", ["kind"])


class Orchestrator:
    """Holds the scenario catalog, the run history, and the one active run."""

    def __init__(self, scenarios: dict[str, Scenario], upstreams: Upstreams, *, history: int = 50):
        self.scenarios = scenarios
        self.upstreams = upstreams
        self.history = history
        self._runs: OrderedDict[str, Run] = OrderedDict()
        self._task: asyncio.Task[None] | None = None
        self._restore_task: asyncio.Task[None] | None = None
        self._active_id: str | None = None

    # ---- catalog and history -----------------------------------------------------------

    def scenario(self, scenario_id: str) -> Scenario:
        scenario = self.scenarios.get(scenario_id)
        if scenario is None:
            raise EncabulatorError(404, "scenario_not_found", f"No scenario {scenario_id}")
        return scenario

    def runs(self, limit: int = 20) -> list[Run]:
        return list(reversed(list(self._runs.values())))[:limit]

    def run(self, run_id: str) -> Run:
        found = self._runs.get(run_id)
        if found is None:
            raise EncabulatorError(404, "run_not_found", f"No run {run_id}")
        return found

    @property
    def active(self) -> Run | None:
        if self._active_id is None:
            return None
        run = self._runs.get(self._active_id)
        return run if run is not None and run.active else None

    # ---- starting and stopping ---------------------------------------------------------

    def start(self, scenario_id: str) -> Run:
        scenario = self.scenario(scenario_id)
        if self.active is not None:
            raise EncabulatorError(
                409,
                "run_in_progress",
                f"{self.active.title} is already on the floor; cancel it first",
            )
        run = Run(
            id=new_id("run"),
            scenario_id=scenario.id,
            title=scenario.title,
            total_steps=len(scenario.steps),
        )
        self._remember(run)
        self._active_id = run.id
        self._task = asyncio.create_task(self._execute(scenario, run), name=f"run-{run.id}")
        RUNS.labels(scenario.id, "started").inc()
        return run

    async def cancel(self, run_id: str) -> Run:
        run = self.run(run_id)
        if not run.active:
            return run
        if self._task is not None:
            self._task.cancel()
            with contextlib.suppress(asyncio.CancelledError, Exception):
                await self._task
        # The restore runs shielded inside the task; make sure it has finished before we claim
        # the works is back to normal.
        if self._restore_task is not None:
            with contextlib.suppress(asyncio.CancelledError, Exception):
                await self._restore_task
        if run.active:
            # Cancelled before the task ever got a turn, so its own cleanup never ran.
            await self._finalise_unstarted(run)
        return self.run(run_id)

    async def _finalise_unstarted(self, run: Run) -> None:
        run.status = RunStatus.CANCELLED
        run.say(run.current_step, "run", "Cancelled before the first step.", level="warn")
        RUNS.labels(run.scenario_id, "cancelled").inc()
        await self._restore(run)
        run.finished_at = _now()
        ACTIVE.set(0)
        if self._active_id == run.id:
            self._active_id = None

    async def shutdown(self) -> None:
        active = self.active
        if active is not None:
            await self.cancel(active.id)

    def _remember(self, run: Run) -> None:
        self._runs[run.id] = run
        while len(self._runs) > self.history:
            self._runs.popitem(last=False)

    # ---- execution ---------------------------------------------------------------------

    async def _execute(self, scenario: Scenario, run: Run) -> None:
        ACTIVE.set(1)
        run.say(0, "run", f"{scenario.title}: the floor is yours.")
        try:
            await self._record_traffic(run)
            # The reference of a malaise act being skipped, until its clearMalaise.
            skipping: str | None = None
            for step in scenario.steps:
                run.current_step = step.index
                if skipping is not None:
                    if step.kind is StepKind.CLEAR_MALAISE and str(step.payload) in {
                        skipping,
                        "all",
                    }:
                        if str(step.payload) == "all":
                            # Still lift every other malaise this run created.
                            await self._run_step(run, step)
                        skipping = None
                    continue
                await self._run_step(run, step)
                STEPS.labels(str(step.kind)).inc()
                if step.kind is StepKind.MALAISE and _reference(step) in run.skipped_malaises:
                    skipping = _reference(step)
            run.status = RunStatus.COMPLETED
            run.say(run.current_step, "run", "Scenario complete. The encabulator is encabulating.")
            RUNS.labels(scenario.id, "completed").inc()
        except asyncio.CancelledError:
            run.status = RunStatus.CANCELLED
            run.say(run.current_step, "run", "Cancelled by the operator.", level="warn")
            RUNS.labels(scenario.id, "cancelled").inc()
            raise
        except Exception as exc:
            run.status = RunStatus.FAILED
            run.error = f"{type(exc).__name__}: {exc}"
            run.say(run.current_step, "run", f"Scenario failed: {run.error}", level="error")
            RUNS.labels(scenario.id, "failed").inc()
            log.error("scenario run failed", exc_info=True, extra={"run_id": run.id})
        finally:
            # Shielded so the cancellation that got us here cannot interrupt the cleanup.
            self._restore_task = asyncio.create_task(self._restore(run), name=f"restore-{run.id}")
            with contextlib.suppress(asyncio.CancelledError):
                await asyncio.shield(self._restore_task)
            run.finished_at = _now()
            ACTIVE.set(0)
            if self._active_id == run.id:
                self._active_id = None

    async def _run_step(self, run: Run, step: Step) -> None:
        match step.kind:
            case StepKind.SAY:
                run.say(step.index, "say", str(step.payload))
            case StepKind.WAIT:
                run.say(step.index, "wait", f"Letting it run for {step.payload:g}s.")
                await asyncio.sleep(float(step.payload))
            case StepKind.TRAFFIC:
                await self._traffic(run, step)
            case StepKind.MALAISE:
                await self._malaise(run, step)
            case StepKind.CLEAR_MALAISE:
                await self._clear_malaise(run, step)
            case StepKind.FLAG:
                await self._flag(run, step)
            case StepKind.WAIT_FOR:
                await self._wait_for(run, step)
            case StepKind.RESTOCK:
                await self._restock(run, step)

    async def _traffic(self, run: Run, step: Step) -> None:
        body = dict(step.payload)
        run.traffic_changed = True
        await self.upstreams.set_traffic(body)
        detail = step.summary.removeprefix("set traffic ")
        run.say(step.index, "traffic", f"Grammeters set: {detail}.")

    async def _record_traffic(self, run: Run) -> None:
        """Remember the grammeter settings, so the run can leave the traffic as it found it."""
        try:
            run.traffic_restore = await self.upstreams.get_traffic()
        except Exception as exc:
            log.warning("could not read the grammeter settings", extra={"run_id": run.id})
            run.say(
                0,
                "traffic",
                f"Could not read the grammeters, so they will not be put back: {exc}",
                level="warn",
            )

    async def _malaise(self, run: Run, step: Step) -> None:
        body = {
            key: value for key, value in step.payload.items() if key not in {"ref", "narration"}
        }
        reference = _reference(step)
        try:
            created = await self.upstreams.inject_malaise(body)
        except UpstreamError as exc:
            store = MANAGED_STORES.get(str(body.get("target", "")))
            if store is None or exc.status_code != 400 or exc.code != "unknown_target":
                raise
            # On a cluster whose PostgreSQL or Valkey is a managed AWS service there is no store
            # pod for the malaise engine to delete. Say so plainly, skip the act, carry on.
            run.skipped_malaises.add(reference)
            run.say(
                step.index,
                "malaise",
                f"{store} runs here as a managed AWS service, so there is no pod to delete. "
                "We skip this act and carry on with the tour.",
                level="warn",
            )
            return
        if created and created.get("id"):
            run.malaise_ids[reference] = str(created["id"])
        run.say(
            step.index,
            "malaise",
            str(step.payload.get("narration") or f"Inducing malaise: {step.summary}."),
        )

    async def _clear_malaise(self, run: Run, step: Step) -> None:
        target = str(step.payload)
        if target in run.skipped_malaises:
            # The act was skipped; there is nothing to lift.
            return
        if target == "all":
            for malaise_id in list(run.malaise_ids.values()):
                await self.upstreams.clear_malaise(malaise_id)
            run.malaise_ids.clear()
            run.say(step.index, "clearMalaise", "Malaise lifted. All of it.")
            return
        malaise_id = run.malaise_ids.pop(target, None)
        if malaise_id is None:
            run.say(
                step.index,
                "clearMalaise",
                f"No malaise called {target} to clear.",
                level="warn",
            )
            return
        await self.upstreams.clear_malaise(malaise_id)
        run.say(step.index, "clearMalaise", f"Malaise {target} lifted.")

    async def _restock(self, run: Run, step: Step) -> None:
        sku = str(step.payload["sku"])
        quantity = int(step.payload["quantity"])
        await self.upstreams.restock(sku, quantity)
        run.say(step.index, "restock", f"Fresh castings in the cage: {quantity} x {sku}.")

    async def _flag(self, run: Run, step: Step) -> None:
        key = str(step.payload["key"])
        value = step.payload["value"]
        if not step.payload.get("keep") and key not in run.flag_restores:
            current = await self.upstreams.get_flag(key)
            if current is not None and "value" in current:
                run.flag_restores[key] = current["value"]
        await self.upstreams.set_flag(
            key,
            value,
            rolloutPercent=step.payload.get("rolloutPercent"),
            description=step.payload.get("description"),
        )
        run.say(step.index, "flag", f"Girdlespring adjusted: {key} = {value}.")

    async def _wait_for(self, run: Run, step: Step) -> None:
        promql = str(step.payload["promql"])
        comparison = COMPARISONS[str(step.payload["op"])]
        target = float(step.payload["value"])
        timeout = float(step.payload["timeout"])
        interval = float(step.payload["interval"])

        if self.upstreams.prometheus is None:
            run.say(
                step.index,
                "waitFor",
                "PROMETHEUS_URL is unset; not waiting for the metric.",
                level="warn",
            )
            return

        run.say(
            step.index,
            "waitFor",
            f"Watching the gauges until {promql} {step.payload['op']} {target:g}.",
        )
        loop = asyncio.get_running_loop()
        deadline = loop.time() + timeout
        last: float | None = None
        while loop.time() < deadline:
            try:
                last = await self.upstreams.query(promql)
            except Exception as exc:  # Prometheus may be absent, slow, or cross
                run.say(step.index, "waitFor", f"Prometheus said no: {exc}", level="warn")
                last = None
            if last is not None and comparison(last, target):
                run.say(step.index, "waitFor", f"Condition met: {promql} = {last:.4g}.")
                return
            await asyncio.sleep(interval)

        observed = "no data" if last is None else f"{last:.4g}"
        run.say(
            step.index,
            "waitFor",
            f"Gave up after {timeout:g}s; {promql} was {observed}. Carrying on.",
            level="warn",
        )

    async def _restore(self, run: Run) -> None:
        """Put the works back: flags to their previous values, our malaises cleared, traffic
        to the settings the run started with."""
        for key, value in list(run.flag_restores.items()):
            try:
                await self.upstreams.set_flag(key, value)
                run.say(run.current_step, "flag", f"Girdlespring restored: {key} = {value}.")
            except Exception as exc:
                run.say(
                    run.current_step,
                    "flag",
                    f"Could not restore {key}: {exc}",
                    level="error",
                )
        run.flag_restores.clear()

        for reference, malaise_id in list(run.malaise_ids.items()):
            try:
                await self.upstreams.clear_malaise(malaise_id)
                run.say(run.current_step, "clearMalaise", f"Malaise {reference} cleared.")
            except Exception as exc:
                run.say(
                    run.current_step,
                    "clearMalaise",
                    f"Could not clear {reference}: {exc}",
                    level="error",
                )
        run.malaise_ids.clear()

        if run.traffic_changed and run.traffic_restore:
            restore = dict(run.traffic_restore)
            try:
                await self.upstreams.set_traffic(restore)
                shown = ", ".join(
                    f"{key}={restore[key]}"
                    for key in ("rps", "pattern", "running")
                    if key in restore
                )
                run.say(run.current_step, "traffic", f"Grammeters restored: {shown}.")
            except Exception as exc:
                run.say(
                    run.current_step,
                    "traffic",
                    f"Could not restore the grammeters: {exc}",
                    level="error",
                )
        run.traffic_changed = False


def _reference(step: Step) -> str:
    """The name a malaise step is cleared by: its ref, or its position."""
    return str(step.payload.get("ref") or f"step-{step.index}")


#: Store targets that may be a managed AWS service (RDS, ElastiCache) with no pod to delete.
MANAGED_STORES = {"postgres": "PostgreSQL", "valkey": "Valkey"}


def _now() -> Any:
    from datetime import UTC, datetime

    return datetime.now(UTC)


__all__ = ["LogEntry", "Orchestrator"]
