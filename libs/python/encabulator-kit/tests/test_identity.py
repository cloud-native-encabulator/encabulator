from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

from encabulator_kit.app import create_app
from encabulator_kit.identity import (
    ROLES_HEADER,
    SUBJECT_HEADER,
    CurrentIdentity,
    Identity,
    require_roles,
    strip_identity_headers,
)


def test_from_headers_parses_roles() -> None:
    identity = Identity.from_headers({SUBJECT_HEADER: "ada", ROLES_HEADER: "customer, Admin"})
    assert identity.subject == "ada"
    assert identity.roles == frozenset({"customer", "admin"})
    assert identity.is_privileged


def test_anonymous_has_no_privileges() -> None:
    identity = Identity.from_headers({})
    assert not identity.authenticated
    assert not identity.is_privileged


def test_customer_is_not_privileged() -> None:
    assert not Identity.of("ada", ["customer"]).is_privileged


def test_round_trip_headers() -> None:
    identity = Identity.of("ada", ["customer", "viewer"])
    assert Identity.from_headers(identity.to_headers()) == identity


def test_strip_identity_headers() -> None:
    cleaned = strip_identity_headers(
        {"X-Encabulator-Subject": "spoofed", "x-encabulator-roles": "admin", "Accept": "*/*"}
    )
    assert cleaned == {"Accept": "*/*"}


@pytest.fixture
def client() -> TestClient:
    app = create_app("kit-identity-test", configure_logs=False)

    @app.get("/me")
    async def me(identity: CurrentIdentity) -> dict[str, object]:
        return {"subject": identity.subject, "roles": sorted(identity.roles)}

    @app.get("/admin", dependencies=[require_roles("admin")])
    async def admin() -> dict[str, bool]:
        return {"ok": True}

    return TestClient(app, raise_server_exceptions=False)


def test_dependency_reads_headers(client: TestClient) -> None:
    response = client.get("/me", headers={SUBJECT_HEADER: "ada", ROLES_HEADER: "customer"})
    assert response.json() == {"subject": "ada", "roles": ["customer"]}


def test_require_roles_rejects_anonymous(client: TestClient) -> None:
    assert client.get("/admin").status_code == 401


def test_require_roles_rejects_wrong_role(client: TestClient) -> None:
    response = client.get("/admin", headers={SUBJECT_HEADER: "ada", ROLES_HEADER: "operator"})
    assert response.status_code == 403


def test_require_roles_accepts_right_role(client: TestClient) -> None:
    response = client.get("/admin", headers={SUBJECT_HEADER: "root", ROLES_HEADER: "admin"})
    assert response.status_code == 200
