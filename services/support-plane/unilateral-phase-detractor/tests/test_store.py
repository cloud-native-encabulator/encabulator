"""Store-level rules that do not need the HTTP layer."""

from __future__ import annotations

import pytest
from sqlalchemy.ext.asyncio import async_sessionmaker

from detractor.db import async_database_url, create_engine, create_session_factory
from detractor.migrations import MIGRATIONS, migrate
from detractor.models import decode_roles, encode_roles
from detractor.store import (
    StoreError,
    authenticate,
    bootstrap_users,
    count_admins,
    create_user,
    get_user,
    list_audit,
    update_user,
    validate_password,
    validate_roles,
    validate_username,
)


@pytest.fixture
async def engine(tmp_path):
    instance = create_engine(f"sqlite:///{tmp_path}/store.db")
    await migrate(instance)
    try:
        yield instance
    finally:
        await instance.dispose()


@pytest.fixture
def sessions(engine) -> async_sessionmaker:
    return create_session_factory(engine)


def test_username_validation() -> None:
    assert validate_username(" ADA ") == "ada"
    for bad in ("ab", "Ada Lovelace", "a" * 65, "no/slashes"):
        with pytest.raises(StoreError) as caught:
            validate_username(bad)
        assert caught.value.code == "invalid_username"


def test_password_validation() -> None:
    validate_password("long-enough-1")
    with pytest.raises(StoreError):
        validate_password("short")


def test_role_validation() -> None:
    assert validate_roles(["Admin", " operator "]) == ["admin", "operator"]
    assert validate_roles([]) == ["customer"]
    with pytest.raises(StoreError) as caught:
        validate_roles(["root"])
    assert caught.value.code == "invalid_roles"


def test_roles_encoding_is_sorted_and_deduplicated() -> None:
    assert encode_roles(["viewer", "admin", "viewer"]) == "admin,viewer"
    assert decode_roles("admin,viewer") == ["admin", "viewer"]
    assert decode_roles(None) == []


def test_migrations_are_ordered_and_unique() -> None:
    versions = [migration.version for migration in MIGRATIONS]
    assert versions == sorted(versions)
    assert len(set(versions)) == len(versions)


async def test_migrations_are_idempotent(engine) -> None:
    assert await migrate(engine) == []


async def test_bootstrap_is_idempotent_and_rotates(sessions) -> None:
    async with sessions() as session:
        assert await bootstrap_users(
            session, admin_password="first-password-1", operator_password=None
        ) == ["admin"]
        await session.commit()

    async with sessions() as session:
        # Same password: nothing to do.
        assert (
            await bootstrap_users(
                session, admin_password="first-password-1", operator_password=None
            )
            == []
        )
        # New password: re-keyed.
        assert await bootstrap_users(
            session, admin_password="second-password-2", operator_password=None
        ) == ["admin"]
        await session.commit()

    async with sessions() as session:
        user = await authenticate(session, "admin", "second-password-2")
        assert user.roles == ["admin"]


async def test_bootstrap_skips_users_without_a_password(sessions) -> None:
    async with sessions() as session:
        assert await bootstrap_users(session, admin_password=None, operator_password=None) == []
        await session.commit()
        assert await get_user(session, "admin") is None


async def test_authenticate_rejects_disabled_users(sessions) -> None:
    async with sessions() as session:
        user = await create_user(
            session, username="ada", password="a-fine-password", roles=["customer"]
        )
        await update_user(session, user, disabled=True, actor="admin")
        await session.commit()
        with pytest.raises(StoreError) as caught:
            await authenticate(session, "ada", "a-fine-password")
        assert caught.value.code == "account_disabled"


async def test_count_admins_ignores_disabled_and_substrings(sessions) -> None:
    async with sessions() as session:
        await create_user(session, username="boss", password="a-fine-password", roles=["admin"])
        # "sysadmin" contains "admin" as a substring but is not an admin role.
        await create_user(
            session, username="sysadmin", password="a-fine-password", roles=["customer"]
        )
        disabled = await create_user(
            session, username="retired-boss", password="a-fine-password", roles=["admin"]
        )
        await update_user(session, disabled, disabled=True, actor="admin")
        await session.commit()
        assert await count_admins(session) == 1


async def test_every_change_is_audited(sessions) -> None:
    async with sessions() as session:
        user = await create_user(session, username="ada", password="a-fine-password")
        await update_user(session, user, display_name="Ada L", actor="admin")
        await session.commit()
        actions = [entry.action for entry in await list_audit(session)]
        assert actions.count("user.updated") == 1
        assert actions.count("user.created") == 1


def test_database_url_translation() -> None:
    url, connect_args = async_database_url(
        "postgres://detractor:pw@postgres:5432/detractor?sslmode=disable"
    )
    assert url.drivername == "postgresql+asyncpg"
    assert url.database == "detractor"
    assert "sslmode" not in url.query
    assert connect_args == {"ssl": False}

    url, connect_args = async_database_url("postgresql://u@h:5432/d?sslmode=require")
    assert connect_args == {"ssl": True}

    url, connect_args = async_database_url("sqlite:////tmp/x.db")
    assert url.drivername == "sqlite+aiosqlite"
    assert connect_args == {}
