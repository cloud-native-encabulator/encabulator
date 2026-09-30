"""Loading and validating scenario YAML.

Built-ins ship inside the image as package data. Extras are mounted from a ConfigMap at
``SCENARIO_DIR`` (``/etc/scenarios``) and may override a built-in by reusing its id, which is how a
presenter tweaks a scenario without rebuilding anything.
"""

from __future__ import annotations

import logging
from collections.abc import Iterable, Mapping
from pathlib import Path
from typing import Any

import yaml

from .models import Scenario, Step, StepKind

log = logging.getLogger(__name__)

__all__ = [
    "ScenarioError",
    "load_all",
    "load_builtin",
    "load_directory",
    "parse_duration",
    "parse_scenario",
]

_KNOWN_KINDS = {str(kind) for kind in StepKind}
_COMPARISONS = frozenset({">", ">=", "<", "<=", "==", "!="})


class ScenarioError(Exception):
    """A scenario file is not a scenario."""


def parse_duration(value: Any, *, field: str = "duration") -> float:
    """Accept ``"30s"``, ``"2m"``, ``"500ms"``, ``"1h"``, or a bare number of seconds."""
    if isinstance(value, bool):
        raise ScenarioError(f"{field} must be a duration, not a boolean")
    if isinstance(value, int | float):
        return float(value)
    text = str(value).strip().lower()
    if not text:
        raise ScenarioError(f"{field} is empty")
    for suffix, multiplier in (("ms", 0.001), ("s", 1.0), ("m", 60.0), ("h", 3600.0)):
        if text.endswith(suffix):
            head = text[: -len(suffix)].strip()
            try:
                return float(head) * multiplier
            except ValueError as exc:
                raise ScenarioError(f"{field} {value!r} is not a duration") from exc
    try:
        return float(text)
    except ValueError as exc:
        raise ScenarioError(f"{field} {value!r} is not a duration") from exc


def _summary(kind: StepKind, payload: Any) -> str:
    match kind:
        case StepKind.SAY:
            text = str(payload)
            return text if len(text) <= 80 else text[:77] + "..."
        case StepKind.TRAFFIC:
            parts = [f"{key}={value}" for key, value in sorted(payload.items())]
            return "set traffic " + ", ".join(parts)
        case StepKind.MALAISE:
            return f"inject {payload.get('kind')} into {payload.get('target')}"
        case StepKind.CLEAR_MALAISE:
            if payload == "all":
                return "clear every malaise this run created"
            return f"clear {payload}"
        case StepKind.FLAG:
            keep = " (kept)" if payload.get("keep") else ""
            return f"set flag {payload.get('key')} = {payload.get('value')}{keep}"
        case StepKind.WAIT:
            return f"wait {payload}s"
        case StepKind.RESTOCK:
            return f"restock {payload['quantity']} of {payload['sku']}"
        case StepKind.WAIT_FOR:
            return f"wait until {payload['promql']} {payload['op']} {payload['value']}"
    raise ScenarioError(f"unknown step kind {kind}")  # pragma: no cover - StrEnum is exhaustive


