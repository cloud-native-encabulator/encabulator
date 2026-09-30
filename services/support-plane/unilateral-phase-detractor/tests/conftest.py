from __future__ import annotations

from collections.abc import Iterator

import pytest
from encabulator_kit import CommonSettings
from encabulator_kit.testing import container_runtime_available
from fastapi.testclient import TestClient

from detractor.app import build_app
from detractor.config import SERVICE_NAME, Settings
from detractor.security import SigningKey

ADMIN_PASSWORD = "prefamulated-amulite-1944"
OPERATOR_PASSWORD = "malleable-logarithmic-casing"
CUSTOMER_PASSWORD = "six-hydrocoptic-marzlevanes"


def pytest_configure(config: pytest.Config) -> None:
    config.addinivalue_line("markers", "integration: needs a container runtime")


def pytest_collection_modifyitems(config: pytest.Config, items: list[pytest.Item]) -> None:
    if container_runtime_available():
        return
    skip = pytest.mark.skip(reason="no container runtime (DOCKER_HOST unset); see dev-env.sh")
    for item in items:
        if "integration" in item.keywords:
            item.add_marker(skip)


@pytest.fixture(scope="session")
def signing_key_pem() -> str:
    """One RSA key for the whole session: generating them is the slowest thing here."""
    key = SigningKey.generate()
    return key._private_pem


@pytest.fixture
def settings(tmp_path, signing_key_pem: str, monkeypatch) -> Settings:
    for name in (
        "MALAISE_ENGINE_URL",
        "DIFFERENTIAL_GIRDLESPRING_URL",
        "OTEL_EXPORTER_OTLP_ENDPOINT",
    ):
        monkeypatch.delenv(name, raising=False)
    return Settings(
        common=CommonSettings.from_env(SERVICE_NAME, service_version="1.0.0"),
        database_url=f"sqlite:///{tmp_path}/detractor.db",
        jwt_private_key_pem=signing_key_pem,
        jwt_ttl_seconds=3600,
        bootstrap_admin_password=ADMIN_PASSWORD,
        bootstrap_operator_password=OPERATOR_PASSWORD,
        register_rate_per_minute=10,
    )


@pytest.fixture
def client(settings: Settings) -> Iterator[TestClient]:
    with TestClient(build_app(settings), raise_server_exceptions=False) as test_client:
        yield test_client


def login(client: TestClient, username: str, password: str) -> str:
    response = client.post("/v1/auth/login", json={"username": username, "password": password})
    assert response.status_code == 200, response.text
    return response.json()["accessToken"]


def bearer(token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}"}


def as_user(username: str, roles: str) -> dict[str, str]:
    """The headers the gateway or the console BFF would set."""
    return {"X-Encabulator-Subject": username, "X-Encabulator-Roles": roles}
