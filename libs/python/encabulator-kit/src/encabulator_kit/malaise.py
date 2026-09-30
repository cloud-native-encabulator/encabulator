"""Malaise (fault injection) middleware, per design section 7.

Every service in the processing, data, and support planes applies injected faults to inbound
HTTP requests (health and metrics excepted) and to message handling. Directives are polled
from the malaise injection engine every 2 seconds, untraced. When the engine is unreachable we
keep applying the directives we already have until each one's ``expiresAt``.

The same :meth:`MalaiseController.guard` drives both paths. HTTP turns an injected fault into a
response; a message handler turns it into a ``nak``.
"""

from __future__ import annotations

import asyncio
import contextlib
import functools
import json
import logging
import random
import threading
import time
from collections.abc import AsyncIterator, Awaitable, Callable, Mapping, Sequence
from contextlib import AsyncExitStack, asynccontextmanager
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any

import httpx

from .config import env_str
from .metrics import MALAISE_APPLIED
from .telemetry import untraced

__all__ = [
    "CpuBurner",
    "Directive",
    "MalaiseCongested",
    "MalaiseController",
    "MalaiseInjected",
    "MalaiseMiddleware",
]

log = logging.getLogger(__name__)

#: Paths that never see injected malaise.
EXCLUDED_PATHS: frozenset[str] = frozenset({"/healthz", "/readyz", "/metrics"})


class MalaiseInjected(Exception):  # noqa: N818 - reads as a control-flow signal
    """Raised by :meth:`MalaiseController.guard` when a fault short-circuits the work."""

    def __init__(
        self,
        status_code: int = 503,
        code: str = "malaise_injected",
        message: str = "The encabulator is experiencing induced malaise",
        *,
        directive_id: str | None = None,
    ) -> None:
        self.status_code = status_code
        self.code = code
        self.message = message
        self.directive_id = directive_id
        super().__init__(f"{code}: {message}")


class MalaiseCongested(MalaiseInjected):
    """Congestion queue timed out."""

    def __init__(self, *, directive_id: str | None = None) -> None:
        super().__init__(
            503,
            "congested",
            "Too many encabulations in flight; the queue timed out",
            directive_id=directive_id,
        )


@dataclass(frozen=True, slots=True)
class Directive:
    """One active fault, as returned by ``GET /v1/directives``."""

    id: str
    kind: str
    params: Mapping[str, Any]
    expires_at: datetime | None = None

    def expired(self, now: datetime | None = None) -> bool:
        if self.expires_at is None:
            return False
        return (now or datetime.now(UTC)) >= self.expires_at

    def number(self, key: str, default: float) -> float:
        raw = self.params.get(key, default)
        try:
            return float(raw)
        except TypeError, ValueError:
            return default

    def integer(self, key: str, default: int) -> int:
        return int(self.number(key, default))

    @property
    def percent(self) -> float:
        return max(0.0, min(100.0, self.number("percent", 100.0)))

    @classmethod
    def from_json(cls, payload: Mapping[str, Any]) -> Directive:
        return cls(
            id=str(payload.get("id", "")),
            kind=str(payload.get("kind", "")),
            params=dict(payload.get("params") or {}),
            expires_at=_parse_time(payload.get("expiresAt")),
        )


def _parse_time(raw: Any) -> datetime | None:
    if not raw:
        return None
    try:
        parsed = datetime.fromisoformat(str(raw).replace("Z", "+00:00"))
    except ValueError:
        return None
    return parsed if parsed.tzinfo else parsed.replace(tzinfo=UTC)


