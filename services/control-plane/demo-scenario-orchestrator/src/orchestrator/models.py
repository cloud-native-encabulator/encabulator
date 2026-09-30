"""Scenarios, steps, and runs."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import StrEnum
from typing import Any

__all__ = ["LogEntry", "Run", "RunStatus", "Scenario", "Step", "StepKind"]


class StepKind(StrEnum):
    SAY = "say"
    TRAFFIC = "traffic"
    MALAISE = "malaise"
    CLEAR_MALAISE = "clearMalaise"
    FLAG = "flag"
    WAIT = "wait"
    WAIT_FOR = "waitFor"
    #: An addition to the design's list, so PREFABULATED_AMULITE_DEPOT_URL has something to do:
    #: pour fresh parts into the cage without waiting for the foundry's next tick.
    RESTOCK = "restock"


@dataclass(frozen=True, slots=True)
class Step:
    index: int  # 1-based
    kind: StepKind
    payload: Any
    summary: str

    def to_json(self) -> dict[str, Any]:
        return {"index": self.index, "kind": str(self.kind), "summary": self.summary}


@dataclass(frozen=True, slots=True)
class Scenario:
    id: str
    title: str
    description: str
    steps: tuple[Step, ...]
    source: str = "built-in"

    def to_json(self, *, include_steps: bool = True) -> dict[str, Any]:
        body: dict[str, Any] = {
            "id": self.id,
            "title": self.title,
            "description": self.description,
            "source": self.source,
            "stepCount": len(self.steps),
        }
        if include_steps:
            body["steps"] = [step.to_json() for step in self.steps]
        return body


class RunStatus(StrEnum):
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


def _iso(moment: datetime) -> str:
    return moment.astimezone(UTC).isoformat(timespec="milliseconds").replace("+00:00", "Z")


@dataclass(frozen=True, slots=True)
class LogEntry:
    time: datetime
    step: int
    kind: str
    level: str
    message: str

    def to_json(self) -> dict[str, Any]:
        return {
            "time": _iso(self.time),
            "step": self.step,
            "kind": self.kind,
            "level": self.level,
            "message": self.message,
        }


@dataclass(slots=True)
class Run:
    id: str
    scenario_id: str
    title: str
    total_steps: int
    status: RunStatus = RunStatus.RUNNING
    current_step: int = 0
    started_at: datetime = field(default_factory=lambda: datetime.now(UTC))
    finished_at: datetime | None = None
    log: list[LogEntry] = field(default_factory=list)
    #: Malaise ids this run created, so cancelling can clear exactly those.
    malaise_ids: dict[str, str] = field(default_factory=dict)
    #: Flags this run changed, and what they were, so they can be put back.
    flag_restores: dict[str, Any] = field(default_factory=dict)
    error: str | None = None

    @property
    def active(self) -> bool:
        return self.status is RunStatus.RUNNING

    def say(self, step: int, kind: str, message: str, level: str = "info") -> LogEntry:
        entry = LogEntry(datetime.now(UTC), step, kind, level, message)
        self.log.append(entry)
        return entry

    def to_json(self, *, include_log: bool = True) -> dict[str, Any]:
        body: dict[str, Any] = {
            "id": self.id,
            "scenarioId": self.scenario_id,
            "title": self.title,
            "status": str(self.status),
            "currentStep": self.current_step,
            "totalSteps": self.total_steps,
            "startedAt": _iso(self.started_at),
            "finishedAt": _iso(self.finished_at) if self.finished_at else None,
        }
        if self.error:
            body["error"] = self.error
        if include_log:
            body["log"] = [entry.to_json() for entry in self.log]
        return body
