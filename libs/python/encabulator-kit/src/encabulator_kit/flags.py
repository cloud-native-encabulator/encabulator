"""differential-girdlespring flag client, per design section 6.

Poll ``GET /v1/flags`` every 5 seconds with ``If-None-Match``, keep the last good set, and fall
back to the built-in defaults when the service is unreachable at startup. Boolean flags may
carry a ``rolloutPercent``; evaluation for a subject is
``fnv1a32(key + ":" + subject) % 100 < rolloutPercent``.
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any

import httpx

from .config import env_str
from .telemetry import untraced

__all__ = [
    "AMULITE_FOUNDRY_RATE",
    "DEFAULT_FLAGS",
    "DINGLE_ARM_DAMPING",
    "GATEWAY_RATE_LIMIT_RPS",
    "QUOTE_CACHE",
    "RETRO_ENCABULATOR_SALES",
    "STRICT_SIDE_FUMBLING",
    "TURBO_ENCABULATION",
    "Flag",
    "FlagClient",
    "fnv1a32",
    "in_rollout",
]

log = logging.getLogger(__name__)

TURBO_ENCABULATION = "turbo-encabulation"
QUOTE_CACHE = "quote-cache"
RETRO_ENCABULATOR_SALES = "retro-encabulator-sales"
STRICT_SIDE_FUMBLING = "strict-side-fumbling"
AMULITE_FOUNDRY_RATE = "amulite-foundry-rate"
GATEWAY_RATE_LIMIT_RPS = "gateway-rate-limit-rps"
DINGLE_ARM_DAMPING = "dingle-arm-damping"

#: Built-in defaults (design section 6). Used until the first successful poll, and forever if
#: DIFFERENTIAL_GIRDLESPRING_URL is unset.
DEFAULT_FLAGS: Mapping[str, bool | float] = {
    TURBO_ENCABULATION: False,
    QUOTE_CACHE: True,
    RETRO_ENCABULATOR_SALES: True,
    STRICT_SIDE_FUMBLING: False,
    AMULITE_FOUNDRY_RATE: 20.0,
    GATEWAY_RATE_LIMIT_RPS: 50.0,
    DINGLE_ARM_DAMPING: 50.0,
}

_FNV_OFFSET = 0x811C9DC5
_FNV_PRIME = 0x01000193
_MASK32 = 0xFFFFFFFF


def fnv1a32(text: str) -> int:
    """32-bit FNV-1a over the UTF-8 bytes of ``text``."""
    value = _FNV_OFFSET
    for byte in text.encode("utf-8"):
        value ^= byte
        value = (value * _FNV_PRIME) & _MASK32
    return value


def in_rollout(key: str, subject: str, percent: float) -> bool:
    if percent <= 0:
        return False
    if percent >= 100:
        return True
    return fnv1a32(f"{key}:{subject}") % 100 < percent


@dataclass(frozen=True, slots=True)
class Flag:
    key: str
    type: str
    value: Any
    rollout_percent: float | None = None
    description: str = ""
    updated_at: str | None = None
    updated_by: str | None = None

    @classmethod
    def from_json(cls, payload: Mapping[str, Any]) -> Flag:
        rollout = payload.get("rolloutPercent")
        return cls(
            key=str(payload.get("key", "")),
            type=str(payload.get("type", "")),
            value=payload.get("value"),
            rollout_percent=None if rollout is None else float(rollout),
            description=str(payload.get("description") or ""),
            updated_at=payload.get("updatedAt"),
            updated_by=payload.get("updatedBy"),
        )

    def evaluate(self, subject: str | None = None) -> Any:
        """Evaluate a boolean flag for a subject; non-boolean flags return their value."""
        if not isinstance(self.value, bool) or self.rollout_percent is None:
            return self.value
        if not self.value:
            return False
        if subject is None:
            # No subject to bucket: only a full rollout applies to everyone.
            return self.rollout_percent >= 100
        return in_rollout(self.key, subject, self.rollout_percent)


class FlagClient:
    """Polls girdlespring and answers flag questions from the last good set."""

    def __init__(
        self,
        base_url: str | None = None,
        *,
        defaults: Mapping[str, bool | float] | None = None,
        poll_interval: float = 5.0,
        timeout: float = 2.0,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        url = base_url if base_url is not None else env_str("DIFFERENTIAL_GIRDLESPRING_URL")
        self.base_url = url.rstrip("/") if url else None
        self.defaults: dict[str, bool | float] = dict(defaults or DEFAULT_FLAGS)
        self.poll_interval = poll_interval
        self.timeout = timeout
        self._transport = transport
        self._flags: dict[str, Flag] = {}
        self._etag: str | None = None
        self._client: httpx.AsyncClient | None = None
        self._task: asyncio.Task[None] | None = None
        self._loaded = False

    @property
    def enabled(self) -> bool:
        return self.base_url is not None

    @property
    def loaded(self) -> bool:
        """True once a poll has succeeded at least once."""
        return self._loaded

    async def start(self) -> None:
        if not self.enabled or self._task is not None:
            return
        self._client = httpx.AsyncClient(
            base_url=self.base_url or "",
            timeout=httpx.Timeout(self.timeout),
            transport=self._transport,
        )
        if not await self.refresh(log_failure=False):
            log.warning("girdlespring unreachable at startup; using built-in flag defaults")
        self._task = asyncio.create_task(self._poll_loop(), name="flag-poller")

    async def stop(self) -> None:
        if self._task is not None:
            self._task.cancel()
            with contextlib.suppress(asyncio.CancelledError, Exception):
                await self._task
            self._task = None
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def __aenter__(self) -> FlagClient:
        await self.start()
        return self

    async def __aexit__(self, *exc: object) -> None:
        await self.stop()

    async def _poll_loop(self) -> None:
        while True:
            await asyncio.sleep(self.poll_interval)
            await self.refresh()

    async def refresh(self, *, log_failure: bool = True) -> bool:
        if self._client is None:
            return False
        headers = {"If-None-Match": self._etag} if self._etag else {}
        try:
            with untraced():
                response = await self._client.get("/v1/flags", headers=headers)
            if response.status_code == 304:
                return True
            response.raise_for_status()
            payload = response.json()
            self._flags = {
                flag.key: flag
                for flag in (Flag.from_json(item) for item in payload.get("flags", []))
                if flag.key
            }
            self._etag = response.headers.get("ETag")
            self._loaded = True
            return True
        except Exception as exc:
            if log_failure:
                log.debug("flag poll failed: %s", exc)
            return False

    # ---- reads -------------------------------------------------------------------------

    def snapshot(self) -> dict[str, Flag]:
        return dict(self._flags)

    def get(self, key: str, subject: str | None = None) -> Any:
        flag = self._flags.get(key)
        if flag is None:
            return self.defaults.get(key)
        return flag.evaluate(subject)

    def bool(self, key: str, subject: str | None = None) -> bool:
        value = self.get(key, subject)
        if isinstance(value, bool):
            return value
        default = self.defaults.get(key)
        return bool(default) if value is None else bool(value)

    def number(self, key: str) -> float:
        value = self.get(key)
        try:
            return float(value)  # type: ignore[arg-type]
        except TypeError, ValueError:
            return float(self.defaults.get(key, 0.0))  # type: ignore[arg-type]
