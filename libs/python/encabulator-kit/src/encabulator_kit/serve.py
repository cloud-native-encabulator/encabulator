"""uvicorn entrypoint with the graceful shutdown sequence from design section 7.

On SIGTERM: fail readiness first, pause long enough for the endpoints controller to take this pod
out of rotation, then stop accepting and drain in-flight requests for up to 20 seconds.

uvicorn installs its own ``signal.signal`` handlers inside ``Server.serve()`` and points them at
``Server.handle_exit``, which sets ``should_exit`` at once -- no window in which to fail readiness
first. So the kit replaces ``handle_exit``: ours flips readiness immediately and hands the actual
stop to the event loop a few seconds later. Replacing the hook rather than installing a competing
``loop.add_signal_handler`` also avoids uvicorn's signal re-raise delivering the same SIGTERM twice.

One uvicorn worker per process; scale with replicas, not with workers, so that per-pod state
(rate-limit buckets, line capacity, run history) stays per-pod and legible.
"""

from __future__ import annotations

import asyncio
import logging
from collections.abc import Callable
from typing import Any

import uvicorn
from fastapi import FastAPI

from .config import env_float, env_int

__all__ = ["build_config", "install_shutdown_hook", "run", "serve"]

log = logging.getLogger(__name__)

DEFAULT_DRAIN_SECONDS = 20.0
DEFAULT_READINESS_GRACE_SECONDS = 3.0


def build_config(
    app: FastAPI,
    *,
    host: str = "0.0.0.0",
    port: int | None = None,
    drain_seconds: float = DEFAULT_DRAIN_SECONDS,
) -> uvicorn.Config:
    return uvicorn.Config(
        app,
        host=host,
        port=port if port is not None else env_int("PORT", 8080),
        log_config=None,  # the kit owns logging
        access_log=False,  # the kit emits the request histogram instead
        timeout_keep_alive=60,  # idle 60s
        timeout_graceful_shutdown=int(drain_seconds),
        server_header=False,
        lifespan="on",
    )


def install_shutdown_hook(
    app: FastAPI,
    server: uvicorn.Server,
    *,
    loop: asyncio.AbstractEventLoop,
    readiness_grace: float = DEFAULT_READINESS_GRACE_SECONDS,
) -> Callable[..., None]:
    """Point uvicorn's exit hook at our sequence. Returns the installed hook, for tests."""
    stopping = False
    drain_task: asyncio.Task[None] | None = None

    def begin_drain() -> None:
        nonlocal drain_task
        log.info("shutdown signal: readiness is now failing, draining in %.1fs", readiness_grace)
        drain_task = loop.create_task(_stop_accepting(server, readiness_grace), name="drain")

    def handle_exit(sig: int | None = None, frame: Any = None) -> None:
        nonlocal stopping
        if stopping:
            log.warning("second shutdown signal, exiting now")
            server.should_exit = True
            server.force_exit = True
            return
        stopping = True
        # Flip readiness in the signal handler itself: /readyz must start failing before the
        # listener closes, or Kubernetes will keep sending us traffic we cannot answer.
        app.state.shutting_down = True
        loop.call_soon_threadsafe(begin_drain)

    server.handle_exit = handle_exit  # type: ignore[method-assign]
    return handle_exit


async def _stop_accepting(server: uvicorn.Server, grace: float) -> None:
    if grace > 0:
        await asyncio.sleep(grace)
    server.should_exit = True


async def serve(
    app: FastAPI,
    *,
    host: str = "0.0.0.0",
    port: int | None = None,
    drain_seconds: float = DEFAULT_DRAIN_SECONDS,
    readiness_grace: float | None = None,
) -> None:
    """Serve ``app`` until SIGTERM/SIGINT, then shut down gracefully."""
    grace = (
        readiness_grace
        if readiness_grace is not None
        else env_float("SHUTDOWN_READINESS_GRACE_SECONDS", DEFAULT_READINESS_GRACE_SECONDS)
    )
    server = uvicorn.Server(build_config(app, host=host, port=port, drain_seconds=drain_seconds))
    install_shutdown_hook(app, server, loop=asyncio.get_running_loop(), readiness_grace=grace)
    await server.serve()


def run(app: FastAPI, **kwargs: Any) -> None:
    """Synchronous entrypoint for ``python -m <service>``."""
    asyncio.run(serve(app, **kwargs))  # type: ignore[arg-type]
