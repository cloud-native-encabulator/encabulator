"""encabulator-kit: the shared Python service kit for the Cloud Native Encabulator.

It implements design section 7 (cross-cutting conventions) for FastAPI/asyncio services:
structured logging, OpenTelemetry, the standard HTTP surface, metrics, malaise injection,
feature flags, identity propagation, and NATS JetStream.

The canonical way to stand up a service::

    from encabulator_kit import CommonSettings, FlagClient, MalaiseController, create_app, run

    settings = CommonSettings.from_env("logarithmic-casing-line")
    flags = FlagClient()
    malaise = MalaiseController(settings.service_name)
    app = create_app(settings.service_name, flags=flags, malaise=malaise, lifespan=my_lifespan)

    if __name__ == "__main__":
        run(app)
"""

from __future__ import annotations

from .app import ReadinessRegistry, create_app
from .clients import ServiceClient
from .config import CommonSettings, env_bool, env_float, env_int, env_str, require_env
from .context import current_request_id
from .errors import EncabulatorError, UpstreamError, error_body
from .flags import DEFAULT_FLAGS, Flag, FlagClient, fnv1a32, in_rollout
from .identity import (
    ANONYMOUS,
    PRIVILEGED_ROLES,
    ROLES,
    CurrentIdentity,
    Identity,
    current_identity,
    require_roles,
)
from .ids import new_event_id, new_id, new_ulid
from .logs import configure_logging, get_logger
from .malaise import Directive, MalaiseController, MalaiseInjected
from .messaging import (
    STREAM_NAME,
    Envelope,
    EventBus,
    PullConsumer,
    Subjects,
    ensure_stream,
    open_bus,
)
from .metrics import render
from .serve import run, serve
from .telemetry import setup_tracing, tracer, tracing_enabled, untraced

__version__ = "1.0.0"

__all__ = [
    "ANONYMOUS",
    "DEFAULT_FLAGS",
    "PRIVILEGED_ROLES",
    "ROLES",
    "STREAM_NAME",
    "CommonSettings",
    "CurrentIdentity",
    "Directive",
    "EncabulatorError",
    "Envelope",
    "EventBus",
    "Flag",
    "FlagClient",
    "Identity",
    "MalaiseController",
    "MalaiseInjected",
    "PullConsumer",
    "ReadinessRegistry",
    "ServiceClient",
    "Subjects",
    "UpstreamError",
    "__version__",
    "configure_logging",
    "create_app",
    "current_identity",
    "current_request_id",
    "ensure_stream",
    "env_bool",
    "env_float",
    "env_int",
    "env_str",
    "error_body",
    "fnv1a32",
    "get_logger",
    "in_rollout",
    "new_event_id",
    "new_id",
    "new_ulid",
    "open_bus",
    "render",
    "require_env",
    "require_roles",
    "run",
    "serve",
    "setup_tracing",
    "tracer",
    "tracing_enabled",
    "untraced",
]
