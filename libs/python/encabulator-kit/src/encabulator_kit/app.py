"""FastAPI application factory implementing design section 7 for Python services.

What you get: ``/healthz``, ``/readyz`` with pluggable async checks, ``/metrics``, the
``http_server_request_duration_seconds`` histogram labelled with the route *template*, request
id propagation, the documented error shape, the malaise middleware, and a lifespan that starts
and stops the malaise poller, the flag client, and tracing.
"""

from __future__ import annotations

import asyncio
import logging
import time
from collections.abc import AsyncIterator, Awaitable, Callable, Mapping, Sequence
from contextlib import asynccontextmanager
from typing import Any

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse, Response
from starlette.exceptions import HTTPException as StarletteHTTPException
from starlette.routing import Match

from .context import reset_request_id, set_request_id
from .errors import DEFAULT_ERROR_CODES, EncabulatorError, error_body
from .flags import FlagClient
from .ids import new_ulid
from .logs import configure_logging
from .malaise import EXCLUDED_PATHS, MalaiseController, MalaiseInjected, MalaiseMiddleware
from .metrics import CONTENT_TYPE_LATEST, http_server_duration, render
from .telemetry import instrument_app, setup_tracing

__all__ = ["ReadinessRegistry", "create_app"]

log = logging.getLogger(__name__)

REQUEST_ID_HEADER = "x-request-id"
UNMATCHED_ROUTE = "unmatched"

ReadinessCheck = Callable[[], Awaitable[Any]]


class ReadinessRegistry:
    """The service's *own* hard dependencies. Never check downstream services here."""

    def __init__(self, timeout: float = 2.0) -> None:
        self._checks: dict[str, ReadinessCheck] = {}
        self.timeout = timeout

    def add(self, name: str, check: ReadinessCheck) -> None:
        self._checks[name] = check

    def remove(self, name: str) -> None:
        self._checks.pop(name, None)

    @property
    def names(self) -> tuple[str, ...]:
        return tuple(self._checks)

    async def run(self) -> tuple[bool, dict[str, str]]:
        if not self._checks:
            return True, {}
        names = list(self._checks)
        results = await asyncio.gather(
            *(self._run_one(self._checks[name]) for name in names), return_exceptions=True
        )
        report: dict[str, str] = {}
        healthy = True
        for name, result in zip(names, results, strict=True):
            if isinstance(result, BaseException):
                healthy = False
                report[name] = f"error: {result}"
            else:
                report[name] = "ok"
        return healthy, report

    async def _run_one(self, check: ReadinessCheck) -> None:
        await asyncio.wait_for(check(), timeout=self.timeout)


class ServerObservabilityMiddleware:
    """Request id in, request id out, and one histogram observation per request.

    Health, readiness, and metrics requests get a request id but are kept out of the RED
    histogram: probe and scrape traffic would otherwise dominate a quiet service's percentiles.
    """

    def __init__(self, app: Any, routes: Sequence[Any], excluded_paths: frozenset[str]) -> None:
        self.app = app
        self.routes = routes
        self.excluded_paths = excluded_paths

    async def __call__(self, scope: Any, receive: Any, send: Any) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        request_id = _inbound_request_id(scope) or new_ulid()
        token = set_request_id(request_id)
        measured = scope.get("path") not in self.excluded_paths
        started = time.perf_counter()
        status_code = 500

        async def send_wrapper(message: Mapping[str, Any]) -> None:
            nonlocal status_code
            if message["type"] == "http.response.start":
                status_code = int(message["status"])
                headers = list(message.get("headers") or [])
                headers.append((b"x-request-id", request_id.encode()))
                message = {**message, "headers": headers}
            await send(message)

        try:
            await self.app(scope, receive, send_wrapper)
        finally:
            if measured:
                http_server_duration(
                    scope.get("method", "GET"),
                    self._route_template(scope),
                    status_code,
                    time.perf_counter() - started,
                )
            reset_request_id(token)

    def _route_template(self, scope: Any) -> str:
        route = scope.get("route")
        path = getattr(route, "path", None)
        if path:
            return str(path)
        # No route in scope: either unmatched, or a middleware short-circuited before routing
        # (injected malaise does exactly that). Match by hand so the label stays meaningful.
        for candidate in self.routes:
            try:
                match, _ = candidate.matches(scope)
            except Exception:  # pragma: no cover - defensive
                continue
            if match == Match.FULL:
                return str(getattr(candidate, "path", UNMATCHED_ROUTE))
        return UNMATCHED_ROUTE


def _inbound_request_id(scope: Any) -> str | None:
    for key, value in scope.get("headers") or ():
        if key == b"x-request-id":
            text = value.decode("latin-1").strip()
            # Bound it: this value ends up in logs and in an outbound header.
            return text[:128] or None
    return None


