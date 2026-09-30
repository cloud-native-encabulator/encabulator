"""The two services the line calls: the recorder it writes readings to, and the depot whose
reservation it commits when a first attempt finishes."""

from __future__ import annotations

import logging
from collections.abc import Sequence
from datetime import UTC, datetime, timedelta
from typing import Any

from encabulator_kit import ServiceClient, UpstreamError

from .signals import Sample

log = logging.getLogger(__name__)


def readings_payload(
    *,
    serial: str,
    order_id: str,
    model: str,
    stage: str,
    samples: Sequence[Sample],
    base_time: datetime | None = None,
) -> dict[str, Any]:
    """Build the batch body for ``POST sinusoidal-recorder /v1/readings``."""
    start = base_time or datetime.now(UTC)
    return {
        "readings": [
            {
                "serial": serial,
                "orderId": order_id,
                "model": model,
                "stage": stage,
                "metric": sample.metric,
                "value": sample.value,
                "time": _iso(start + timedelta(milliseconds=sample.offset_ms)),
            }
            for sample in samples
        ]
    }


def _iso(moment: datetime) -> str:
    return moment.astimezone(UTC).isoformat(timespec="milliseconds").replace("+00:00", "Z")


class Recorder:
    """sinusoidal-recorder. Losing readings would blind the detector, so a failed write is fatal
    to the message: the line raises and the consumer naks, and the order is refabricated."""

    def __init__(self, client: ServiceClient | None) -> None:
        self._client = client

    @property
    def enabled(self) -> bool:
        return self._client is not None

    async def write(self, payload: dict[str, Any], *, idempotency_key: str) -> None:
        if self._client is None:
            log.debug("SINUSOIDAL_RECORDER_URL is unset; discarding readings")
            return
        await self._client.json(
            "POST", "/v1/readings", json=payload, idempotency_key=idempotency_key
        )

    async def aclose(self) -> None:
        if self._client is not None:
            await self._client.aclose()


class Depot:
    """prefabulated-amulite-depot. Committing is bookkeeping: if it fails the parts stay reserved
    and the depot's sweeper releases them, so a failure is logged, not fatal."""

    def __init__(self, client: ServiceClient | None) -> None:
        self._client = client

    @property
    def enabled(self) -> bool:
        return self._client is not None

    async def commit(self, reservation_id: str) -> bool:
        if self._client is None or not reservation_id:
            return False
        try:
            await self._client.json(
                "POST",
                f"/v1/reservations/{reservation_id}/commit",
                idempotency_key=reservation_id,
            )
            return True
        except UpstreamError as exc:
            log.warning(
                "could not commit reservation",
                extra={"reservation_id": reservation_id, "code": exc.code},
            )
            return False

    async def aclose(self) -> None:
        if self._client is not None:
            await self._client.aclose()
