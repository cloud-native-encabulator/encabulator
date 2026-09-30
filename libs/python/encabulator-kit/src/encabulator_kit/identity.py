"""Identity propagation, per design section 7.

The gateway strips inbound ``X-Encabulator-*`` headers and sets ``X-Encabulator-Subject`` and
``X-Encabulator-Roles``. Internal services trust them.
"""

from __future__ import annotations

from collections.abc import Iterable, Mapping
from dataclasses import dataclass, field
from typing import Annotated, Any

from fastapi import Depends, Request

from .errors import EncabulatorError

__all__ = [
    "ANONYMOUS",
    "PRIVILEGED_ROLES",
    "ROLES",
    "ROLES_HEADER",
    "SUBJECT_HEADER",
    "CurrentIdentity",
    "Identity",
    "current_identity",
    "require_roles",
    "strip_identity_headers",
]

SUBJECT_HEADER = "X-Encabulator-Subject"
ROLES_HEADER = "X-Encabulator-Roles"

#: Every role the system knows about (design section 7).
ROLES = frozenset({"customer", "viewer", "operator", "admin"})

#: Roles that may see other customers' data (design section 8.6 visibility rule).
PRIVILEGED_ROLES = frozenset({"viewer", "operator", "admin"})


@dataclass(frozen=True, slots=True)
class Identity:
    """Who is calling, as asserted by the gateway or the console BFF."""

    subject: str | None = None
    roles: frozenset[str] = field(default_factory=frozenset)

    @property
    def authenticated(self) -> bool:
        return bool(self.subject)

    @property
    def is_privileged(self) -> bool:
        """True when this caller may read across customers."""
        return bool(self.roles & PRIVILEGED_ROLES)

    def has_role(self, *roles: str) -> bool:
        return bool(self.roles.intersection(roles))

    def to_headers(self) -> dict[str, str]:
        headers: dict[str, str] = {}
        if self.subject:
            headers[SUBJECT_HEADER] = self.subject
        if self.roles:
            headers[ROLES_HEADER] = ",".join(sorted(self.roles))
        return headers

    @classmethod
    def from_headers(cls, headers: Mapping[str, Any]) -> Identity:
        subject = _lookup(headers, SUBJECT_HEADER)
        roles = _lookup(headers, ROLES_HEADER)
        return cls(subject=subject or None, roles=parse_roles(roles))

    @classmethod
    def of(cls, subject: str | None, roles: Iterable[str] = ()) -> Identity:
        return cls(subject=subject, roles=frozenset(roles))


ANONYMOUS = Identity()


def _lookup(headers: Mapping[str, Any], name: str) -> str | None:
    value = headers.get(name)
    if value is None:
        value = headers.get(name.lower())
    if value is None:
        return None
    return str(value).strip() or None


def parse_roles(raw: str | None) -> frozenset[str]:
    if not raw:
        return frozenset()
    return frozenset(part.strip().lower() for part in raw.split(",") if part.strip())


def strip_identity_headers(headers: Mapping[str, str]) -> dict[str, str]:
    """Drop every ``X-Encabulator-*`` header; the gateway sets its own."""
    return {k: v for k, v in headers.items() if not k.lower().startswith("x-encabulator-")}


def current_identity(request: Request) -> Identity:
    """FastAPI dependency: the caller's identity from the trusted headers."""
    return Identity.from_headers(request.headers)


CurrentIdentity = Annotated[Identity, Depends(current_identity)]


def require_roles(*roles: str, allow_anonymous: bool = False):
    """FastAPI dependency factory enforcing that the caller holds one of ``roles``."""
    required = frozenset(r.lower() for r in roles)

    def dependency(identity: CurrentIdentity) -> Identity:
        if not identity.authenticated and not allow_anonymous:
            raise EncabulatorError(401, "unauthenticated", "No identity was presented")
        if required and not identity.has_role(*required):
            raise EncabulatorError(
                403,
                "forbidden",
                f"Requires one of the roles: {', '.join(sorted(required))}",
            )
        return identity

    return Depends(dependency)
