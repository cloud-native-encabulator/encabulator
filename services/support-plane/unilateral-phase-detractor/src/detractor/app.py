"""The detractor's HTTP surface (design section 8.9) and its application factory."""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from encabulator_kit import (
    EncabulatorError,
    FlagClient,
    MalaiseController,
    create_app,
    error_body,
)
from fastapi import FastAPI, Query, Request, Response
from fastapi.responses import JSONResponse
from prometheus_client import Counter, Gauge
from sqlalchemy import func, select

from . import store
from .config import Settings
from .db import create_engine, create_session_factory, ping
from .deps import AdminDep, AuthenticatedDep, SessionDep
from .migrations import migrate
from .models import User
from .ratelimit import RateLimiter
from .schemas import CreateUserRequest, LoginRequest, PatchUserRequest, RegisterRequest
from .security import load_signing_key

log = logging.getLogger(__name__)

LOGINS = Counter("encabulator_logins", "Login attempts.", ["result"])
REGISTRATIONS = Counter("encabulator_registrations", "Registration attempts.", ["result"])
TOKENS = Counter("encabulator_tokens_issued", "Access tokens issued.")
USERS = Gauge("encabulator_users", "Users in the store.")


def build_app(settings: Settings | None = None) -> FastAPI:
    settings = settings or Settings.from_env()
    flags = FlagClient(settings.common.girdlespring_url)
    malaise = MalaiseController(settings.common.service_name, settings.common.malaise_engine_url)

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        engine = create_engine(settings.database_url)
        app.state.engine = engine
        app.state.session_factory = create_session_factory(engine)
        app.state.readiness.add("postgres", lambda: ping(engine))

        await migrate(engine)
        async with app.state.session_factory() as session:
            created = await store.bootstrap_users(
                session,
                admin_password=settings.bootstrap_admin_password,
                operator_password=settings.bootstrap_operator_password,
            )
            await session.commit()
            await _refresh_user_gauge(session)
        if created:
            log.info("bootstrapped users", extra={"users": created})
        try:
            yield
        finally:
            await engine.dispose()

    app = create_app(
        settings.common.service_name,
        version="1.0.0",
        description=(
            "Supplies inverse reactive credentials for use in unilateral phase detractors."
        ),
        lifespan=lifespan,
        flags=flags,
        malaise=malaise,
    )
    app.state.settings = settings
    app.state.signing_key = load_signing_key(settings.jwt_private_key_pem, settings.jwt_ttl_seconds)
    app.state.register_limiter = RateLimiter(settings.register_rate_per_minute)

    @app.exception_handler(store.StoreError)
    async def _store_error(_: Request, exc: store.StoreError) -> Response:
        return JSONResponse(error_body(exc.code, exc.message), status_code=exc.status_code)

    _register_routes(app)
    return app


async def _refresh_user_gauge(session: Any) -> None:
    total = (await session.execute(select(func.count()).select_from(User))).scalar_one()
    USERS.set(int(total))


def _client_ip(request: Request) -> str:
    forwarded = request.headers.get("X-Forwarded-For", "")
    if forwarded:
        return forwarded.split(",")[0].strip()
    return request.client.host if request.client else "unknown"


