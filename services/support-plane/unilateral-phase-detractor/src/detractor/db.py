"""Database wiring: an async engine, a session factory, and a readiness ping."""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator
from typing import Any

from sqlalchemy import text
from sqlalchemy.engine import URL, make_url
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

log = logging.getLogger(__name__)

#: libpq sslmode values. asyncpg takes the same names with the same meaning, so pass them
#: through: ``require`` encrypts without verifying the server certificate (the libpq and pgx
#: behaviour the other services get), ``verify-full`` verifies it against the system roots.
#: Mapping ``require`` to ``ssl=True`` would verify too, and the Amazon RDS CA is not in the
#: system trust store, so the detractor could not reach RDS while every other service could.
_SSLMODES = frozenset({"disable", "allow", "prefer", "require", "verify-ca", "verify-full"})


def async_database_url(raw: str) -> tuple[URL, dict[str, Any]]:
    """Turn a libpq-style ``DATABASE_URL`` into an asyncpg URL plus connect args."""
    url = make_url(raw)
    if url.drivername in {"postgres", "postgresql"}:
        url = url.set(drivername="postgresql+asyncpg")
    elif url.drivername == "sqlite":
        url = url.set(drivername="sqlite+aiosqlite")

    query = dict(url.query)
    connect_args: dict[str, Any] = {}
    sslmode = query.pop("sslmode", None)
    if sslmode is not None and url.drivername.endswith("asyncpg"):
        mode = str(sslmode)
        if mode not in _SSLMODES:
            raise ValueError(f"unsupported sslmode {mode!r}")
        connect_args["ssl"] = mode
    return url.set(query=query), connect_args


def create_engine(database_url: str, *, echo: bool = False) -> AsyncEngine:
    url, connect_args = async_database_url(database_url)
    engine = create_async_engine(
        url,
        echo=echo,
        pool_pre_ping=True,
        pool_size=5,
        max_overflow=5,
        pool_recycle=1800,
        connect_args=connect_args,
    )
    from encabulator_kit.telemetry import instrument_sqlalchemy_engine

    instrument_sqlalchemy_engine(engine)
    log.info("database engine ready", extra={"db_name": url.database, "db_driver": url.drivername})
    return engine


def create_session_factory(engine: AsyncEngine) -> async_sessionmaker[AsyncSession]:
    return async_sessionmaker(engine, expire_on_commit=False, autoflush=False)


async def ping(engine: AsyncEngine) -> None:
    """Readiness check: Postgres answers. Raises on failure."""
    async with engine.connect() as connection:
        await connection.execute(text("SELECT 1"))


async def session_scope(
    factory: async_sessionmaker[AsyncSession],
) -> AsyncIterator[AsyncSession]:
    """One session per request; commit on success, roll back on any error."""
    async with factory() as session:
        try:
            yield session
            await session.commit()
        except Exception:
            await session.rollback()
            raise
