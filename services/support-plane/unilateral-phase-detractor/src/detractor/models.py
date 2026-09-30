"""SQLAlchemy models. The schema itself is created by the SQL migrations in :mod:`migrations`.

Roles are stored as a sorted, comma-separated string and audit detail as a JSON string. Both are
deliberately portable types, so the schema behaves identically on Postgres (production) and on
SQLite (the containerless authorization tests).
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from datetime import UTC, datetime
from typing import Any

from sqlalchemy import Boolean, DateTime, String, Text
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column


class Base(DeclarativeBase):
    pass


def utcnow() -> datetime:
    return datetime.now(UTC)


def as_utc(value: datetime | None) -> datetime | None:
    """SQLite hands back naive datetimes; normalise everything to aware UTC."""
    if value is None:
        return None
    return value if value.tzinfo else value.replace(tzinfo=UTC)


def encode_roles(roles: Iterable[str]) -> str:
    return ",".join(sorted({role.strip().lower() for role in roles if role.strip()}))


def decode_roles(raw: str | None) -> list[str]:
    if not raw:
        return []
    return sorted({part.strip().lower() for part in raw.split(",") if part.strip()})


class User(Base):
    __tablename__ = "users"

    username: Mapped[str] = mapped_column(String(64), primary_key=True)
    display_name: Mapped[str] = mapped_column(String(128), default="")
    password_hash: Mapped[str] = mapped_column(Text)
    roles_csv: Mapped[str] = mapped_column("roles", Text, default="")
    disabled: Mapped[bool] = mapped_column(Boolean, default=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)

    @property
    def roles(self) -> list[str]:
        return decode_roles(self.roles_csv)

    @roles.setter
    def roles(self, value: Iterable[str]) -> None:
        self.roles_csv = encode_roles(value)

    def to_json(self) -> dict[str, Any]:
        return {
            "username": self.username,
            "displayName": self.display_name,
            "roles": self.roles,
            "disabled": self.disabled,
            "createdAt": _iso(self.created_at),
            "updatedAt": _iso(self.updated_at),
        }


class AuditEntry(Base):
    __tablename__ = "audit_log"

    # ULID ids sort by creation time, so "newest first" is ORDER BY id DESC.
    id: Mapped[str] = mapped_column(String(40), primary_key=True)
    time: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    action: Mapped[str] = mapped_column(String(64))
    actor: Mapped[str] = mapped_column(String(64), default="")
    subject: Mapped[str] = mapped_column(String(64), default="")
    result: Mapped[str] = mapped_column(String(16), default="success")
    detail_json: Mapped[str] = mapped_column("detail", Text, default="{}")

    @property
    def detail(self) -> dict[str, Any]:
        try:
            return json.loads(self.detail_json or "{}")
        except ValueError:  # pragma: no cover - defensive
            return {}

    def to_json(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "time": _iso(self.time),
            "action": self.action,
            "actor": self.actor,
            "subject": self.subject,
            "result": self.result,
            "detail": self.detail,
        }


def _iso(value: datetime | None) -> str | None:
    aware = as_utc(value)
    if aware is None:
        return None
    return aware.astimezone(UTC).isoformat(timespec="milliseconds").replace("+00:00", "Z")
