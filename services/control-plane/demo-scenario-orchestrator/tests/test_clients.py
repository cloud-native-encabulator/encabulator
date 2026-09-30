"""The Prometheus result parsing and the identity the orchestrator asserts."""

from __future__ import annotations

import httpx
import pytest
from encabulator_kit import ServiceClient

from orchestrator.clients import COMPARISONS, IDENTITY, Upstreams, _first_sample
from orchestrator.config import Settings


def test_the_orchestrator_asserts_an_operator_identity() -> None:
    assert IDENTITY.subject == "demo-scenario-orchestrator"
    assert IDENTITY.roles == frozenset({"operator"})
    assert IDENTITY.to_headers() == {
        "X-Encabulator-Subject": "demo-scenario-orchestrator",
        "X-Encabulator-Roles": "operator",
    }


def test_every_comparison_the_loader_accepts_is_implemented() -> None:
    assert set(COMPARISONS) == {">", ">=", "<", "<=", "==", "!="}


@pytest.mark.parametrize(
    ("body", "expected"),
    [
        ({"status": "success", "data": {"resultType": "scalar", "result": [1, "4.5"]}}, 4.5),
        (
            {
                "status": "success",
                "data": {"resultType": "vector", "result": [{"metric": {}, "value": [1, "7"]}]},
            },
            7.0,
        ),
        ({"status": "success", "data": {"resultType": "vector", "result": []}}, None),
        ({"status": "error", "error": "bad query"}, None),
        (
            {
                "status": "success",
                "data": {"resultType": "vector", "result": [{"value": [1, "NaN"]}]},
            },
            None,
        ),
        ("not json at all", None),
    ],
)
def test_prometheus_result_parsing(body: object, expected: float | None) -> None:
    assert _first_sample(body) == expected


async def test_query_goes_to_the_instant_query_endpoint() -> None:
    seen: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(str(request.url))
        return httpx.Response(
            200,
            json={
                "status": "success",
                "data": {"resultType": "vector", "result": [{"value": [1, "3"]}]},
            },
        )

    upstreams = Upstreams(
        prometheus=ServiceClient("http://prometheus:9090", transport=httpx.MockTransport(handler))
    )
    try:
        assert await upstreams.query("up") == 3.0
    finally:
        await upstreams.aclose()
    assert "/api/v1/query?query=up" in seen[0]


def test_unset_urls_make_every_client_absent() -> None:
    from encabulator_kit import CommonSettings

    settings = Settings(
        common=CommonSettings.from_env("demo-scenario-orchestrator"),
        grammeter_url=None,
        malaise_engine_url=None,
        girdlespring_url=None,
        depot_url=None,
        prometheus_url=None,
        scenario_dir="/etc/scenarios",
        run_history=50,
    )
    upstreams = Upstreams.from_settings(settings)
    assert upstreams.grammeter is None
    assert upstreams.prometheus is None


async def test_absent_clients_turn_steps_into_no_ops() -> None:
    upstreams = Upstreams()
    assert await upstreams.set_traffic({"rps": 1}) is None
    assert await upstreams.inject_malaise({"target": "x", "kind": "latency"}) is None
    assert await upstreams.set_flag("turbo-encabulation", True) is None
    assert await upstreams.restock("HYD-MARZ", 10) is None
    assert await upstreams.query("up") is None
    await upstreams.clear_malaise("mal_1")


async def test_clearing_a_malaise_that_already_expired_is_not_an_error() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(404, json={"error": {"code": "not_found", "message": "gone"}})

    upstreams = Upstreams(
        malaise=ServiceClient(
            "http://malaise-injection-engine:8080", transport=httpx.MockTransport(handler)
        )
    )
    try:
        await upstreams.clear_malaise("mal_gone")
    finally:
        await upstreams.aclose()
