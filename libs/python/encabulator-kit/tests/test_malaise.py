from __future__ import annotations

import asyncio
import random
from datetime import UTC, datetime, timedelta

import httpx
import pytest
from fastapi.testclient import TestClient

from encabulator_kit.app import create_app
from encabulator_kit.malaise import (
    CpuBurner,
    Directive,
    MalaiseCongested,
    MalaiseController,
    MalaiseInjected,
)


def _directive(kind: str, seconds: float = 60, **params: object) -> Directive:
    return Directive(
        id=f"mal_{kind}",
        kind=kind,
        params=params,
        expires_at=datetime.now(UTC) + timedelta(seconds=seconds),
    )


def _controller(*directives: Directive, seed: int = 7) -> MalaiseController:
    controller = MalaiseController("kit-test", "http://malaise:8080", rng=random.Random(seed))
    controller.set_directives(directives)
    return controller


def test_disabled_without_engine_url() -> None:
    assert MalaiseController("kit-test", "").enabled is False
    assert MalaiseController("kit-test", "http://malaise:8080").enabled is True


async def test_no_directives_is_a_no_op() -> None:
    controller = _controller()
    async with controller.guard():
        pass


async def test_latency_sleeps() -> None:
    controller = _controller(_directive("latency", ms=120, jitterMs=0))
    started = asyncio.get_running_loop().time()
    async with controller.guard():
        pass
    assert asyncio.get_running_loop().time() - started >= 0.1


async def test_error_raises_with_the_configured_status() -> None:
    controller = _controller(_directive("error", status=502))
    with pytest.raises(MalaiseInjected) as caught:
        async with controller.guard():
            pytest.fail("the handler must not run")
    assert caught.value.status_code == 502
    assert caught.value.code == "malaise_injected"


async def test_error_percent_is_honoured() -> None:
    controller = _controller(_directive("error", status=503, percent=50), seed=1234)
    injected = 0
    for _ in range(400):
        try:
            async with controller.guard():
                pass
        except MalaiseInjected:
            injected += 1
    assert 140 < injected < 260


async def test_congestion_admits_only_max_concurrent() -> None:
    controller = _controller(_directive("congestion", maxConcurrent=2, queueTimeoutMs=2000))
    peak = 0
    active = 0

    async def worker() -> None:
        nonlocal peak, active
        async with controller.guard():
            active += 1
            peak = max(peak, active)
            await asyncio.sleep(0.05)
            active -= 1

    await asyncio.gather(*(worker() for _ in range(6)))
    assert peak == 2


async def test_congestion_queue_timeout_gives_congested() -> None:
    controller = _controller(_directive("congestion", maxConcurrent=1, queueTimeoutMs=20))

    async def hold() -> None:
        async with controller.guard():
            await asyncio.sleep(0.3)

    holder = asyncio.create_task(hold())
    await asyncio.sleep(0.02)
    with pytest.raises(MalaiseCongested) as caught:
        async with controller.guard():
            pytest.fail("must not be admitted")
    assert caught.value.status_code == 503
    assert caught.value.code == "congested"
    await holder


async def test_expired_directives_stop_applying() -> None:
    controller = _controller(_directive("error", seconds=-1, status=503))
    async with controller.guard():
        pass  # nothing raised: the directive is past expiresAt


async def test_message_handler_wrapper() -> None:
    controller = _controller(_directive("error", status=503))
    calls = 0

    async def handle(value: int) -> int:
        nonlocal calls
        calls += 1
        return value * 2

    wrapped = controller.wrap_message_handler(handle)
    with pytest.raises(MalaiseInjected):
        await wrapped(21)
    assert calls == 0

    controller.set_directives(())
    assert await wrapped(21) == 42
    assert calls == 1


def test_cpu_burner_starts_and_stops_threads() -> None:
    burner = CpuBurner()
    try:
        burner.set_load(2500)
        assert burner.millicores == 2500
        assert len(burner._threads) == 3  # 1000 + 1000 + 500
        burner.set_load(0)
        assert burner.millicores == 0
        assert burner._threads == []
    finally:
        burner.stop()


