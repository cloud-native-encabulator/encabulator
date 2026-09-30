"""Test helpers shared by the Python services.

Integration tests use Testcontainers against real Postgres, Valkey, and NATS, and skip cleanly
when no container runtime is available. Import this module only from tests; it needs the ``dev``
dependency group (``testcontainers``, ``pytest``), which runtime images do not install.
"""

from __future__ import annotations

import os
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path
from typing import Any

__all__ = [
    "container_runtime_available",
    "nats_container",
    "postgres_container",
    "requires_container_runtime",
    "valkey_container",
]

_SOCKETS = ("/var/run/docker.sock", "/run/docker.sock")


def container_runtime_available() -> bool:
    """True when Testcontainers has something to talk to.

    Honours ``DOCKER_HOST`` (Finch sets it via ``dev-env.sh``) and falls back to the default
    Docker sockets.
    """
    if os.environ.get("DOCKER_HOST"):
        return True
    return any(Path(socket).exists() for socket in _SOCKETS)


def requires_container_runtime() -> Any:
    """``pytest.mark.skipif`` for tests that need a container runtime."""
    import pytest

    return pytest.mark.skipif(
        not container_runtime_available(),
        reason="no container runtime: set DOCKER_HOST (see dev-env.sh) to run integration tests",
    )


@contextmanager
def postgres_container(image: str = "postgres:18", dbname: str = "encabulator") -> Iterator[str]:
    """Yield an asyncpg DSN for a throwaway Postgres 18."""
    from testcontainers.community.postgres import PostgresContainer

    with PostgresContainer(image, dbname=dbname, driver=None) as container:
        url = container.get_connection_url()
        # SQLAlchemy wants postgresql+asyncpg://; asyncpg itself wants postgresql://.
        yield url.replace("postgresql+psycopg2://", "postgresql://")


@contextmanager
def nats_container(image: str = "nats:2.12") -> Iterator[str]:
    """Yield a NATS URL for a throwaway JetStream-enabled server."""
    from testcontainers.community.nats import NatsContainer

    with NatsContainer(image, jetstream=True) as container:
        yield container.nats_uri()


@contextmanager
def valkey_container(image: str = "valkey/valkey:9") -> Iterator[str]:
    """Yield a ``redis://`` URL for a throwaway Valkey 9."""
    from testcontainers.core.container import DockerContainer
    from testcontainers.core.waiting_utils import wait_for_logs

    container = DockerContainer(image).with_exposed_ports(6379)
    with container:
        wait_for_logs(container, "Ready to accept connections", timeout=60)
        host = container.get_container_host_ip()
        port = container.get_exposed_port(6379)
        yield f"redis://{host}:{port}/0"