class CpuBurner:
    """Burns roughly ``millicores`` of CPU in background threads until told to stop.

    Each thread duty-cycles over a 10 ms window, so it yields the GIL often enough that the
    event loop still makes progress -- the service gets slower, it does not die.
    """

    _WINDOW = 0.01

    def __init__(self) -> None:
        self._threads: list[tuple[threading.Thread, threading.Event]] = []
        self._millicores = 0
        self._lock = threading.Lock()

    @property
    def millicores(self) -> int:
        return self._millicores

    def set_load(self, millicores: int) -> None:
        with self._lock:
            if millicores == self._millicores:
                return
            self._stop_locked()
            self._millicores = max(0, millicores)
            remaining = self._millicores
            while remaining > 0:
                share = min(1000, remaining)
                remaining -= share
                stop = threading.Event()
                thread = threading.Thread(
                    target=self._burn,
                    args=(stop, share / 1000.0),
                    name="malaise-cpu-burn",
                    daemon=True,
                )
                thread.start()
                self._threads.append((thread, stop))

    def stop(self) -> None:
        with self._lock:
            self._stop_locked()
            self._millicores = 0

    def _stop_locked(self) -> None:
        for _, stop in self._threads:
            stop.set()
        for thread, _ in self._threads:
            thread.join(timeout=1.0)
        self._threads.clear()

    @classmethod
    def _burn(cls, stop: threading.Event, duty: float) -> None:
        busy = cls._WINDOW * min(1.0, max(0.0, duty))
        idle = cls._WINDOW - busy
        acc = 0.0
        while not stop.is_set():
            deadline = time.perf_counter() + busy
            while time.perf_counter() < deadline:
                # A deliberately un-elidable float grind: sinusoidal repleneration, by hand.
                acc = (acc + 1.000_000_1) % 1_000_003.0
            if idle > 0:
                stop.wait(idle)


