"""Versioned SQL migrations applied at startup.

A deliberately small in-house runner: an ordered list of statements per version, a
``schema_migrations`` ledger, and a Postgres advisory lock so two replicas starting at once do
not race. The SQL is plain enough to run on SQLite too, which is what the containerless tests use.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

log = logging.getLogger(__name__)

#: A stable, arbitrary key for pg_advisory_xact_lock.
_ADVISORY_LOCK_KEY = 0x0DE7_AC70  # "detractor", squinting


@dataclass(frozen=True, slots=True)
class Migration:
    version: int
    name: str
    statements: tuple[str, ...]


MIGRATIONS: tuple[Migration, ...] = (
    Migration(
        1,
        "users",
        (
            """
            CREATE TABLE IF NOT EXISTS users (
                username      VARCHAR(64) PRIMARY KEY,
                display_name  VARCHAR(128) NOT NULL DEFAULT '',
                password_hash TEXT NOT NULL,
                roles         TEXT NOT NULL DEFAULT '',
                disabled      BOOLEAN NOT NULL DEFAULT FALSE,
                created_at    TIMESTAMP WITH TIME ZONE NOT NULL,
                updated_at    TIMESTAMP WITH TIME ZONE NOT NULL
            )
            """,
        ),
    ),
    Migration(
        2,
        "audit_log",
        (
            """
            CREATE TABLE IF NOT EXISTS audit_log (
                id      VARCHAR(40) PRIMARY KEY,
                time    TIMESTAMP WITH TIME ZONE NOT NULL,
                action  VARCHAR(64) NOT NULL,
                actor   VARCHAR(64) NOT NULL DEFAULT '',
                subject VARCHAR(64) NOT NULL DEFAULT '',
                result  VARCHAR(16) NOT NULL DEFAULT 'success',
                detail  TEXT NOT NULL DEFAULT '{}'
            )
            """,
            "CREATE INDEX IF NOT EXISTS audit_log_time_idx ON audit_log (time DESC)",
        ),
    ),
)

_LEDGER = """
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       VARCHAR(64) NOT NULL,
    applied_at TIMESTAMP WITH TIME ZONE NOT NULL
)
"""


async def migrate(engine: AsyncEngine) -> list[int]:
    """Apply every migration the database has not seen. Returns the versions applied."""
    applied: list[int] = []
    is_postgres = engine.dialect.name == "postgresql"

    async with engine.begin() as connection:
        await connection.execute(text(_LEDGER))

    async with engine.begin() as connection:
        if is_postgres:
            await connection.execute(
                text("SELECT pg_advisory_xact_lock(:key)"), {"key": _ADVISORY_LOCK_KEY}
            )
        result = await connection.execute(text("SELECT version FROM schema_migrations"))
        known = {row[0] for row in result}

        for migration in MIGRATIONS:
            if migration.version in known:
                continue
            for statement in migration.statements:
                await connection.execute(text(statement))
            await connection.execute(
                text(
                    "INSERT INTO schema_migrations (version, name, applied_at)"
                    " VALUES (:version, :name, CURRENT_TIMESTAMP)"
                ),
                {"version": migration.version, "name": migration.name},
            )
            applied.append(migration.version)
            log.info(
                "applied migration",
                extra={"migration_version": migration.version, "migration": migration.name},
            )
    return applied
