from __future__ import annotations

import httpx
import pytest

from encabulator_kit.flags import (
    DEFAULT_FLAGS,
    STRICT_SIDE_FUMBLING,
    TURBO_ENCABULATION,
    Flag,
    FlagClient,
    fnv1a32,
    in_rollout,
)


def test_fnv1a32_known_vectors() -> None:
    # Reference values for FNV-1a 32-bit.
    assert fnv1a32("") == 0x811C9DC5
    assert fnv1a32("a") == 0xE40C292C
    assert fnv1a32("foobar") == 0xBF9CF968


def test_rollout_is_deterministic_and_spread() -> None:
    subjects = [f"cust-{i}" for i in range(1000)]
    included = [s for s in subjects if in_rollout(TURBO_ENCABULATION, s, 30)]
    assert 200 < len(included) < 400  # roughly 30%
    # Same subject, same answer, every time.
    assert all(in_rollout(TURBO_ENCABULATION, s, 30) for s in included)


def test_rollout_edges() -> None:
    assert in_rollout("k", "s", 100) is True
    assert in_rollout("k", "s", 0) is False


def test_defaults_used_when_disabled() -> None:
    client = FlagClient(None)
    assert client.enabled is False
    assert client.bool(TURBO_ENCABULATION) is False
    assert client.bool("quote-cache") is True
    assert client.number("amulite-foundry-rate") == 20.0
    assert client.number("gateway-rate-limit-rps") == 50.0
    assert client.number("dingle-arm-damping") == 50.0
    assert set(DEFAULT_FLAGS) == {
        "turbo-encabulation",
        "quote-cache",
        "retro-encabulator-sales",
        "strict-side-fumbling",
        "amulite-foundry-rate",
        "gateway-rate-limit-rps",
        "dingle-arm-damping",
    }


def _flags_payload(**overrides: object) -> dict[str, object]:
    flags = [
        {"key": TURBO_ENCABULATION, "type": "boolean", "value": True},
        {"key": STRICT_SIDE_FUMBLING, "type": "boolean", "value": False},
        {"key": "dingle-arm-damping", "type": "number", "value": 75},
    ]
    flags.extend(overrides.get("extra", []))  # type: ignore[arg-type]
    return {"flags": flags}


class _Girdlespring:
    """A stand-in girdlespring that counts requests and honours If-None-Match."""

    def __init__(self, payload: dict[str, object], etag: str = '"v1"') -> None:
        self.payload = payload
        self.etag = etag
        self.requests: list[dict[str, str]] = []
        self.fail = False

    def handler(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(dict(request.headers))
        if self.fail:
            raise httpx.ConnectError("girdlespring is unreachable")
        if request.headers.get("if-none-match") == self.etag:
            return httpx.Response(304, headers={"ETag": self.etag})
        return httpx.Response(200, json=self.payload, headers={"ETag": self.etag})

    @property
    def transport(self) -> httpx.MockTransport:
        return httpx.MockTransport(self.handler)


async def test_fetches_and_serves_flags() -> None:
    girdlespring = _Girdlespring(_flags_payload())
    client = FlagClient("http://girdlespring:8080", transport=girdlespring.transport)
    await client.start()
    try:
        assert client.loaded is True
        assert client.bool(TURBO_ENCABULATION) is True
        assert client.number("dingle-arm-damping") == 75.0
        # A key the service did not return still answers from the built-in defaults.
        assert client.bool("quote-cache") is True
    finally:
        await client.stop()


async def test_etag_polling_sends_if_none_match() -> None:
    girdlespring = _Girdlespring(_flags_payload())
    client = FlagClient(
        "http://girdlespring:8080", transport=girdlespring.transport, poll_interval=0.01
    )
    await client.start()
    try:
        assert await client.refresh() is True
    finally:
        await client.stop()
    assert "if-none-match" not in girdlespring.requests[0]
    assert girdlespring.requests[-1]["if-none-match"] == '"v1"'


async def test_keeps_last_good_set_when_unreachable() -> None:
    girdlespring = _Girdlespring(_flags_payload())
    client = FlagClient("http://girdlespring:8080", transport=girdlespring.transport)
    await client.start()
    try:
        assert client.bool(TURBO_ENCABULATION) is True
        girdlespring.fail = True
        assert await client.refresh() is False
        assert client.bool(TURBO_ENCABULATION) is True  # last good value survives
    finally:
        await client.stop()


async def test_falls_back_to_defaults_when_unreachable_at_startup() -> None:
    girdlespring = _Girdlespring(_flags_payload())
    girdlespring.fail = True
    client = FlagClient("http://girdlespring:8080", transport=girdlespring.transport)
    await client.start()
    try:
        assert client.loaded is False
        assert client.bool(TURBO_ENCABULATION) is False  # the built-in default
    finally:
        await client.stop()


@pytest.mark.parametrize(
    ("value", "rollout", "subject", "expected"),
    [
        (True, None, None, True),
        (False, None, None, False),
        (True, 100, "cust-1", True),
        (True, 0, "cust-1", False),
        (False, 100, "cust-1", False),
        (True, 50, None, False),  # a partial rollout with nobody to bucket applies to nobody
    ],
)
def test_flag_evaluation(
    value: bool, rollout: float | None, subject: str | None, expected: bool
) -> None:
    flag = Flag(key="turbo-encabulation", type="boolean", value=value, rollout_percent=rollout)
    assert flag.evaluate(subject) is expected
