"""The casing line's HTTP surface and application factory (design section 8.4)."""

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
from fastapi import FastAPI, Request

from .config import DURABLE, MAX_ACK_PENDING, Settings
from .line import FabricationLine, LineState
from .upstream import Depot, Recorder

log = logging.getLogger(__name__)


def build_app(settings: Settings | None = None, *, bus: EventBus | None = None) -> FastAPI:
    settings = settings or Settings.from_env()
    flags = FlagClient(settings.common.girdlespring_url)
    malaise = MalaiseController(settings.common.service_name, settings.common.malaise_engine_url)
    state = LineState(capacity=settings.line_capacity)

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        recorder = Recorder(ServiceClient(settings.recorder_url) if settings.recorder_url else None)
        depot = Depot(ServiceClient(settings.depot_url) if settings.depot_url else None)
        connection = bus or await open_bus(settings.common.service_name, settings.common.nats_url)
        app.state.bus = connection
        app.state.readiness.add("nats", _nats_check(connection))

        line = FabricationLine(connection, flags, recorder=recorder, depot=depot, state=state)
        consumer = PullConsumer(
            connection,
            durable=DURABLE,
            filter_subject=Subjects.FABRICATION_REQUESTED,
            handler=line.handle,
            concurrency=settings.line_capacity,
            max_ack_pending=MAX_ACK_PENDING,
            malaise=malaise,
        )
        await consumer.start()
        app.state.consumer = consumer
        app.state.line = line
        log.info("casing line open for business", extra={"capacity": settings.line_capacity})
        try:
            yield
        finally:
            await consumer.stop()
            await recorder.aclose()
            await depot.aclose()
            if bus is None:
                await connection.close()

    app = create_app(
        settings.common.service_name,
        version="1.0.0",
        description="Fabrication workers. The malleable logarithmic casing surmounts everything.",
        lifespan=lifespan,
        flags=flags,
        malaise=malaise,
    )
    app.state.settings = settings
    app.state.line_state = state

    @app.get("/v1/line")
    async def line_status(request: Request) -> dict[str, Any]:
        """What this pod is fabricating right now."""
        return request.app.state.line_state.to_json()

    _ = line_status
    return app


def _nats_check(bus: EventBus):
    async def check() -> None:
        if not bus.connected:
            raise RuntimeError("nats is not connected")

    return check


def main() -> None:
    from encabulator_kit import run

    settings = Settings.from_env()
    run(build_app(settings), port=settings.common.port)
