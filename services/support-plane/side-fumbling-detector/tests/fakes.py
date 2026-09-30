"""Shared fakes: a bus, a recorder, and a Valkey list."""

from __future__ import annotations

from typing import Any

import httpx
from encabulator_kit import Envelope, ServiceClient


class FakeBus:
    def __init__(self) -> None:
        self.published: list[tuple[str, dict[str, Any]]] = []
        self.source = "side-fumbling-detector"
        self.connected = True

    async def publish(self, subject: str, data: dict[str, Any], **_: Any) -> Envelope:
        self.published.append((subject, data))
        return Envelope.create(subject, data, self.source)

    def data_for(self, subject: str) -> list[dict[str, Any]]:
        return [data for published, data in self.published if published == subject]


class FakeValkey:
    """Enough of a Valkey list for the incident store."""

    def __init__(self, *, fail: bool = False) -> None:
        self.items: list[str] = []
        self.fail = fail

    async def lpush(self, name: str, *values: str) -> int:
        if self.fail:
            raise RuntimeError("valkey is unreachable")
        for value in values:
            self.items.insert(0, value)
        return len(self.items)

    async def ltrim(self, name: str, start: int, end: int) -> None:
        if self.fail:
            raise RuntimeError("valkey is unreachable")
        self.items = self.items[start : end + 1]

    async def lrange(self, name: str, start: int, end: int) -> list[str]:
        if self.fail:
            raise RuntimeError("valkey is unreachable")
        return self.items[start : end + 1]

    async def ping(self) -> bool:
        if self.fail:
            raise RuntimeError("valkey is unreachable")
        return True

    async def aclose(self) -> None:
        return None


def recorder_client(readings_by_serial: dict[str, list[dict[str, Any]]], *, status: int = 200):
    """A ServiceClient wired to a stand-in sinusoidal-recorder."""

    def handler(request: httpx.Request) -> httpx.Response:
        if status != 200:
            return httpx.Response(status, json={"error": {"code": "boom", "message": "no"}})
        serial = request.url.params.get("serial", "")
        return httpx.Response(
            200, json={"serial": serial, "readings": readings_by_serial.get(serial, [])}
        )

    return ServiceClient(
        "http://sinusoidal-recorder:8080", transport=httpx.MockTransport(handler), retries=0
    )