def _register_routes(app: FastAPI) -> None:  # noqa: C901 - one function per route, read top down
    # ---- authentication ----------------------------------------------------------------

    @app.post("/v1/auth/login", status_code=200)
    async def login(body: LoginRequest, session: SessionDep, request: Request) -> dict[str, Any]:
        """Exchange a username and password for an RS256 access token."""
        signing_key = request.app.state.signing_key
        try:
            user = await store.authenticate(session, body.username, body.password)
        except store.StoreError as exc:
            await store.record(
                session,
                store.LOGIN_FAILURE,
                actor=body.username[:64],
                subject=body.username[:64],
                result="failure",
                detail={"code": exc.code, "ip": _client_ip(request)},
            )
            await session.commit()
            LOGINS.labels("failure").inc()
            log.info("login failed", extra={"username": body.username, "code": exc.code})
            raise

        token = signing_key.issue(user.username, user.roles)
        await store.record(
            session,
            store.LOGIN_SUCCESS,
            actor=user.username,
            subject=user.username,
            detail={"roles": user.roles, "ip": _client_ip(request)},
        )
        LOGINS.labels("success").inc()
        TOKENS.inc()
        return {
            "accessToken": token,
            "tokenType": "Bearer",
            "expiresIn": signing_key.ttl_seconds,
            "user": user.to_json(),
        }

    @app.post("/v1/auth/register", status_code=201)
    async def register(
        body: RegisterRequest, session: SessionDep, request: Request
    ) -> dict[str, Any]:
        """Self-service registration. Always creates a plain ``customer``."""
        limiter: RateLimiter = request.app.state.register_limiter
        client = _client_ip(request)
        if not limiter.allow(client):
            REGISTRATIONS.labels("rate_limited").inc()
            raise EncabulatorError(
                429,
                "rate_limited",
                "Too many registrations from this address",
                headers={"Retry-After": str(limiter.retry_after(client))},
            )
        try:
            user = await store.create_user(
                session,
                username=body.username,
                password=body.password,
                display_name=body.displayName,
                roles=["customer"],
                actor=body.username,
                action=store.USER_REGISTERED,
            )
        except store.StoreError:
            REGISTRATIONS.labels("rejected").inc()
            raise
        REGISTRATIONS.labels("created").inc()
        USERS.inc()
        log.info("customer registered", extra={"username": user.username})
        return user.to_json()

    @app.get("/.well-known/jwks.json")
    async def jwks(request: Request) -> dict[str, Any]:
        """The public key set the gateway verifies tokens against."""
        return request.app.state.signing_key.jwks()

    # ---- users -------------------------------------------------------------------------

    @app.get("/v1/users/me")
    async def me(caller: AuthenticatedDep) -> dict[str, Any]:
        """The calling user, from the identity header or from a bearer token."""
        if caller.user is None:
            raise EncabulatorError(404, "user_not_found", "No such user")
        return caller.user.to_json()

    @app.get("/v1/users")
    async def list_all(
        session: SessionDep, _: AdminDep, limit: int = Query(default=500, ge=1, le=500)
    ) -> list[dict[str, Any]]:
        return [user.to_json() for user in await store.list_users(session, limit)]

    @app.post("/v1/users", status_code=201)
    async def create(
        body: CreateUserRequest, session: SessionDep, caller: AdminDep
    ) -> dict[str, Any]:
        user = await store.create_user(
            session,
            username=body.username,
            password=body.password,
            display_name=body.displayName,
            roles=body.roles,
            actor=caller.subject or "admin",
        )
        USERS.inc()
        return user.to_json()

    @app.patch("/v1/users/{username}")
    async def patch(
        username: str, body: PatchUserRequest, session: SessionDep, caller: AdminDep
    ) -> dict[str, Any]:
        user = await store.get_user(session, username)
        if user is None:
            raise EncabulatorError(404, "user_not_found", f"No such user: {username}")
        updated = await store.update_user(
            session,
            user,
            password=body.password,
            display_name=body.displayName,
            roles=body.roles,
            disabled=body.disabled,
            actor=caller.subject or "admin",
        )
        return updated.to_json()

    @app.delete("/v1/users/{username}", status_code=204)
    async def remove(username: str, session: SessionDep, caller: AdminDep) -> Response:
        user = await store.get_user(session, username)
        if user is None:
            raise EncabulatorError(404, "user_not_found", f"No such user: {username}")
        await store.delete_user(session, user, actor=caller.subject or "admin")
        USERS.dec()
        return Response(status_code=204)

    # ---- audit -------------------------------------------------------------------------

    @app.get("/v1/audit")
    async def audit(
        session: SessionDep, _: AdminDep, limit: int = Query(default=100, ge=1, le=1000)
    ) -> list[dict[str, Any]]:
        """Login successes and failures, and every user change, newest first."""
        return [entry.to_json() for entry in await store.list_audit(session, limit)]

    # Keep the unused-name checker quiet about route functions referenced only by FastAPI.
    _ = (login, register, jwks, me, list_all, create, patch, remove, audit)


def main() -> None:
    from encabulator_kit import run

    settings = Settings.from_env()
    run(build_app(settings), port=settings.common.port)