def create_app(
    service_name: str,
    *,
    version: str = "1.0.0",
    description: str | None = None,
    lifespan: Callable[[FastAPI], Any] | None = None,
    malaise: MalaiseController | None = None,
    flags: FlagClient | None = None,
    readiness_timeout: float = 2.0,
    validation_error_code: str = "invalid_request",
    log_level: str | None = None,
    configure_logs: bool = True,
    enable_tracing: bool = True,
    **fastapi_kwargs: Any,
) -> FastAPI:
    """Build the standard Encabulator FastAPI app.

    ``malaise`` and ``flags``, when given, are started and stopped by the lifespan. Register
    readiness checks on ``app.state.readiness`` (usually from inside your own lifespan, once the
    database pool exists).
    """
    from .config import env_str

    if configure_logs:
        configure_logging(service_name, log_level or env_str("LOG_LEVEL", "info") or "info")
    if enable_tracing:
        setup_tracing(service_name, version)

    readiness = ReadinessRegistry(timeout=readiness_timeout)

    @asynccontextmanager
    async def _lifespan(app: FastAPI) -> AsyncIterator[None]:
        log.info("starting", extra={"version": version})
        if malaise is not None:
            await malaise.start()
        if flags is not None:
            await flags.start()
        try:
            if lifespan is not None:
                async with _as_context(lifespan, app):
                    yield
            else:
                yield
        finally:
            app.state.shutting_down = True
            if flags is not None:
                await flags.stop()
            if malaise is not None:
                await malaise.stop()
            from .telemetry import shutdown_tracing

            shutdown_tracing()
            log.info("stopped")

    app = FastAPI(
        title=fastapi_kwargs.pop("title", service_name),
        version=version,
        description=description or "",
        lifespan=_lifespan,
        **fastapi_kwargs,
    )
    app.state.service_name = service_name
    app.state.version = version
    app.state.readiness = readiness
    app.state.shutting_down = False
    app.state.malaise = malaise
    app.state.flags = flags

    _add_platform_routes(app, service_name, version, readiness)
    _install_error_handlers(app, validation_error_code)

    # Middleware added later wraps earlier ones. Observability must be outermost so the
    # histogram sees the latency that injected malaise adds.
    if malaise is not None:
        app.add_middleware(MalaiseMiddleware, controller=malaise)
    app.add_middleware(
        ServerObservabilityMiddleware, routes=app.router.routes, excluded_paths=EXCLUDED_PATHS
    )

    if enable_tracing:
        instrument_app(app)
    return app


def _as_context(factory: Callable[[FastAPI], Any], app: FastAPI) -> Any:
    """Accept either an async context manager factory or an async generator function."""
    result = factory(app)
    if hasattr(result, "__aenter__"):
        return result
    return asynccontextmanager(lambda: result)()  # pragma: no cover - defensive


def _add_platform_routes(
    app: FastAPI, service_name: str, version: str, readiness: ReadinessRegistry
) -> None:
    @app.get("/healthz", include_in_schema=False)
    async def healthz() -> dict[str, str]:
        """Liveness: the process is running."""
        return {"status": "ok"}

    @app.get("/readyz", include_in_schema=False)
    async def readyz(request: Request) -> Response:
        """Readiness: this service's own hard dependencies are reachable."""
        if request.app.state.shutting_down:
            return JSONResponse(
                {"status": "shutting_down", "service": service_name},
                status_code=503,
            )
        healthy, checks = await readiness.run()
        body: dict[str, Any] = {
            "status": "ok" if healthy else "unready",
            "service": service_name,
            "version": version,
        }
        if checks:
            body["checks"] = checks
        return JSONResponse(body, status_code=200 if healthy else 503)

    @app.get("/metrics", include_in_schema=False)
    async def metrics() -> Response:
        return Response(content=render(), media_type=CONTENT_TYPE_LATEST)


def _install_error_handlers(app: FastAPI, validation_error_code: str) -> None:
    @app.exception_handler(EncabulatorError)
    async def _encabulator(_: Request, exc: EncabulatorError) -> Response:
        return JSONResponse(exc.body(), status_code=exc.status_code, headers=exc.headers or None)

    @app.exception_handler(MalaiseInjected)
    async def _malaise(_: Request, exc: MalaiseInjected) -> Response:
        return JSONResponse(error_body(exc.code, exc.message), status_code=exc.status_code)

    @app.exception_handler(StarletteHTTPException)
    async def _http(_: Request, exc: StarletteHTTPException) -> Response:
        code, message = _code_and_message(exc)
        return JSONResponse(
            error_body(code, message), status_code=exc.status_code, headers=exc.headers or None
        )

    @app.exception_handler(RequestValidationError)
    async def _validation(_: Request, exc: RequestValidationError) -> Response:
        return JSONResponse(
            error_body(validation_error_code, _validation_message(exc)), status_code=400
        )

    @app.exception_handler(Exception)
    async def _unhandled(request: Request, exc: Exception) -> Response:
        log.error("unhandled error on %s", request.url.path, exc_info=exc)
        return JSONResponse(
            error_body("internal_error", "The encabulator suffered an unexpected malaise"),
            status_code=500,
        )


def _code_and_message(exc: StarletteHTTPException) -> tuple[str, str]:
    detail = exc.detail
    default = DEFAULT_ERROR_CODES.get(exc.status_code, "error")
    if isinstance(detail, Mapping):
        inner = detail.get("error", detail)
        if isinstance(inner, Mapping):
            return str(inner.get("code", default)), str(inner.get("message", default))
    if detail:
        return default, str(detail)
    return default, default.replace("_", " ")


def _validation_message(exc: RequestValidationError) -> str:
    """Summarise validation failures without echoing the submitted values."""
    parts: list[str] = []
    for error in exc.errors()[:5]:
        location = ".".join(str(item) for item in error.get("loc", ()) if item != "body")
        parts.append(f"{location or 'body'}: {error.get('msg', 'invalid')}")
    return "; ".join(parts) or "invalid request"
