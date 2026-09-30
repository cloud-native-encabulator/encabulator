"""The graceful shutdown sequence: fail readiness, then stop accepting, then drain."""

from __future__ import annotations

import asyncio
import signal

import uvicorn
from fastapi.testclient import TestClient

from encabulator_kit.app import create_app
from encabulator_kit.serve import build_config, install_shutdown_hook


def test_config_matches_the_designs_timeouts() -> None:
    app = create_app("kit-serve-test", configure_logs=False)
    config = build_config(app, port=9999, drain_seconds=20.0)
    assert config.port == 9999
    assert config.timeout_keep_alive == 60  # idle 60s
    assert config.timeout_graceful_shutdown == 20  # drain up to 20s
    assert config.access_log is False  # the request histogram replaces it
    assert config.log_config is None  # the kit owns logging


def test_port_comes_from_the_environment(monkeypatch) -> None:
    monkeypatch.setenv("PORT", "8123")
    app = create_app("kit-serve-test", configure_logs=False)
    assert build_config(app).port == 8123


async def test_sigterm_fails_readiness_before_it_stops_accepting() -> None:
    app = create_app("kit-serve-test", configure_logs=False)
    server = uvicorn.Server(build_config(app, port=0))
    hook = install_shutdown_hook(app, server, loop=asyncio.get_running_loop(), readiness_grace=0.2)

    with TestClient(app) as client:
        assert client.get("/readyz").status_code == 200

        hook(signal.SIGTERM, None)

        # Readiness fails immediately, and the server is still accepting.
        assert app.state.shutting_down is True
        assert client.get("/readyz").status_code == 503
        assert client.get("/readyz").json()["status"] == "shutting_down"
        assert server.should_exit is False

        # Only after the grace does uvicorn get told to stop.
        await asyncio.sleep(0.4)
        assert server.should_exit is True
        assert server.force_exit is False


async def test_a_second_signal_forces_an_immediate_exit() -> None:
    app = create_app("kit-serve-test", configure_logs=False)
    server = uvicorn.Server(build_config(app, port=0))
    hook = install_shutdown_hook(app, server, loop=asyncio.get_running_loop(), readiness_grace=30.0)
    hook(signal.SIGTERM, None)
    assert server.force_exit is False
    hook(signal.SIGTERM, None)
    assert server.should_exit is True
    assert server.force_exit is True


async def test_the_hook_replaces_uvicorns_own_so_one_signal_is_handled_once() -> None:
    """uvicorn re-raises captured signals after serve() returns; sharing the hook would mean
    handling the same SIGTERM twice and skipping the readiness grace entirely."""
    app = create_app("kit-serve-test", configure_logs=False)
    server = uvicorn.Server(build_config(app, port=0))
    original = server.handle_exit
    hook = install_shutdown_hook(app, server, loop=asyncio.get_running_loop())
    assert server.handle_exit is hook
    assert server.handle_exit is not original

    hook(signal.SIGTERM, None)
    await asyncio.sleep(0)
    # Nothing was recorded for uvicorn to re-raise, so the process exits on its own terms.
    assert server._captured_signals == []
