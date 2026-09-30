"""The HTTP surface, with NATS faked out."""

from __future__ import annotations

from typing import Any

import pytest
from encabulator_kit import CommonSettings
from fastapi.testclient import TestClient

from casing_line.app import build_app
from casing_line.config import SERVICE_NAME, Settings


class FakeClient:
    is_connected = True


class FakeBus:
    def __init__(self) -> None:
        self.client = FakeClient()
        self.source = SERVICE_NAME
        self.published: list[tuple[str, dict[str, Any]]] = []

    @property
    def connected(self) -> bool:
        return self.client.is_connected

    async def publish(self, subject: str, data: dict[str, Any], **_: Any) -> None:
        self.published.append((subject, data))

    async def close(self) -> None:
        return None


@pytest.fixture
def app(monkeypatch):
    for name in ("MALAISE_ENGINE_URL", "DIFFERENTIAL_GIRDLESPRING_URL"):
        monkeypatch.delenv(name, raising=False)
    bus = FakeBus()
    settings = Settings(
        common=CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0"),
        line_capacity=3,
        recorder_url=None,
        depot_url=None,
    )

    async def no_consumer(self) -> None:
        return None

    monkeypatch.setattr("encabulator_kit.messaging.PullConsumer.start", no_consumer)
    monkeypatch.setattr("encabulator_kit.messaging.PullConsumer.stop", no_consumer)
    application = build_app(settings, bus=bus)  # type: ignore[arg-type]
    application.state.fake_bus = bus
    return application


def test_line_endpoint(app) -> None:
    with TestClient(app) as client:
        body = client.get("/v1/line").json()
    assert body == {
        "capacity": 3,
        "activeOrders": 0,
        "activeUnits": [],
        "unitsCompleted": 0,
    }


def test_readiness_follows_the_nats_connection(app) -> None:
    with TestClient(app, raise_server_exceptions=False) as client:
        assert client.get("/readyz").json()["checks"] == {"nats": "ok"}
        app.state.fake_bus.client.is_connected = False
        response = client.get("/readyz")
        assert response.status_code == 503
        assert "not connected" in response.json()["checks"]["nats"]


def test_metrics_expose_the_line_gauges(app) -> None:
    with TestClient(app) as client:
        text = client.get("/metrics").text
    assert "encabulator_line_active_orders" in text
    assert "encabulator_stage_duration_seconds" in text
    assert "encabulator_units_fabricated_total" in text
