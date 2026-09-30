from __future__ import annotations

from detractor.ratelimit import RateLimiter


def test_burst_then_refill() -> None:
    limiter = RateLimiter(per_minute=6)  # 0.1/s, burst 6
    assert [limiter.allow("ip", now=0.0) for _ in range(6)] == [True] * 6
    assert limiter.allow("ip", now=0.0) is False
    assert limiter.allow("ip", now=10.0) is True  # one token back after 10s


def test_buckets_are_per_key() -> None:
    limiter = RateLimiter(per_minute=1)
    assert limiter.allow("a", now=0.0) is True
    assert limiter.allow("a", now=0.0) is False
    assert limiter.allow("b", now=0.0) is True


def test_zero_rate_blocks_everything() -> None:
    assert RateLimiter(per_minute=0).allow("ip") is False


def test_bucket_map_is_bounded() -> None:
    limiter = RateLimiter(per_minute=10, capacity=8)
    for index in range(100):
        limiter.allow(f"ip-{index}", now=float(index))
    assert len(limiter._buckets) <= 8


def test_retry_after_is_at_least_one_second() -> None:
    limiter = RateLimiter(per_minute=60)
    limiter.allow("ip", now=0.0)
    assert limiter.retry_after("ip") >= 1
    assert limiter.retry_after("never-seen") == 60
