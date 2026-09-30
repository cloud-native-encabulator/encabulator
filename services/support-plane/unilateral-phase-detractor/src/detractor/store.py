"""The user and audit store. Everything that touches the database lives here."""

from __future__ import annotations

import json
import logging
import re
from collections.abc import Iterable, Sequence
from typing import Any

from encabulator_kit import new_id
from sqlalchemy import delete, func, select
from sqlalchemy.ext.asyncio import AsyncSession

from .models import AuditEntry, User, encode_roles, utcnow
from .security import hash_password, needs_rehash, verify_password

log = logging.getLogger(__name__)

USERNAME_PATTERN = re.compile(r"^[a-z0-9._-]{3,64}$")
MIN_PASSWORD_LENGTH = 8
MAX_PASSWORD_LENGTH = 1024
VALID_ROLES = frozenset({"customer", "viewer", "operator", "admin"})

# Audit actions.
LOGIN_SUCCESS = "login.success"
LOGIN_FAILURE = "login.failure"
USER_REGISTERED = "user.registered"
USER_CREATED = "user.created"
USER_UPDATED = "user.updated"
USER_DELETED = "user.deleted"
USER_BOOTSTRAPPED = "user.bootstrapped"


class StoreError(Exception):
    """A domain rule was violated. Carries the API error code."""

    def __init__(self, code: str, message: str, status_code: int = 400) -> None:
        self.code = code
        self.message = message
        self.status_code = status_code
        super().__init__(f"{code}: {message}")


def validate_username(username: str) -> str:
    candidate = username.strip().lower()
    if not USERNAME_PATTERN.match(candidate):
        raise StoreError(
            "invalid_username",
            "Usernames are 3 to 64 characters of [a-z0-9._-]",
        )
    return candidate


def validate_password(password: str) -> str:
    if not MIN_PASSWORD_LENGTH <= len(password) <= MAX_PASSWORD_LENGTH:
        raise StoreError(
            "invalid_password",
            f"Passwords are {MIN_PASSWORD_LENGTH} to {MAX_PASSWORD_LENGTH} characters",
        )
    return password


def validate_roles(roles: Iterable[str]) -> list[str]:
    cleaned = sorted({role.strip().lower() for role in roles if role.strip()})
    unknown = [role for role in cleaned if role not in VALID_ROLES]
    if unknown:
        raise StoreError("invalid_roles", f"Unknown roles: {', '.join(unknown)}")
    return cleaned or ["customer"]


async def get_user(session: AsyncSession, username: str) -> User | None:
    return await session.get(User, username.strip().lower())


async def list_users(session: AsyncSession, limit: int = 500) -> Sequence[User]:
    result = await session.execute(select(User).order_by(User.username).limit(limit))
    return result.scalars().all()


async def count_admins(session: AsyncSession, *, excluding: str | None = None) -> int:
    statement = (
        select(func.count())
        .select_from(User)
        .where(User.disabled.is_(False), User.roles_csv.like("%admin%"))
    )
    if excluding:
        statement = statement.where(User.username != excluding)
    return int((await session.execute(statement)).scalar_one())


async def create_user(
    session: AsyncSession,
    *,
    username: str,
    password: str,
    display_name: str = "",
    roles: Iterable[str] = ("customer",),
    actor: str = "",
    action: str = USER_CREATED,
) -> User:
    username = validate_username(username)
    validate_password(password)
    wanted_roles = validate_roles(roles)

    if await get_user(session, username) is not None:
        raise StoreError("username_taken", f"{username} is already registered", 409)

    now = utcnow()
    user = User(
        username=username,
        display_name=(display_name or username)[:128],
        password_hash=hash_password(password),
        disabled=False,
        created_at=now,
        updated_at=now,
    )
    user.roles = wanted_roles
    session.add(user)
    await record(
        session, action, actor=actor or username, subject=username, detail={"roles": wanted_roles}
    )
    await session.flush()
    return user


async def update_user(
    session: AsyncSession,
    user: User,
    *,
    password: str | None = None,
    display_name: str | None = None,
    roles: Iterable[str] | None = None,
    disabled: bool | None = None,
    actor: str = "",
) -> User:
    changed: dict[str, Any] = {}
    if password is not None:
        validate_password(password)
        user.password_hash = hash_password(password)
        changed["password"] = "rotated"  # noqa: S105 - a marker, not a password
    if display_name is not None:
        user.display_name = display_name[:128]
        changed["displayName"] = user.display_name
    if roles is not None:
        wanted = validate_roles(roles)
        if "admin" in user.roles and "admin" not in wanted:
            await _guard_last_admin(session, user.username)
        user.roles = wanted
        changed["roles"] = wanted
    if disabled is not None:
        if disabled and "admin" in user.roles:
            await _guard_last_admin(session, user.username)
        user.disabled = disabled
        changed["disabled"] = disabled

    user.updated_at = utcnow()
    await record(session, USER_UPDATED, actor=actor, subject=user.username, detail=changed)
    await session.flush()
    return user