def test_cpu_burn_directive_drives_the_burner() -> None:
    controller = _controller(_directive("cpu-burn", millicores=100))
    try:
        assert controller._burner.millicores == 100
        controller.set_directives(())
        assert controller._burner.millicores == 0
    finally:
        controller._burner.stop()


# ---- polling ---------------------------------------------------------------------------


def _engine(directives: list[dict[str, object]]) -> tuple[httpx.MockTransport, list[str]]:
    queries: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        queries.append(str(request.url))
        if request.url.path != "/v1/directives":
            return httpx.Response(404)
        return httpx.Response(200, json={"directives": directives})

    return httpx.MockTransport(handler), queries


async def test_polls_the_engine_for_its_own_service() -> None:
    expires = (datetime.now(UTC) + timedelta(seconds=60)).isoformat()
    transport, queries = _engine(
        [{"id": "mal_1", "kind": "latency", "params": {"ms": 5}, "expiresAt": expires}]
    )
    controller = MalaiseController(
        "logarithmic-casing-line", "http://malaise:8080", transport=transport, poll_interval=0.01
    )
    await controller.start()
    try:
        assert "service=logarithmic-casing-line" in queries[0]
        assert controller.directive("latency") is not None
    finally:
        await controller.stop()


async def test_unreachable_engine_keeps_last_directives_until_expiry() -> None:
    calls = {"n": 0}

    def handler(request: httpx.Request) -> httpx.Response:
        calls["n"] += 1
        if calls["n"] == 1:
            expires = (datetime.now(UTC) + timedelta(milliseconds=120)).isoformat()
            return httpx.Response(
                200,
                json={
                    "directives": [
                        {"id": "m", "kind": "latency", "params": {"ms": 1}, "expiresAt": expires}
                    ]
                },
            )
        raise httpx.ConnectError("engine down")

    controller = MalaiseController(
        "kit-test", "http://malaise:8080", transport=httpx.MockTransport(handler)
    )
    await controller.start()
    try:
        assert await controller.refresh() is False  # engine is down now
        assert controller.directive("latency") is not None  # still applying
        await asyncio.sleep(0.15)
        assert controller.directive("latency") is None  # expiresAt passed
    finally:
        await controller.stop()


# ---- HTTP middleware -------------------------------------------------------------------


@pytest.fixture
def malaise_app():
    controller = MalaiseController("kit-test", "http://malaise:8080", rng=random.Random(3))
    app = create_app("kit-malaise-test", malaise=controller, configure_logs=False)

    @app.get("/v1/thing")
    async def thing() -> dict[str, bool]:
        return {"ok": True}

    return app, controller


def test_middleware_injects_errors(malaise_app) -> None:
    app, controller = malaise_app
    controller.set_directives([_directive("error", status=503)])
    client = TestClient(app, raise_server_exceptions=False)
    response = client.get("/v1/thing")
    assert response.status_code == 503
    assert response.json() == {
        "error": {
            "code": "malaise_injected",
            "message": "The encabulator is experiencing induced malaise",
        }
    }


def test_middleware_spares_health_and_metrics(malaise_app) -> None:
    app, controller = malaise_app
    controller.set_directives([_directive("error", status=503)])
    client = TestClient(app, raise_server_exceptions=False)
    assert client.get("/healthz").status_code == 200
    assert client.get("/readyz").status_code == 200
    assert client.get("/metrics").status_code == 200


def test_middleware_passes_through_when_no_directives(malaise_app) -> None:
    app, controller = malaise_app
    controller.set_directives(())
    client = TestClient(app, raise_server_exceptions=False)
    assert client.get("/v1/thing").json() == {"ok": True}


def test_injected_errors_still_carry_the_route_label(malaise_app) -> None:
    from prometheus_client.parser import text_string_to_metric_families

    app, controller = malaise_app
    controller.set_directives([_directive("error", status=503)])
    client = TestClient(app, raise_server_exceptions=False)
    client.get("/v1/thing")
    text = client.get("/metrics").text
    labelled = {
        (sample.labels["http_route"], sample.labels["http_response_status_code"])
        for family in text_string_to_metric_families(text)
        if family.name == "http_server_request_duration_seconds"
        for sample in family.samples
    }
    assert ("/v1/thing", "503") in labelled