def _parse_step(index: int, raw: Any) -> Step:  # noqa: C901 - one branch per step kind
    if not isinstance(raw, Mapping):
        raise ScenarioError(f"step {index} must be a mapping, got {type(raw).__name__}")
    present = [key for key in raw if key in _KNOWN_KINDS]
    if len(present) != 1:
        found = present or "none"
        raise ScenarioError(
            f"step {index} must name exactly one of {sorted(_KNOWN_KINDS)}, found {found}"
        )
    kind = StepKind(present[0])
    payload = raw[present[0]]

    match kind:
        case StepKind.SAY:
            if not isinstance(payload, str) or not payload.strip():
                raise ScenarioError(f"step {index}: say needs some narration")
        case StepKind.TRAFFIC:
            if not isinstance(payload, Mapping) or not payload:
                raise ScenarioError(f"step {index}: traffic needs a body for PUT /v1/traffic")
            payload = dict(payload)
        case StepKind.MALAISE:
            if not isinstance(payload, Mapping):
                raise ScenarioError(f"step {index}: malaise needs a mapping")
            payload = dict(payload)
            for required in ("target", "kind"):
                if not payload.get(required):
                    raise ScenarioError(f"step {index}: malaise needs {required}")
        case StepKind.CLEAR_MALAISE:
            payload = str(payload)
        case StepKind.FLAG:
            if not isinstance(payload, Mapping) or "key" not in payload or "value" not in payload:
                raise ScenarioError(f"step {index}: flag needs key and value")
            payload = dict(payload)
        case StepKind.WAIT:
            payload = parse_duration(payload, field=f"step {index} wait")
        case StepKind.RESTOCK:
            if not isinstance(payload, Mapping) or "sku" not in payload:
                raise ScenarioError(f"step {index}: restock needs a sku")
            payload = dict(payload)
            payload["quantity"] = int(payload.get("quantity", 1000))
        case StepKind.WAIT_FOR:
            if not isinstance(payload, Mapping):
                raise ScenarioError(f"step {index}: waitFor needs a mapping")
            payload = dict(payload)
            for required in ("promql", "op", "value"):
                if required not in payload:
                    raise ScenarioError(f"step {index}: waitFor needs {required}")
            if payload["op"] not in _COMPARISONS:
                raise ScenarioError(
                    f"step {index}: waitFor op must be one of {sorted(_COMPARISONS)}"
                )
            payload["value"] = float(payload["value"])
            payload["timeout"] = parse_duration(
                payload.get("timeout", "120s"), field=f"step {index} waitFor timeout"
            )
            payload["interval"] = parse_duration(
                payload.get("interval", "2s"), field=f"step {index} waitFor interval"
            )

    return Step(index=index, kind=kind, payload=payload, summary=_summary(kind, payload))


def parse_scenario(document: Any, *, source: str = "built-in") -> Scenario:
    if not isinstance(document, Mapping):
        raise ScenarioError("a scenario must be a mapping with id, title, and steps")
    for required in ("id", "title", "steps"):
        if not document.get(required):
            raise ScenarioError(f"a scenario needs {required}")
    steps = document["steps"]
    if not isinstance(steps, list) or not steps:
        raise ScenarioError("steps must be a non-empty list")
    return Scenario(
        id=str(document["id"]),
        title=str(document["title"]),
        description=str(document.get("description") or ""),
        steps=tuple(_parse_step(index, raw) for index, raw in enumerate(steps, start=1)),
        source=source,
    )


def load_builtin() -> list[Scenario]:
    """The scenarios baked into the image."""
    from importlib.resources import files

    scenarios: list[Scenario] = []
    root = files(__package__).joinpath("scenarios")
    for entry in sorted(root.iterdir(), key=lambda item: item.name):
        if not entry.name.endswith((".yaml", ".yml")):
            continue
        scenarios.append(
            parse_scenario(yaml.safe_load(entry.read_text(encoding="utf-8")), source="built-in")
        )
    return scenarios


def load_directory(directory: str | Path) -> list[Scenario]:
    """Extra scenarios mounted from a ConfigMap. A bad file is skipped, loudly."""
    path = Path(directory)
    if not path.is_dir():
        return []
    scenarios: list[Scenario] = []
    for entry in sorted(path.iterdir()):
        if entry.name.startswith(".") or entry.suffix not in {".yaml", ".yml"}:
            continue
        try:
            scenarios.append(
                parse_scenario(yaml.safe_load(entry.read_text(encoding="utf-8")), source=str(entry))
            )
        except (ScenarioError, yaml.YAMLError) as exc:
            log.error("ignoring scenario %s: %s", entry, exc)
    return scenarios


def load_all(directory: str | Path | None = None) -> dict[str, Scenario]:
    """Built-ins, then mounted extras, which may override a built-in by reusing its id."""
    scenarios: dict[str, Scenario] = {}
    for scenario in _chain(load_builtin(), load_directory(directory) if directory else []):
        scenarios[scenario.id] = scenario
    return scenarios


def _chain(*groups: Iterable[Scenario]) -> Iterable[Scenario]:
    for group in groups:
        yield from group
