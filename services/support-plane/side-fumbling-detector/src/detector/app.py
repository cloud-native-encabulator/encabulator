"""The detector's HTTP surface and application factory (design section 8.11)."""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from encabulator_kit import (
    EventBus,
    FlagClient,
    MalaiseController,
    PullConsumer,
    ServiceClient,
    create_app,
    open_bus,
)
from encabulator_kit.messaging import Subjects
from fastapi import FastAPI, Query, Request

from .config import Settings
from .incidents import IncidentStore
from .inspection import Inspector
from .stats import InspectionStats

log = logging.getLogger(__name__)


def build_app(
    settings: Settings | None = None,
    *,
    bus: EventBus | None = None,
    valkey: Any = None,
) -> FastAPI:
    settings = settings or Settings.from_env()
    flags = FlagClient(settings.common.girdlespring_url)
    malaise = MalaiseController(settings.common.service_name, settings.common.malaise_engine_url)
    stats = InspectionStats()

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        client = valkey if valkey is not None else _open_valkey(settings.valkey_url)
        store = IncidentStore(client)
        recorder = ServiceClient(settings.recorder_url) if settings.recorder_url else None
        connection = bus or await open_bus(settings.common.service_name, settings.common.nats_url)

        app.state.bus = connection
        app.state.incidents = store
        app.state.readiness.add("nats", _nats_check(connection))
        if client is not None:
            app.state.readiness.add("valkey", _valkey_check(client))

        inspector = Inspector(connection, flags, recorder=recorder, incidents=store, stats=stats)
        consumer = PullConsumer(
            connection,
            durable=settings.common.service_name,
            filter_subject=Subjects.FABRICATION_COMPLETED,
            handler=inspector.handle,
            concurrency=settings.concurrency,
            malaise=malaise,
        )
        await consumer.start()
        app.state.inspector = inspector
        log.info("inspection bench staffed")
        try:
            yield
        finally:
            await consumer.stop()
            if recorder is not None:
                await recorder.aclose()
            if valkey is None and client is not None:
                await client.aclose()
            if bus is None:
                await connection.close()

    app = create_app(
        settings.common.service_name,
        version="1.0.0",
        description="So fitted that side fumbling is effectively prevented, or at least detected.",
        lifespan=lifespan,
        flags=flags,
        malaise=malaise,
    )
    app.state.settings = settings
    app.state.stats = stats

    @app.get("/v1/incidents")
    async def incidents(
        request: Request, limit: int = Query(default=50, ge=1, le=500)
    ) -> list[dict[str, Any]]:
        """The most recent units that fumbled, newest first."""
        return await request.app.state.incidents.recent(limit)

    @app.get("/v1/stats")
    async def inspection_stats(request: Request) -> dict[str, float | int]:
        return request.app.state.stats.to_json()

    _ = (incidents, inspection_stats)
    return app


def _open_valkey(url: str | None) -> Any:
    if not url:
        log.warning("VALKEY_URL is unset; incidents will not be recorded")
        return None
    from redis.asyncio import Redis

    return Redis.from_url(url)


def _nats_check(bus: EventBus):
    async def check() -> None:
        if not bus.connected:
            raise RuntimeError("nats is not connected")

    return check


def _valkey_check(client: Any):
    async def check() -> None:
        await client.ping()

    return check


def main() -> None:
    from encabulator_kit import run

    settings = Settings.from_env()
    run(build_app(settings), port=settings.common.port)