class MalaiseController:
    """Polls the engine for directives and applies them."""

    def __init__(
        self,
        service: str,
        engine_url: str | None = None,
        *,
        poll_interval: float = 2.0,
        timeout: float = 2.0,
        transport: httpx.AsyncBaseTransport | None = None,
        rng: random.Random | None = None,
    ) -> None:
        self.service = service
        self.engine_url = engine_url if engine_url is not None else env_str("MALAISE_ENGINE_URL")
        if self.engine_url:
            self.engine_url = self.engine_url.rstrip("/")
        self.poll_interval = poll_interval
        self.timeout = timeout
        self._transport = transport
        self._rng = rng or random.Random()
        self._directives: tuple[Directive, ...] = ()
        self._client: httpx.AsyncClient | None = None
        self._task: asyncio.Task[None] | None = None
        self._burner = CpuBurner()
        self._congestion: tuple[str, int, asyncio.Semaphore] | None = None

    # ---- lifecycle ---------------------------------------------------------------------

    @property
    def enabled(self) -> bool:
        return bool(self.engine_url)

    async def start(self) -> None:
        if not self.enabled or self._task is not None:
            return
        self._client = httpx.AsyncClient(
            base_url=self.engine_url or "",
            timeout=httpx.Timeout(self.timeout),
            transport=self._transport,
        )
        await self.refresh(log_failure=False)
        self._task = asyncio.create_task(self._poll_loop(), name="malaise-poller")
        log.info("malaise middleware armed", extra={"malaise_engine_url": self.engine_url})

    async def stop(self) -> None:
        if self._task is not None:
            self._task.cancel()
            with contextlib.suppress(asyncio.CancelledError, Exception):
                await self._task
            self._task = None
        if self._client is not None:
            await self._client.aclose()
            self._client = None
        self._burner.stop()

    async def _poll_loop(self) -> None:
        while True:
            await asyncio.sleep(self.poll_interval)
            await self.refresh()

    async def refresh(self, *, log_failure: bool = True) -> bool:
        """Fetch directives. On failure keep the last good set (each expires on its own)."""
        fetched = False
        if self._client is not None:
            try:
                with untraced():
                    response = await self._client.get(
                        "/v1/directives", params={"service": self.service}
                    )
                response.raise_for_status()
                payload = response.json()
                self._directives = tuple(
                    Directive.from_json(item) for item in payload.get("directives", [])
                )
                fetched = True
            except Exception as exc:
                if log_failure:
                    log.debug("malaise directive poll failed: %s", exc)
        self._prune()
        self._reconcile_cpu_burn()
        return fetched

    def _prune(self) -> None:
        now = datetime.now(UTC)
        live = tuple(d for d in self._directives if not d.expired(now))
        if len(live) != len(self._directives):
            self._directives = live
        if self._congestion is not None and not any(
            d.id == self._congestion[0] for d in self._directives
        ):
            self._congestion = None

    def _reconcile_cpu_burn(self) -> None:
        total = sum(d.integer("millicores", 0) for d in self._directives if d.kind == "cpu-burn")
        if total != self._burner.millicores:
            if total > 0:
                MALAISE_APPLIED.labels("cpu-burn").inc()
            self._burner.set_load(total)

    # ---- inspection --------------------------------------------------------------------

    def directives(self) -> Sequence[Directive]:
        self._prune()
        return self._directives

    def directive(self, kind: str) -> Directive | None:
        for candidate in self.directives():
            if candidate.kind == kind:
                return candidate
        return None

    def set_directives(self, directives: Sequence[Directive]) -> None:
        """Set directives directly. Used by tests and by services with a local override."""
        self._directives = tuple(directives)
        self._prune()
        self._reconcile_cpu_burn()

    # ---- application -------------------------------------------------------------------

    @asynccontextmanager
    async def guard(self) -> AsyncIterator[None]:
        """Apply congestion, latency, and error to one unit of work.

        Raises :class:`MalaiseInjected` when the work must not run at all.
        """
        if not self._directives:
            yield
            return
        async with AsyncExitStack() as stack:
            congestion = self.directive("congestion")
            if congestion is not None:
                await stack.enter_async_context(self._hold(congestion))
            await self._inject_latency()
            self._inject_error()
            yield

    def wrap_message_handler[**P, R](
        self, handler: Callable[P, Awaitable[R]]
    ) -> Callable[P, Awaitable[R]]:
        """Wrap a message handler so injected malaise applies to message handling too."""

        @functools.wraps(handler)
        async def wrapped(*args: P.args, **kwargs: P.kwargs) -> R:
            async with self.guard():
                return await handler(*args, **kwargs)

        return wrapped

    def _rolls(self, percent: float) -> bool:
        return percent >= 100.0 or self._rng.random() * 100.0 < percent

    async def _inject_latency(self) -> None:
        directive = self.directive("latency")
        if directive is None or not self._rolls(directive.percent):
            return
        base = directive.number("ms", 0.0)
        jitter = directive.number("jitterMs", 0.0)
        delay = max(0.0, base + self._rng.uniform(-jitter, jitter)) / 1000.0
        MALAISE_APPLIED.labels("latency").inc()
        await asyncio.sleep(delay)

    def _inject_error(self) -> None:
        directive = self.directive("error")
        if directive is None or not self._rolls(directive.percent):
            return
        status = directive.integer("status", 503)
        MALAISE_APPLIED.labels("error").inc()
        raise MalaiseInjected(status, directive_id=directive.id)

    @asynccontextmanager
    async def _hold(self, directive: Directive) -> AsyncIterator[None]:
        max_concurrent = max(1, directive.integer("maxConcurrent", 1))
        timeout = max(0.0, directive.number("queueTimeoutMs", 0.0)) / 1000.0
        if self._congestion is None or self._congestion[:2] != (directive.id, max_concurrent):
            self._congestion = (directive.id, max_concurrent, asyncio.Semaphore(max_concurrent))
        semaphore = self._congestion[2]
        MALAISE_APPLIED.labels("congestion").inc()
        try:
            await asyncio.wait_for(semaphore.acquire(), timeout=timeout or None)
        except TimeoutError as exc:
            raise MalaiseCongested(directive_id=directive.id) from exc
        try:
            yield
        finally:
            semaphore.release()


class MalaiseMiddleware:
    """Pure-ASGI middleware applying malaise to inbound HTTP requests."""

    def __init__(
        self,
        app: Any,
        controller: MalaiseController,
        excluded_paths: frozenset[str] = EXCLUDED_PATHS,
    ) -> None:
        self.app = app
        self.controller = controller
        self.excluded_paths = excluded_paths

    async def __call__(self, scope: Any, receive: Any, send: Any) -> None:
        if (
            scope["type"] != "http"
            or not self.controller.enabled
            or scope.get("path") in self.excluded_paths
        ):
            await self.app(scope, receive, send)
            return
        try:
            async with self.controller.guard():
                await self.app(scope, receive, send)
        except MalaiseInjected as injected:
            await _send_error(send, injected)


async def _send_error(send: Any, injected: MalaiseInjected) -> None:
    body = json.dumps({"error": {"code": injected.code, "message": injected.message}}).encode()
    await send(
        {
            "type": "http.response.start",
            "status": injected.status_code,
            "headers": [
                (b"content-type", b"application/json"),
                (b"content-length", str(len(body)).encode()),
            ],
        }
    )
    await send({"type": "http.response.body", "body": body})