async def delete_user(session: AsyncSession, user: User, *, actor: str) -> None:
    if user.username == actor:
        raise StoreError("cannot_delete_self", "An administrator cannot delete themselves", 409)
    if "admin" in user.roles:
        await _guard_last_admin(session, user.username)
    await session.execute(delete(User).where(User.username == user.username))
    await record(session, USER_DELETED, actor=actor, subject=user.username, detail={})
    await session.flush()


async def _guard_last_admin(session: AsyncSession, username: str) -> None:
    if await count_admins(session, excluding=username) == 0:
        raise StoreError(
            "last_admin",
            "The works must keep at least one enabled administrator",
            409,
        )


async def authenticate(session: AsyncSession, username: str, password: str) -> User:
    """Verify credentials. Raises :class:`StoreError` with a deliberately vague message."""
    invalid = StoreError("invalid_credentials", "Invalid username or password", 401)
    user = await get_user(session, username)
    if user is None:
        # Spend comparable time so a missing user is not obviously faster than a wrong password.
        verify_password(_DUMMY_HASH, password)
        raise invalid
    if not verify_password(user.password_hash, password):
        raise invalid
    if user.disabled:
        raise StoreError("account_disabled", "This account is disabled", 403)
    if needs_rehash(user.password_hash):
        user.password_hash = hash_password(password)
        user.updated_at = utcnow()
    return user


async def record(
    session: AsyncSession,
    action: str,
    *,
    actor: str = "",
    subject: str = "",
    result: str = "success",
    detail: dict[str, Any] | None = None,
) -> AuditEntry:
    entry = AuditEntry(
        id=new_id("aud"),
        time=utcnow(),
        action=action,
        actor=actor[:64],
        subject=subject[:64],
        result=result,
        detail_json=json.dumps(detail or {}, default=str),
    )
    session.add(entry)
    return entry


async def list_audit(session: AsyncSession, limit: int = 100) -> Sequence[AuditEntry]:
    result = await session.execute(
        select(AuditEntry).order_by(AuditEntry.id.desc()).limit(max(1, min(limit, 1000)))
    )
    return result.scalars().all()


async def bootstrap_users(
    session: AsyncSession, *, admin_password: str | None, operator_password: str | None
) -> list[str]:
    """Create or re-key the ``admin`` and ``operator`` users from the environment.

    No default password exists in code: an unset variable means the user is not created.
    """
    touched: list[str] = []
    for username, password, roles in (
        ("admin", admin_password, ["admin"]),
        ("operator", operator_password, ["operator"]),
    ):
        if not password:
            log.warning(
                "bootstrap password not set; %s will not be created",
                username,
                extra={"bootstrap_user": username},
            )
            continue
        existing = await get_user(session, username)
        if existing is None:
            await create_user(
                session,
                username=username,
                password=password,
                display_name=username.capitalize(),
                roles=roles,
                actor="bootstrap",
                action=USER_BOOTSTRAPPED,
            )
            touched.append(username)
        elif not verify_password(existing.password_hash, password):
            existing.password_hash = hash_password(password)
            existing.roles = sorted(set(existing.roles) | set(roles))
            existing.disabled = False
            existing.updated_at = utcnow()
            await record(
                session,
                USER_BOOTSTRAPPED,
                actor="bootstrap",
                subject=username,
                detail={"password": "rotated"},
            )
            touched.append(username)
    return touched


# A valid Argon2id hash of a random value, used only to equalise timing on unknown usernames.
_DUMMY_HASH = hash_password("nofer trunnion, for the operation of which")

# Roles helper re-exported for the API layer.
__all__ = [
    "LOGIN_FAILURE",
    "LOGIN_SUCCESS",
    "MAX_PASSWORD_LENGTH",
    "MIN_PASSWORD_LENGTH",
    "USERNAME_PATTERN",
    "USER_BOOTSTRAPPED",
    "USER_CREATED",
    "USER_DELETED",
    "USER_REGISTERED",
    "USER_UPDATED",
    "VALID_ROLES",
    "StoreError",
    "authenticate",
    "bootstrap_users",
    "count_admins",
    "create_user",
    "delete_user",
    "encode_roles",
    "get_user",
    "list_audit",
    "list_users",
    "record",
    "update_user",
    "validate_password",
    "validate_roles",
    "validate_username",
]
