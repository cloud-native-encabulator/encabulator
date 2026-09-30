"""The orchestrator's HTTP surface."""

from __future__ import annotations

import time

import pytest
from encabulator_kit import CommonSettings
from fastapi.testclient import TestClient

from orchestrator.app import build_app
from orchestrator.config import SERVICE_NAME, Settings
from orchestrator.loader import load_builtin, parse_scenario

from .fakes import FakeUpstreams


@pytest.fixture
def settings(monkeypatch) -> Settings:
    for name in ("MALAISE_ENGINE_URL", "DIFFERENTIAL_GIRDLESPRING_URL", "PROMETHEUS_URL"):
        monkeypatch.delenv(name, raising=False)
    return Settings(
        common=CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0"),
        grammeter_url=None,
        malaise_engine_url=None,
        girdlespring_url=None,
        depot_url=None,
        prometheus_url=None,
        scenario_dir="/nonexistent",
        run_history=10,
    )


@pytest.fixture
def upstreams() -> FakeUpstreams:
    return FakeUpstreams()


@pytest.fixture
def client(settings: Settings, upstreams: FakeUpstreams):
    quick = parse_scenario(
        {
            "id": "quick",
            "title": "Quick Scenario",
            "description": "Two steps and out.",
            "steps": [{"say": "hello"}, {"traffic": {"rps": 3}}],
        }
    )
    slow = parse_scenario({"id": "slow", "title": "Slow Scenario", "steps": [{"wait": "30s"}]})
    catalog = {scenario.id: scenario for scenario in (*load_builtin(), quick, slow)}
    app = build_app(settings, upstreams=upstreams, scenarios=catalog)
    with TestClient(app, raise_server_exceptions=False) as test_client:
        yield test_client


def _wait_for_status(client: TestClient, run_id: str, status: str, timeout: float = 5.0) -> dict:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        body = client.get(f"/v1/runs/{run_id}").json()
        if body["status"] == status:
            return body
        time.sleep(0.02)
    raise AssertionError(f"run stayed {body['status']}, wanted {status}")


def test_platform_endpoints(client: TestClient) -> None:
    assert client.get("/healthz").json() == {"status": "ok"}
    assert client.get("/readyz").status_code == 200
    assert "http_server_request_duration_seconds" in client.get("/metrics").text


def test_list_scenarios_includes_the_six_built_ins(client: TestClient) -> None:
    body = client.get("/v1/scenarios").json()
    ids = {scenario["id"] for scenario in body}
    assert {
        "morning-shift",
        "marzlevane-shortage",
        "hydrocoptic-outage",
        "side-fumbling-outbreak",
        "dingle-arm-congestion",
        "grand-tour",
    } <= ids
    tour = next(scenario for scenario in body if scenario["id"] == "grand-tour")
    assert tour["stepCount"] == len(tour["steps"])
    assert tour["steps"][0] == {"index": 1, "kind": "say", "summary": tour["steps"][0]["summary"]}
    assert tour["source"] == "built-in"


def test_get_one_scenario(client: TestClient) -> None:
    body = client.get("/v1/scenarios/quick").json()
    assert body["title"] == "Quick Scenario"
    assert [step["kind"] for step in body["steps"]] == ["say", "traffic"]


def test_unknown_scenario_is_404(client: TestClient) -> None:
    response = client.get("/v1/scenarios/nope")
    assert response.status_code == 404
    assert response.json()["error"]["code"] == "scenario_not_found"


def test_starting_a_run_returns_202_and_the_run(client: TestClient, upstreams) -> None:
    response = client.post("/v1/scenarios/quick/runs")
    assert response.status_code == 202
    run = response.json()
    assert run["id"].startswith("run_")
    assert run["scenarioId"] == "quick"
    assert run["totalSteps"] == 2
    assert run["status"] in {"running", "completed"}

    finished = _wait_for_status(client, run["id"], "completed")
    assert finished["currentStep"] == 2
    assert finished["finishedAt"] is not None
    assert [entry["message"] for entry in finished["log"] if entry["kind"] == "say"] == ["hello"]
    assert upstreams.traffic_calls == [{"rps": 3}]


def test_a_second_run_is_409(client: TestClient) -> None:
    started = client.post("/v1/scenarios/slow/runs").json()
    try:
        conflict = client.post("/v1/scenarios/quick/runs")
        assert conflict.status_code == 409
        assert conflict.json()["error"]["code"] == "run_in_progress"
    finally:
        client.post(f"/v1/runs/{started['id']}/cancel")


def test_cancelling_a_run(client: TestClient) -> None:
    started = client.post("/v1/scenarios/slow/runs").json()
    cancelled = client.post(f"/v1/runs/{started['id']}/cancel").json()
    assert cancelled["status"] == "cancelled"
    assert cancelled["finishedAt"] is not None
    # And the floor is free again.
    assert client.post("/v1/scenarios/quick/runs").status_code == 202


def test_runs_are_listed_newest_first_without_the_log(client: TestClient) -> None:
    first = client.post("/v1/scenarios/quick/runs").json()
    _wait_for_status(client, first["id"], "completed")
    second = client.post("/v1/scenarios/quick/runs").json()
    _wait_for_status(client, second["id"], "completed")

    listed = client.get("/v1/runs?limit=5").json()
    assert [run["id"] for run in listed[:2]] == [second["id"], first["id"]]
    assert "log" not in listed[0]


def test_unknown_run_is_404(client: TestClient) -> None:
    assert client.get("/v1/runs/run_nope").status_code == 404
    assert client.post("/v1/runs/run_nope/cancel").json()["error"]["code"] == "run_not_found"


def test_runs_limit_is_validated(client: TestClient) -> None:
    assert client.get("/v1/runs?limit=0").status_code == 400
    assert client.get("/v1/runs?limit=999").status_code == 400


def test_narration_endpoint_follows_the_active_run(client: TestClient) -> None:
    assert client.get("/v1/narration").json() == {"active": False}
    started = client.post("/v1/scenarios/slow/runs").json()
    try:
        body = client.get("/v1/narration").json()
        assert body["active"] is True
        assert body["run"]["id"] == started["id"]
    finally:
        client.post(f"/v1/runs/{started['id']}/cancel")
    assert client.get("/v1/narration").json() == {"active": False}


def test_scenario_metrics_are_exposed(client: TestClient) -> None:
    run = client.post("/v1/scenarios/quick/runs").json()
    _wait_for_status(client, run["id"], "completed")
    text = client.get("/metrics").text
    assert 'encabulator_scenario_runs_total{result="completed",scenario="quick"}' in text
    assert "encabulator_scenario_active" in text
    assert 'encabulator_scenario_steps_total{kind="say"}' in text
