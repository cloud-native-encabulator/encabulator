"""The orchestrator's HTTP surface and application factory (design section 8.14).

The orchestrator does not apply malaise to itself (design section 7), so no malaise middleware is
installed here.
"""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from encabulator_kit import create_app
from fastapi import FastAPI, Query, Request, Response

from .clients import Upstreams
from .config import Settings
from .loader import load_all
from .runner import Orchestrator

log = logging.getLogger(__name__)


def build_app(  # noqa: C901 - one nested function per route, read top down
    settings: Settings | None = None,
    *,
    upstreams: Upstreams | None = None,
    scenarios: dict[str, Any] | None = None,
) -> FastAPI:
    settings = settings or Settings.from_env()

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        catalog = scenarios if scenarios is not None else load_all(settings.scenario_dir)
        clients = upstreams if upstreams is not None else Upstreams.from_settings(settings)
        orchestrator = Orchestrator(catalog, clients, history=settings.run_history)
        app.state.orchestrator = orchestrator
        log.info(
            "scenarios loaded",
            extra={"scenarios": sorted(catalog), "scenario_dir": settings.scenario_dir},
        )
        try:
            yield
        finally:
            await orchestrator.shutdown()
            if upstreams is None:
                await clients.aclose()

    app = create_app(
        settings.common.service_name,
        version="1.0.0",
        description="Rehearses the demo so the presenter does not have to.",
        lifespan=lifespan,
    )
    app.state.settings = settings

    def orchestrator(request: Request) -> Orchestrator:
        return request.app.state.orchestrator

    @app.get("/v1/scenarios")
    async def list_scenarios(request: Request) -> list[dict[str, Any]]:
        """Every scenario the orchestrator knows: built-ins plus anything mounted."""
        catalog = orchestrator(request).scenarios
        return [catalog[key].to_json() for key in sorted(catalog)]

    @app.get("/v1/scenarios/{scenario_id}")
    async def get_scenario(scenario_id: str, request: Request) -> dict[str, Any]:
        return orchestrator(request).scenario(scenario_id).to_json()

    @app.post("/v1/scenarios/{scenario_id}/runs", status_code=202)
    async def start_run(scenario_id: str, request: Request) -> dict[str, Any]:
        """Start a run. One at a time: 409 while another is on the floor."""
        return orchestrator(request).start(scenario_id).to_json()

    @app.get("/v1/runs")
    async def list_runs(
        request: Request, limit: int = Query(default=20, ge=1, le=200)
    ) -> list[dict[str, Any]]:
        return [run.to_json(include_log=False) for run in orchestrator(request).runs(limit)]

    @app.get("/v1/runs/{run_id}")
    async def get_run(run_id: str, request: Request) -> dict[str, Any]:
        """Status, current step, and the full narration log."""
        return orchestrator(request).run(run_id).to_json()

    @app.post("/v1/runs/{run_id}/cancel")
    async def cancel_run(run_id: str, request: Request) -> dict[str, Any]:
        """Stop the run, put the flags back, and clear the malaises it created."""
        run = await orchestrator(request).cancel(run_id)
        return run.to_json()

    @app.get("/v1/narration")
    async def narration(request: Request, response: Response) -> dict[str, Any]:
        """The active run's narration, for the console's ticker."""
        active = orchestrator(request).active
        if active is None:
            response.headers["Cache-Control"] = "no-store"
            return {"active": False}
        return {"active": True, "run": active.to_json()}

    _ = (list_scenarios, get_scenario, start_run, list_runs, get_run, cancel_run, narration)
    return app


def main() -> None:
    from encabulator_kit import run

    settings = Settings.from_env()
    run(build_app(settings), port=settings.common.port)
