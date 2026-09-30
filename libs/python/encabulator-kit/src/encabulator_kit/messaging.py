"""NATS JetStream helpers, per design section 5.

One stream, ``ENCABULATOR``, subjects ``encabulator.>``, file storage, limits retention, max age
24h, 1 replica, duplicate window 2m. Every publisher and consumer creates the stream if it is
missing and never updates an existing one.

Publishers set ``Nats-Msg-Id`` to the event id (JetStream dedupes on it) and inject W3C trace
context into the message headers inside a ``PRODUCER`` span. Consumers extract that context and
handle the message inside a ``CONSUMER`` span, so the service graph links producer to consumer.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
from collections.abc import AsyncIterator, Awaitable, Callable, Mapping
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any

import nats
from nats.aio.client import Client as NatsClient
from nats.aio.msg import Msg
from nats.errors import TimeoutError as NatsTimeoutError
from nats.js import JetStreamContext
from nats.js.api import (
    AckPolicy,
    ConsumerConfig,
    DiscardPolicy,
    RetentionPolicy,
    StorageType,
    StreamConfig,
)
from nats.js.errors import NotFoundError
from opentelemetry import propagate, trace

from .config import env_str
from .ids import new_event_id
from .malaise import MalaiseController, MalaiseInjected

__all__ = [
    "STREAM_CONFIG",
    "STREAM_NAME",
    "STREAM_SUBJECTS",
    "Envelope",
    "EventBus",
    "PullConsumer",
    "Subjects",
    "connect",
    "ensure_stream",
    "open_bus",
]

log = logging.getLogger(__name__)

STREAM_NAME = "ENCABULATOR"
STREAM_SUBJECTS = ["encabulator.>"]

#: Exactly the stream config from design section 5. Do not vary it per service.
STREAM_CONFIG = StreamConfig(
    name=STREAM_NAME,
    subjects=list(STREAM_SUBJECTS),
    retention=RetentionPolicy.LIMITS,
    storage=StorageType.FILE,
    discard=DiscardPolicy.OLD,
    max_age=24 * 60 * 60,  # seconds
    num_replicas=1,
    duplicate_window=120,  # seconds
)

MSG_ID_HEADER = "Nats-Msg-Id"


class Subjects:
    """The subjects from design section 5."""

    ORDER_ACCEPTED = "encabulator.order.accepted"
    ORDER_SCRAPPED = "encabulator.order.scrapped"
    FABRICATION_REQUESTED = "encabulator.fabrication.requested"
    FABRICATION_STAGE = "encabulator.fabrication.stage"
    FABRICATION_COMPLETED = "encabulator.fabrication.completed"
    INSPECTION_PASSED = "encabulator.inspection.passed"
    INSPECTION_FAILED = "encabulator.inspection.failed"
    SHIPMENT_DISPATCHED = "encabulator.shipment.dispatched"
    SHIPMENT_DELIVERED = "encabulator.shipment.delivered"
    ALL = "encabulator.>"


@dataclass(frozen=True, slots=True)
class Envelope:
    """The event envelope from design section 5."""

    id: str
    type: str
    time: datetime
    source: str
    data: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def create(
        cls,
        event_type: str,
        data: Mapping[str, Any],
        source: str,
        *,
        event_id: str | None = None,
        time: datetime | None = None,
    ) -> Envelope:
        return cls(
            id=event_id or new_event_id(),
            type=event_type,
            time=time or datetime.now(UTC),
            source=source,
            data=dict(data),
        )

    def to_json(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "type": self.type,
            "time": self.time.astimezone(UTC)
            .isoformat(timespec="milliseconds")
            .replace("+00:00", "Z"),
            "source": self.source,
            "data": self.data,
        }

    def to_bytes(self) -> bytes:
        return json.dumps(self.to_json(), separators=(",", ":")).encode()

    @classmethod
    def from_bytes(cls, payload: bytes) -> Envelope:
        raw = json.loads(payload)
        when = raw.get("time")
        parsed = datetime.now(UTC)
        if isinstance(when, str):
            with contextlib.suppress(ValueError):
                parsed = datetime.fromisoformat(when.replace("Z", "+00:00"))
        return cls(
            id=str(raw.get("id", "")),
            type=str(raw.get("type", "")),
            time=parsed if parsed.tzinfo else parsed.replace(tzinfo=UTC),
            source=str(raw.get("source", "")),
            data=dict(raw.get("data") or {}),
        )


async def connect(url: str | None = None, *, name: str | None = None, **kwargs: Any) -> NatsClient:
    """Connect to NATS with the reconnect behaviour every service wants."""
    target = url or env_str("NATS_URL", "nats://nats:4222")
    options: dict[str, Any] = {
        "name": name,
        "max_reconnect_attempts": -1,
        "reconnect_time_wait": 1.0,
        "ping_interval": 10,
        "error_cb": _log_error,
        "disconnected_cb": _log_disconnected,
        "reconnected_cb": _log_reconnected,
    }
    options.update(kwargs)
    return await nats.connect(target, **options)


async def _log_error(exc: Exception) -> None:
    log.warning("nats error: %s", exc)


async def _log_disconnected() -> None:
    log.warning("nats disconnected")


async def _log_reconnected() -> None:
    log.info("nats reconnected")


async def ensure_stream(js: JetStreamContext) -> None:
    """Create the ENCABULATOR stream if it is missing. Never update an existing stream."""
    try:
        await js.stream_info(STREAM_NAME)
        return
    except NotFoundError:
        pass
    try:
        await js.add_stream(config=STREAM_CONFIG)
        log.info("created JetStream stream", extra={"stream": STREAM_NAME})
    except Exception as exc:  # another replica won the race
        log.debug("stream creation raced: %s", exc)
        await js.stream_info(STREAM_NAME)


class EventBus:
    """A NATS connection plus the JetStream context, with envelope publishing."""

    def __init__(self, client: NatsClient, source: str) -> None:
        self._client = client
        self._js = client.jetstream()
        self.source = source
        self._tracer = trace.get_tracer("encabulator.messaging")

    @property
    def client(self) -> NatsClient:
        return self._client

    @property
    def js(self) -> JetStreamContext:
        return self._js

    @property
    def connected(self) -> bool:
        return self._client.is_connected

    async def close(self) -> None:
        try:
            await self._client.drain()
        except Exception:  # pragma: no cover - shutdown is best effort
            await self._client.close()

    async def publish(
        self,
        subject: str,
        data: Mapping[str, Any],
        *,
        event_id: str | None = None,
        timeout: float = 5.0,
    ) -> Envelope:
        """Publish one envelope. The event type is the subject (design section 5)."""
        envelope = Envelope.create(subject, data, self.source, event_id=event_id)
        headers: dict[str, str] = {MSG_ID_HEADER: envelope.id}
        with self._tracer.start_as_current_span(
            f"{subject} publish",
            kind=trace.SpanKind.PRODUCER,
            attributes={
                "messaging.system": "nats",
                "messaging.destination.name": subject,
                "messaging.operation": "publish",
                "messaging.message.id": envelope.id,
            },
        ):
            propagate.inject(headers)
            await self._js.publish(
                subject,
                envelope.to_bytes(),
                headers=headers,
                timeout=timeout,
                stream=STREAM_NAME,
            )
        return envelope


async def open_bus(
    source: str, url: str | None = None, *, wait_seconds: float = 30.0, **kwargs: Any
) -> EventBus:
    """Connect, ensure the stream exists, and return the bus.

    A NATS that is still starting gets ``wait_seconds`` to appear -- a Helm install brings the
    application and its backing stores up together, and crash-looping through that is noise. One
    that never appears lets this raise, so the pod restarts with backoff rather than sitting there
    half alive.
    """
    loop = asyncio.get_running_loop()
    deadline = loop.time() + wait_seconds
    attempt = 0
    while True:
        try:
            # max_reconnect_attempts=-1 makes nats-py retry the *initial* connect forever, so the
            # first attempt is bounded here and the deadline is ours to enforce.
            client = await asyncio.wait_for(
                connect(url, name=source, **kwargs),
                timeout=max(1.0, min(5.0, deadline - loop.time())),
            )
            break
        except Exception as exc:
            attempt += 1
            if loop.time() >= deadline:
                log.error("nats unreachable after %d attempts", attempt)
                raise
            log.warning("nats not reachable yet (attempt %d): %s", attempt, exc)
            await asyncio.sleep(min(2.0, 0.5 * attempt))

    bus = EventBus(client, source)
    await ensure_stream(bus.js)
    return bus


Handler = Callable[[Envelope, Msg], Awaitable[None]]


class PullConsumer:
    """A durable pull consumer loop: explicit ack, AckWait 30s, MaxDeliver 5.

    ``concurrency`` messages are handled at once; each in-flight message gets in-progress acks
    every ``ack_wait / 2`` seconds so slow work does not get redelivered underneath us.
    """

    def __init__(
        self,
        bus: EventBus,
        *,
        durable: str,
        filter_subject: str,
        handler: Handler,
        concurrency: int = 1,
        max_ack_pending: int | None = None,
        ack_wait: float = 30.0,
        max_deliver: int = 5,
        fetch_timeout: float = 5.0,
        nak_delay: float = 1.0,
        malaise: MalaiseController | None = None,
    ) -> None:
        self.bus = bus
        self.durable = durable
        self.filter_subject = filter_subject
        self.handler = handler
        self.concurrency = max(1, concurrency)
        self.max_ack_pending = max_ack_pending or max(self.concurrency * 2, 64)
        self.ack_wait = ack_wait
        self.max_deliver = max_deliver
        self.fetch_timeout = fetch_timeout
        self.nak_delay = nak_delay
        self.malaise = malaise
        self._subscription: JetStreamContext.PullSubscription | None = None
        self._task: asyncio.Task[None] | None = None
        self._inflight: set[asyncio.Task[None]] = set()
        self._stopping = asyncio.Event()
        self._tracer = trace.get_tracer("encabulator.messaging")

    @property
    def inflight(self) -> int:
        return len(self._inflight)

    async def subscribe(self) -> None:
        await ensure_stream(self.bus.js)
        self._subscription = await self.bus.js.pull_subscribe(
            self.filter_subject,
            durable=self.durable,
            stream=STREAM_NAME,
            config=ConsumerConfig(
                durable_name=self.durable,
                filter_subject=self.filter_subject,
                ack_policy=AckPolicy.EXPLICIT,
                ack_wait=self.ack_wait,
                max_deliver=self.max_deliver,
                max_ack_pending=self.max_ack_pending,
            ),
        )
        log.info(
            "durable consumer ready",
            extra={"durable": self.durable, "filter_subject": self.filter_subject},
        )

    async def start(self) -> None:
        if self._task is not None:
            return
        await self.subscribe()
        self._task = asyncio.create_task(self.run(), name=f"consumer-{self.durable}")

    async def stop(self, *, drain_timeout: float = 20.0) -> None:
        self._stopping.set()
        if self._task is not None:
            self._task.cancel()
            with contextlib.suppress(asyncio.CancelledError, Exception):
                await self._task
            self._task = None
        if self._inflight:
            await asyncio.wait(set(self._inflight), timeout=drain_timeout)
        if self._subscription is not None:
            with contextlib.suppress(Exception):  # pragma: no cover
                await self._subscription.unsubscribe()
            self._subscription = None

    async def run(self) -> None:
        """Fetch-and-handle until stopped. Survives NATS hiccups by retrying."""
        assert self._subscription is not None, "call subscribe() first"
        while not self._stopping.is_set():
            capacity = self.concurrency - len(self._inflight)
            if capacity <= 0:
                await asyncio.wait(set(self._inflight), return_when=asyncio.FIRST_COMPLETED)
                continue
            try:
                messages = await self._subscription.fetch(
                    batch=capacity, timeout=self.fetch_timeout
                )
            except NatsTimeoutError, TimeoutError:
                continue
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                log.warning("jetstream fetch failed: %s", exc)
                await asyncio.sleep(1.0)
                continue
            for message in messages:
                task = asyncio.create_task(self._handle(message))
                self._inflight.add(task)
                task.add_done_callback(self._inflight.discard)

    async def _handle(self, message: Msg) -> None:
        try:
            envelope = Envelope.from_bytes(message.data)
        except Exception:
            log.error("undecodable message, terminating", exc_info=True)
            await _safe(message.term())
            return

        context = propagate.extract(message.headers or {})
        attributes = {
            "messaging.system": "nats",
            "messaging.destination.name": message.subject,
            "messaging.operation": "process",
            "messaging.message.id": envelope.id,
            "messaging.consumer.group.name": self.durable,
        }
        keepalive = asyncio.create_task(self._in_progress(message))
        try:
            with self._tracer.start_as_current_span(
                f"{message.subject} process",
                context=context,
                kind=trace.SpanKind.CONSUMER,
                attributes=attributes,
            ) as span:
                try:
                    if self.malaise is not None:
                        async with self.malaise.guard():
                            await self.handler(envelope, message)
                    else:
                        await self.handler(envelope, message)
                except MalaiseInjected as injected:
                    span.set_attribute("encabulator.malaise", injected.code)
                    log.info(
                        "naking message under injected malaise",
                        extra={"event_type": envelope.type, "malaise": injected.code},
                    )
                    await _safe(message.nak(delay=self.nak_delay))
                    return
                except Exception as exc:
                    span.record_exception(exc)
                    span.set_status(trace.Status(trace.StatusCode.ERROR, str(exc)))
                    log.error(
                        "message handler failed",
                        exc_info=True,
                        extra={"event_type": envelope.type, "event_id": envelope.id},
                    )
                    await _safe(message.nak(delay=self.nak_delay))
                    return
                await _safe(message.ack())
        finally:
            keepalive.cancel()

    async def _in_progress(self, message: Msg) -> None:
        interval = max(1.0, self.ack_wait / 2)
        try:
            while True:
                await asyncio.sleep(interval)
                await message.in_progress()
        except asyncio.CancelledError:
            return
        except Exception:  # pragma: no cover - the message may already be acked
            return


async def _safe(awaitable: Awaitable[Any]) -> None:
    try:
        await awaitable
    except Exception as exc:  # pragma: no cover - ack/nak races are not fatal
        log.debug("ack/nak failed: %s", exc)


async def nats_readiness(bus: EventBus) -> None:
    """Readiness check: raises when the NATS connection is down."""
    if not bus.connected:
        raise RuntimeError("nats is not connected")


async def iter_messages(
    subscription: JetStreamContext.PullSubscription, *, batch: int = 1, timeout: float = 1.0
) -> AsyncIterator[Msg]:
    """Small helper for tests: yield whatever is available right now."""
    try:
        for message in await subscription.fetch(batch=batch, timeout=timeout):
            yield message
    except NatsTimeoutError, TimeoutError:
        return
