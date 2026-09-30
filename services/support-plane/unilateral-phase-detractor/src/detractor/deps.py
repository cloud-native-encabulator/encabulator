"""Request-scoped dependencies: the session, and who is calling.

Identity resolution, in order:

1. ``X-Encabulator-Subject`` -- set by the gateway or the console BFF, which the design says
   internal services trust.
2. Otherwise a ``Bearer`` token, verified locally against our own public key, for callers that
   talk to the detractor directly.

Either way the caller's **roles come from the user row**, not from the header or the token. The
detractor owns the user store, so it can be authoritative: a role change takes effect at once,
and a caller that forges ``X-Encabulator-Roles: admin`` gets exactly the roles it really has.
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Annotated

from encabulator_kit import EncabulatorError, Identity
from encabulator_kit.identity import ROLES_HEADER, SUBJECT_HEADER, parse_roles
from fastapi import Depends, Request
from sqlalchemy.ext.asyncio import AsyncSession

from .db import session_scope
from .models import User
from .security import SigningKey, TokenError
from .store import get_user


async def get_session(request: Request) -> AsyncIterator[AsyncSession]:
    """One session per request, committed on success and rolled back on any error."""
    async for session in session_scope(request.app.state.session_factory):
        yield session


SessionDep = Annotated[AsyncSession, Depends(get_session)]


@dataclass(frozen=True, slots=True)
class Caller:
    """The authenticated caller, with the authoritative view of their roles."""

    identity: Identity
    user: User | None = None
    via: str = "anonymous"

    @property
    def subject(self) -> str | None:
        return self.identity.subject

    @property
    def roles(self) -> frozenset[str]:
        return self.identity.roles

    def has_role(self, *roles: str) -> bool:
        return self.identity.has_role(*roles)


async def resolve_caller(request: Request, session: SessionDep) -> Caller:
    signing_key: SigningKey = request.app.state.signing_key

    subject = (request.headers.get(SUBJECT_HEADER) or "").strip().lower()
    via = "header"
    claimed_roles = parse_roles(request.headers.get(ROLES_HEADER))

    if not subject:
        token = _bearer_token(request)
        if token is None:
            return Caller(identity=Identity())
        try:
            claims = signing_key.verify(token)
        except TokenError as exc:
            raise EncabulatorError(401, "invalid_token", f"Token rejected: {exc}") from exc
        subject = claims.subject
        claimed_roles = frozenset(claims.roles)
        via = "bearer"

    user = await get_user(session, subject)
    if user is not None and user.disabled:
        raise EncabulatorError(403, "account_disabled", "This account is disabled")

    # The user row wins. A caller with no row keeps only what it was handed, and can never
    # claim a role it was not really given.
    roles = frozenset(user.roles) if user is not None else claimed_roles
    return Caller(identity=Identity(subject=subject, roles=roles), user=user, via=via)


def _bearer_token(request: Request) -> str | None:
    header = request.headers.get("Authorization", "")
    scheme, _, value = header.partition(" ")
    if scheme.lower() != "bearer" or not value.strip():
        return None
    return value.strip()


CallerDep = Annotated[Caller, Depends(resolve_caller)]


async def require_authenticated(caller: CallerDep) -> Caller:
    if not caller.identity.authenticated:
        raise EncabulatorError(401, "unauthenticated", "No credentials were presented")
    return caller


async def require_admin(caller: CallerDep) -> Caller:
    if not caller.identity.authenticated:
        raise EncabulatorError(401, "unauthenticated", "No credentials were presented")
    if not caller.has_role("admin"):
        raise EncabulatorError(403, "forbidden", "This operation requires the admin role")
    return caller


AuthenticatedDep = Annotated[Caller, Depends(require_authenticated)]
AdminDep = Annotated[Caller, Depends(require_admin)]
