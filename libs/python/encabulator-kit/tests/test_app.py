from __future__ import annotations

import pytest
from fastapi import Body
from fastapi.testclient import TestClient
from prometheus_client.parser import text_string_to_metric_families

from encabulator_kit.app import create_app
from encabulator_kit.errors import EncabulatorError


@pytest.fixture
def app():
    application = create_app("kit-app-test", version="9.9.9", configure_logs=False)

    @application.get("/v1/orders/{order_id}")
    async def order(order_id: str) -> dict[str, str]:
        return {"id": order_id}

    @application.post("/v1/orders")
    async def place(quantity: int = Body(embed=True)) -> dict[str, int]:
        return {"quantity": quantity}

    @application.get("/v1/boom")
    async def boom() -> None:
        raise EncabulatorError(409, "insufficient_parts", "Out of hydrocoptic marzlevanes")

    @application.get("/v1/kaboom")
    async def kaboom() -> None:
        raise RuntimeError("prefamulated amulite has delaminated")

    return application


@pytest.fixture
def client(app) -> TestClient:
    with TestClient(app, raise_server_exceptions=False) as test_client:
        yield test_client


def test_healthz(client: TestClient) -> None:
    response = client.get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_readyz_with_no_checks(client: TestClient) -> None:
    response = client.get("/readyz")
    assert response.status_code == 200
    assert response.json()["status"] == "ok"
    assert response.json()["version"] == "9.9.9"


def test_readyz_reports_failing_check(app, client: TestClient) -> None:
    async def broken() -> None:
        raise RuntimeError("postgres is asleep")

    app.state.readiness.add("postgres", broken)
    response = client.get("/readyz")
    assert response.status_code == 503
    body = response.json()
    assert body["status"] == "unready"
    assert "postgres is asleep" in body["checks"]["postgres"]
    app.state.readiness.remove("postgres")


def test_readyz_fails_while_shutting_down(app, client: TestClient) -> None:
    app.state.shutting_down = True
    try:
        response = client.get("/readyz")
        assert response.status_code == 503
        assert response.json()["status"] == "shutting_down"
    finally:
        app.state.shutting_down = False


def test_readyz_check_timeout_is_not_fatal(app, client: TestClient) -> None:
    import asyncio

    async def hangs() -> None:
        await asyncio.sleep(30)

    app.state.readiness.timeout = 0.05
    app.state.readiness.add("valkey", hangs)
    try:
        assert client.get("/readyz").status_code == 503
    finally:
        app.state.readiness.remove("valkey")


def test_metrics_endpoint_serves_prometheus_text(client: TestClient) -> None:
    client.get("/v1/orders/ord_1")
    response = client.get("/metrics")
    assert response.status_code == 200
    assert "text/plain" in response.headers["content-type"]
    assert "http_server_request_duration_seconds" in response.text


def test_histogram_uses_the_route_template(client: TestClient) -> None:
    client.get("/v1/orders/ord_deadbeef")
    families = _families(client.get("/metrics").text)
    routes = {
        sample.labels["http_route"]
        for family in families
        if family.name == "http_server_request_duration_seconds"
        for sample in family.samples
    }
    assert "/v1/orders/{order_id}" in routes
    assert "/v1/orders/ord_deadbeef" not in routes


def test_health_and_metrics_are_not_in_the_histogram(client: TestClient) -> None:
    client.get("/healthz")
    families = _families(client.get("/metrics").text)
    routes = {
        sample.labels["http_route"]
        for family in families
        if family.name == "http_server_request_duration_seconds"
        for sample in family.samples
    }
    assert "/healthz" not in routes
    assert "/metrics" not in routes


def test_request_id_is_echoed(client: TestClient) -> None:
    response = client.get("/v1/orders/ord_1", headers={"X-Request-Id": "req-abc"})
    assert response.headers["x-request-id"] == "req-abc"


def test_request_id_is_generated_when_absent(client: TestClient) -> None:
    response = client.get("/v1/orders/ord_1")
    assert len(response.headers["x-request-id"]) == 26


def test_error_shape_for_encabulator_error(client: TestClient) -> None:
    response = client.get("/v1/boom")
    assert response.status_code == 409
    assert response.json() == {
        "error": {"code": "insufficient_parts", "message": "Out of hydrocoptic marzlevanes"}
    }


def test_error_shape_for_unhandled_exception(client: TestClient) -> None:
    response = client.get("/v1/kaboom")
    assert response.status_code == 500
    assert response.json()["error"]["code"] == "internal_error"


def test_error_shape_for_not_found(client: TestClient) -> None:
    response = client.get("/v1/nope")
    assert response.status_code == 404
    assert response.json()["error"]["code"] == "not_found"


def test_validation_errors_are_400_with_a_code(client: TestClient) -> None:
    response = client.post("/v1/orders", json={"quantity": "six marzlevanes"})
    assert response.status_code == 400
    body = response.json()
    assert body["error"]["code"] == "invalid_request"
    assert "quantity" in body["error"]["message"]
    # The submitted value must not be echoed back.
    assert "six marzlevanes" not in body["error"]["message"]


def _families(text: str):
    return list(text_string_to_metric_families(text))
