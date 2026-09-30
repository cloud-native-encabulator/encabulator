"""The HTTP surface, with NATS and Valkey faked out."""

from __future__ import annotations

import pytest
from encabulator_kit import CommonSettings
from fastapi.testclient import TestClient

from detector.app import build_app
from detector.config import SERVICE_NAME, Settings

from .fakes import FakeBus, FakeValkey


class BusWithClose(FakeBus):
    async def close(self) -> None:
        return None


@pytest.fixture
def rig(monkeypatch):
    for name in ("MALAISE_ENGINE_URL", "DIFFERENTIAL_GIRDLESPRING_URL"):
        monkeypatch.delenv(name, raising=False)

    async def no_consumer(self) -> None:
        return None

    monkeypatch.setattr("encabulator_kit.messaging.PullConsumer.start", no_consumer)
    monkeypatch.setattr("encabulator_kit.messaging.PullConsumer.stop", no_consumer)

    bus = BusWithClose()
    valkey = FakeValkey()
    settings = Settings(
        common=CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0"),
        recorder_url=None,
        valkey_url="redis://valkey:6379/0",
        concurrency=2,
    )
    app = build_app(settings, bus=bus, valkey=valkey)  # type: ignore[arg-type]
    return app, bus, valkey


def test_incidents_endpoint_is_empty_at_first(rig) -> None:
    app, _, _ = rig
    with TestClient(app) as client:
        assert client.get("/v1/incidents").json() == []


def test_incidents_endpoint_returns_what_the_store_has(rig) -> None:
    app, _, valkey = rig
    with TestClient(app) as client:
        valkey.items = [
            '{"id":"inc_2","serial":"ENC-2"}',
            '{"id":"inc_1","serial":"ENC-1"}',
        ]
        assert [item["id"] for item in client.get("/v1/incidents").json()] == ["inc_2", "inc_1"]
        assert [item["id"] for item in client.get("/v1/incidents?limit=1").json()] == ["inc_2"]


def test_incidents_limit_is_validated(rig) -> None:
    app, _, _ = rig
    with TestClient(app, raise_server_exceptions=False) as client:
        assert client.get("/v1/incidents?limit=0").status_code == 400
        assert client.get("/v1/incidents?limit=501").status_code == 400


def test_stats_endpoint(rig) -> None:
    app, _, _ = rig
    with TestClient(app) as client:
        assert client.get("/v1/stats").json() == {
            "inspected": 0,
            "passed": 0,
            "failed": 0,
            "failRate5m": 0.0,
        }


def test_readiness_covers_nats_and_valkey(rig) -> None:
    app, bus, valkey = rig
    with TestClient(app, raise_server_exceptions=False) as client:
        assert client.get("/readyz").json()["checks"] == {"nats": "ok", "valkey": "ok"}
        valkey.fail = True
        response = client.get("/readyz")
        assert response.status_code == 503
        assert "unreachable" in response.json()["checks"]["valkey"]
        valkey.fail = False
        bus.connected = False
        assert client.get("/readyz").status_code == 503


def test_metrics_expose_the_inspection_series(rig) -> None:
    app, _, _ = rig
    with TestClient(app) as client:
        text = client.get("/metrics").text
    assert "encabulator_units_inspected_total" in text
    assert "encabulator_anomaly_score" in text
