"""The detractor against a real PostgreSQL 18, via Testcontainers.

Skipped cleanly when no container runtime is available.
"""

from __future__ import annotations

import pytest
from encabulator_kit import CommonSettings
from encabulator_kit.testing import postgres_container
from fastapi.testclient import TestClient

from detractor.app import build_app
from detractor.config import SERVICE_NAME, Settings
from detractor.db import create_engine, create_session_factory
from detractor.migrations import MIGRATIONS, migrate
from detractor.store import authenticate, create_user, list_audit

from .conftest import ADMIN_PASSWORD, CUSTOMER_PASSWORD, OPERATOR_PASSWORD, bearer, login

pytestmark = pytest.mark.integration


@pytest.fixture(scope="module")
def postgres_dsn():
    with postgres_container(dbname="detractor") as dsn:
        yield dsn


@pytest.fixture
def pg_settings(postgres_dsn: str, signing_key_pem: str) -> Settings:
    return Settings(
        common=CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0"),
        database_url=postgres_dsn,
        jwt_private_key_pem=signing_key_pem,
        jwt_ttl_seconds=3600,
        bootstrap_admin_password=ADMIN_PASSWORD,
        bootstrap_operator_password=OPERATOR_PASSWORD,
        register_rate_per_minute=100,
    )


async def test_migrations_apply_then_are_idempotent(postgres_dsn: str) -> None:
    engine = create_engine(postgres_dsn)
    try:
        applied = await migrate(engine)
        assert set(applied) <= {migration.version for migration in MIGRATIONS}
        assert await migrate(engine) == []
    finally:
        await engine.dispose()


async def test_users_and_audit_round_trip_on_postgres(postgres_dsn: str) -> None:
    engine = create_engine(postgres_dsn)
    await migrate(engine)
    factory = create_session_factory(engine)
    try:
        async with factory() as session:
            await create_user(
                session,
                username="pg.customer",
                password=CUSTOMER_PASSWORD,
                display_name="PG Customer",
                roles=["customer", "viewer"],
            )
            await session.commit()
        async with factory() as session:
            user = await authenticate(session, "pg.customer", CUSTOMER_PASSWORD)
            assert user.roles == ["customer", "viewer"]
            assert user.created_at.tzinfo is not None
            assert any(entry.action == "user.created" for entry in await list_audit(session))
    finally:
        await engine.dispose()


def test_full_stack_against_postgres(pg_settings: Settings) -> None:
    with TestClient(build_app(pg_settings), raise_server_exceptions=False) as client:
        ready = client.get("/readyz")
        assert ready.status_code == 200
        assert ready.json()["checks"] == {"postgres": "ok"}

        token = login(client, "admin", ADMIN_PASSWORD)
        created = client.post(
            "/v1/auth/register",
            json={"username": "pg.walkin", "password": CUSTOMER_PASSWORD, "displayName": "Walk"},
        )
        assert created.status_code in {201, 409}

        users = client.get("/v1/users", headers=bearer(token)).json()
        assert {"admin", "operator"} <= {user["username"] for user in users}

        audit = client.get("/v1/audit", headers=bearer(token)).json()
        assert any(entry["action"] == "login.success" for entry in audit)

        # A second start of the same database must not re-run migrations or duplicate users.
    with TestClient(build_app(pg_settings), raise_server_exceptions=False) as client:
        token = login(client, "admin", ADMIN_PASSWORD)
        usernames = [
            user["username"] for user in client.get("/v1/users", headers=bearer(token)).json()
        ]
        assert len(usernames) == len(set(usernames))
