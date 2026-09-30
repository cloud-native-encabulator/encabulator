"""Outbound HTTP, per design section 7.

Default timeout 3s. Idempotent requests (GET, HEAD, PUT, DELETE, OPTIONS) retry twice with
exponential backoff (100 ms base, full jitter). POST retries only when the caller supplies an
idempotency key. Every attempt lands in ``http_client_request_duration_seconds`` labelled with
the target service name.
"""

from __future__ import annotations

import asyncio
import logging
import random
import time
from collections.abc import Mapping
from typing import Any
from urllib.parse import urlsplit

import httpx

from .context import current_request_id
from .errors import UpstreamError
from .identity import Identity
from .telemetry import untraced

__all__ = ["IDEMPOTENT_METHODS", "RETRY_STATUS", "ServiceClient"]

log = logging.getLogger(__name__)

IDEMPOTENT_METHODS = frozenset({"GET", "HEAD", "PUT", "DELETE", "OPTIONS"})
RETRY_STATUS = frozenset({429, 500, 502, 503, 504})

REQUEST_ID_HEADER = "X-Request-Id"
IDEMPOTENCY_HEADER = "Idempotency-Key"


def service_name_from_url(url: str) -> str:
    """``http://spurving-bearing-quoter:8080`` -> ``spurving-bearing-quoter``."""
    host = urlsplit(url).hostname or url
    return host


class ServiceClient:
    """An httpx client for one upstream service, with the house retry and metric policy."""

    def __init__(
        self,
        base_url: str,
        *,
        service_name: str | None = None,
        timeout: float = 3.0,
        retries: int = 2,
        backoff_base: float = 0.1,
        headers: Mapping[str, str] | None = None,
        traced: bool = True,
        transport: httpx.AsyncBaseTransport | None = None,
        rng: random.Random | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.service_name = service_name or service_name_from_url(base_url)
        self.retries = max(0, retries)
        self.backoff_base = backoff_base
        self.traced = traced
        self._rng = rng or random.Random()
        self._client = httpx.AsyncClient(
            base_url=self.base_url,
            timeout=httpx.Timeout(timeout),
            headers=dict(headers or {}),
            transport=transport,
            follow_redirects=False,
        )

    async def aclose(self) -> None:
        await self._client.aclose()

    async def __aenter__(self) -> ServiceClient:
        return self

    async def __aexit__(self, *exc: object) -> None:
        await self.aclose()

    @property
    def httpx_client(self) -> httpx.AsyncClient:
        return self._client

    async def request(
        self,
        method: str,
        path: str,
        *,
        json: Any = None,
        content: bytes | None = None,
        params: Mapping[str, Any] | None = None,
        headers: Mapping[str, str] | None = None,
        identity: Identity | None = None,
        idempotency_key: str | None = None,
        timeout: float | None = None,
        retries: int | None = None,
    ) -> httpx.Response:
        method = method.upper()
        merged = self._headers(headers, identity, idempotency_key)
        retryable = method in IDEMPOTENT_METHODS or idempotency_key is not None
        attempts = (retries if retries is not None else self.retries) + 1 if retryable else 1

        last_error: Exception | None = None
        for attempt in range(attempts):
            started = time.perf_counter()
            try:
                response = await self._send(
                    method,
                    path,
                    json=json,
                    content=content,
                    params=params,
                    headers=merged,
                    timeout=timeout,
                )
            except httpx.HTTPError as exc:
                self._observe(method, 0, started)
                last_error = exc
                if attempt + 1 >= attempts:
                    break
                await self._sleep(attempt)
                continue

            self._observe(method, response.status_code, started)
            if response.status_code in RETRY_STATUS and attempt + 1 < attempts:
                await response.aclose()
                await self._sleep(attempt)
                continue
            return response

        raise UpstreamError(
            self.service_name,
            0,
            "upstream_unavailable",
            f"{self.service_name} is unreachable: {last_error}",
        ) from last_error

    async def _send(self, method: str, path: str, **kwargs: Any) -> httpx.Response:
        if self.traced:
            return await self._client.request(method, path, **kwargs)
        with untraced():
            return await self._client.request(method, path, **kwargs)

    def _headers(
        self,
        headers: Mapping[str, str] | None,
        identity: Identity | None,
        idempotency_key: str | None,
    ) -> dict[str, str]:
        merged: dict[str, str] = {}
        request_id = current_request_id()
        if request_id:
            merged[REQUEST_ID_HEADER] = request_id
        if identity is not None:
            merged.update(identity.to_headers())
        if idempotency_key:
            merged[IDEMPOTENCY_HEADER] = idempotency_key
        if headers:
            merged.update(headers)
        return merged

    def _observe(self, method: str, status_code: int, started: float) -> None:
        from .metrics import http_client_duration

        http_client_duration(method, self.service_name, status_code, time.perf_counter() - started)

    async def _sleep(self, attempt: int) -> None:
        # Exponential backoff with full jitter: 100 ms base.
        ceiling = self.backoff_base * (2**attempt)
        await asyncio.sleep(self._rng.uniform(0.0, ceiling))

    # ---- convenience -------------------------------------------------------------------

    async def get(self, path: str, **kwargs: Any) -> httpx.Response:
        return await self.request("GET", path, **kwargs)

    async def post(self, path: str, **kwargs: Any) -> httpx.Response:
        return await self.request("POST", path, **kwargs)

    async def put(self, path: str, **kwargs: Any) -> httpx.Response:
        return await self.request("PUT", path, **kwargs)

    async def delete(self, path: str, **kwargs: Any) -> httpx.Response:
        return await self.request("DELETE", path, **kwargs)

    async def json(self, method: str, path: str, **kwargs: Any) -> Any:
        """Make a request and return the decoded body, raising :class:`UpstreamError` on 4xx/5xx."""
        response = await self.request(method, path, **kwargs)
        if response.status_code >= 400:
            raise _upstream_error(self.service_name, response)
        if not response.content:
            return None
        return response.json()


def _upstream_error(service: str, response: httpx.Response) -> UpstreamError:
    code = "upstream_error"
    message = f"{service} responded {response.status_code}"
    body: Any = None
    try:
        body = response.json()
        error = body.get("error") if isinstance(body, dict) else None
        if isinstance(error, dict):
            code = str(error.get("code", code))
            message = str(error.get("message", message))
    except ValueError:
        pass
    return UpstreamError(service, response.status_code, code, message, body=body)
