from __future__ import annotations

import random

import httpx
import pytest

from encabulator_kit.clients import ServiceClient
from encabulator_kit.context import reset_request_id, set_request_id
from encabulator_kit.errors import UpstreamError
from encabulator_kit.identity import Identity


class _Upstream:
    def __init__(self, *responses: httpx.Response | Exception) -> None:
        self.responses = list(responses)
        self.requests: list[httpx.Request] = []

    def handler(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        result = self.responses.pop(0) if self.responses else httpx.Response(200, json={})
        if isinstance(result, Exception):
            raise result
        return result

    @property
    def transport(self) -> httpx.MockTransport:
        return httpx.MockTransport(self.handler)


def _client(upstream: _Upstream, **kwargs: object) -> ServiceClient:
    return ServiceClient(
        "http://hydrocoptic-datastore:8080",
        transport=upstream.transport,
        rng=random.Random(1),
        backoff_base=0.001,
        **kwargs,  # type: ignore[arg-type]
    )


def test_service_name_comes_from_the_host() -> None:
    assert _client(_Upstream()).service_name == "hydrocoptic-datastore"


async def test_get_retries_twice_then_gives_up() -> None:
    upstream = _Upstream(*[httpx.ConnectError("down")] * 3)
    async with _client(upstream) as client:
        with pytest.raises(UpstreamError) as caught:
            await client.get("/v1/orders")
    assert len(upstream.requests) == 3  # one attempt plus two retries
    assert caught.value.code == "upstream_unavailable"


async def test_get_retries_then_succeeds() -> None:
    upstream = _Upstream(httpx.Response(503), httpx.Response(200, json={"ok": True}))
    async with _client(upstream) as client:
        assert await client.json("GET", "/v1/orders") == {"ok": True}
    assert len(upstream.requests) == 2


async def test_post_does_not_retry_without_an_idempotency_key() -> None:
    upstream = _Upstream(httpx.Response(503), httpx.Response(200))
    async with _client(upstream) as client:
        response = await client.post("/v1/orders", json={})
    assert response.status_code == 503
    assert len(upstream.requests) == 1


async def test_post_retries_with_an_idempotency_key() -> None:
    upstream = _Upstream(httpx.Response(503), httpx.Response(201, json={"id": "ord_1"}))
    async with _client(upstream) as client:
        body = await client.json("POST", "/v1/orders", json={}, idempotency_key="ord_1")
    assert body == {"id": "ord_1"}
    assert len(upstream.requests) == 2
    assert upstream.requests[0].headers["idempotency-key"] == "ord_1"


async def test_identity_and_request_id_headers_are_forwarded() -> None:
    upstream = _Upstream(httpx.Response(200, json={}))
    token = set_request_id("req-99")
    try:
        async with _client(upstream) as client:
            await client.get("/v1/orders", identity=Identity.of("ada", ["customer"]))
    finally:
        reset_request_id(token)
    headers = upstream.requests[0].headers
    assert headers["x-encabulator-subject"] == "ada"
    assert headers["x-encabulator-roles"] == "customer"
    assert headers["x-request-id"] == "req-99"


async def test_json_raises_upstream_error_with_the_upstream_code() -> None:
    upstream = _Upstream(
        httpx.Response(409, json={"error": {"code": "insufficient_parts", "message": "no amulite"}})
    )
    async with _client(upstream) as client:
        with pytest.raises(UpstreamError) as caught:
            await client.json("POST", "/v1/reservations", json={})
    assert caught.value.status_code == 409
    assert caught.value.code == "insufficient_parts"
    assert caught.value.message == "no amulite"


async def test_client_metric_is_recorded_with_the_target_service() -> None:
    from prometheus_client.parser import text_string_to_metric_families

    from encabulator_kit.metrics import render

    upstream = _Upstream(httpx.Response(200, json={}))
    async with _client(upstream) as client:
        await client.get("/v1/stats")
    labelled = {
        (sample.labels["server_address"], sample.labels["http_request_method"])
        for family in text_string_to_metric_families(render().decode())
        if family.name == "http_client_request_duration_seconds"
        for sample in family.samples
    }
    assert ("hydrocoptic-datastore", "GET") in labelled
