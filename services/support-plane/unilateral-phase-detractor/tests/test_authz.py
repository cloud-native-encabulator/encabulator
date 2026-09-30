"""The authorization matrix.

Every protected route, crossed with every way a caller can present itself. The detractor is the
one service in the works that must never get this wrong, so the matrix is explicit rather than
clever.
"""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest
from fastapi.testclient import TestClient

from detractor.security import SigningKey

from .conftest import ADMIN_PASSWORD, CUSTOMER_PASSWORD, OPERATOR_PASSWORD, as_user, bearer, login

# ---- fixtures ------------------------------------------------------------------------------


@pytest.fixture
def populated(client: TestClient) -> TestClient:
    """admin and operator come from bootstrap; add a customer, a viewer, and a victim."""
    token = login(client, "admin", ADMIN_PASSWORD)
    for username, roles in (("ada", ["customer"]), ("vera", ["viewer"]), ("victim", ["customer"])):
        response = client.post(
            "/v1/users",
            json={
                "username": username,
                "password": CUSTOMER_PASSWORD,
                "displayName": username.title(),
                "roles": roles,
            },
            headers=bearer(token),
        )
        assert response.status_code == 201, response.text
    return client


ADMIN_ONLY_ROUTES = [
    ("GET", "/v1/users", None),
    ("POST", "/v1/users", {"username": "newbie", "password": CUSTOMER_PASSWORD}),
    ("PATCH", "/v1/users/victim", {"displayName": "Renamed"}),
    ("DELETE", "/v1/users/victim", None),
    ("GET", "/v1/audit", None),
]

CALLERS = {
    "anonymous": (None, 401),
    "customer": (as_user("ada", "customer"), 403),
    "viewer": (as_user("vera", "viewer"), 403),
    "operator": (as_user("operator", "operator"), 403),
    "admin": (as_user("admin", "admin"), 200),
}


@pytest.mark.parametrize(("method", "path", "body"), ADMIN_ONLY_ROUTES, ids=lambda v: str(v))
@pytest.mark.parametrize("caller", list(CALLERS), ids=list(CALLERS))
def test_admin_only_routes(
    populated: TestClient, method: str, path: str, body: dict | None, caller: str
) -> None:
    headers, expected = CALLERS[caller]
    response = populated.request(method, path, json=body, headers=headers or {})
    if expected == 200:
        assert response.status_code in {200, 201, 204}, response.text
    else:
        assert response.status_code == expected, response.text
        assert response.json()["error"]["code"] in {"unauthenticated", "forbidden"}


PUBLIC_ROUTES = [
    ("POST", "/v1/auth/login", {"username": "nobody", "password": "x" * 12}, {401}),
    (
        "POST",
        "/v1/auth/register",
        {"username": "walk-in", "password": CUSTOMER_PASSWORD, "displayName": "Walk In"},
        {201},
    ),
    ("GET", "/.well-known/jwks.json", None, {200}),
]


@pytest.mark.parametrize(("method", "path", "body", "expected"), PUBLIC_ROUTES)
def test_public_routes_need_no_identity(
    client: TestClient, method: str, path: str, body: dict | None, expected: set[int]
) -> None:
    response = client.request(method, path, json=body)
    assert response.status_code in expected, response.text


# ---- /v1/users/me --------------------------------------------------------------------------


def test_me_requires_an_identity(client: TestClient) -> None:
    response = client.get("/v1/users/me")
    assert response.status_code == 401
    assert response.json()["error"]["code"] == "unauthenticated"


@pytest.mark.parametrize(
    ("username", "roles"),
    [("admin", "admin"), ("operator", "operator"), ("ada", "customer"), ("vera", "viewer")],
)
def test_me_works_for_every_role_via_headers(
    populated: TestClient, username: str, roles: str
) -> None:
    response = populated.get("/v1/users/me", headers=as_user(username, roles))
    assert response.status_code == 200
    assert response.json()["username"] == username
    assert isinstance(response.json()["roles"], list)


def test_me_works_via_a_bearer_token(populated: TestClient) -> None:
    token = login(populated, "ada", CUSTOMER_PASSWORD)
    response = populated.get("/v1/users/me", headers=bearer(token))
    assert response.status_code == 200
    assert response.json()["username"] == "ada"


# ---- forgery and escalation ----------------------------------------------------------------


def test_forged_roles_header_does_not_grant_admin(populated: TestClient) -> None:
    """The subject header is trusted; the roles header is not. Roles come from the user row."""
    response = populated.get("/v1/users", headers=as_user("ada", "admin,operator"))
    assert response.status_code == 403
    assert response.json()["error"]["code"] == "forbidden"


def test_forged_roles_header_cannot_reach_the_audit_log(populated: TestClient) -> None:
    assert populated.get("/v1/audit", headers=as_user("vera", "admin")).status_code == 403


def test_unknown_subject_header_gets_only_the_roles_it_was_handed(populated: TestClient) -> None:
    """A subject with no user row (mid-registration, say) cannot see other users."""
    response = populated.get("/v1/users/me", headers=as_user("ghost", "admin"))
    assert response.status_code == 404
    assert response.json()["error"]["code"] == "user_not_found"


def test_token_signed_by_another_key_is_rejected(populated: TestClient) -> None:
    impostor = SigningKey.generate()
    response = populated.get("/v1/users", headers=bearer(impostor.issue("admin", ["admin"])))
    assert response.status_code == 401
    assert response.json()["error"]["code"] == "invalid_token"


def test_expired_token_is_rejected(populated: TestClient, signing_key_pem: str) -> None:
    key = SigningKey.from_pem(signing_key_pem)
    stale = key.issue("admin", ["admin"], now=datetime.now(UTC) - timedelta(hours=2))
    response = populated.get("/v1/users", headers=bearer(stale))
    assert response.status_code == 401
    assert response.json()["error"]["code"] == "invalid_token"


def test_garbage_bearer_token_is_rejected(populated: TestClient) -> None:
    response = populated.get("/v1/users", headers=bearer("not.a.token"))
    assert response.status_code == 401


def test_token_with_stale_roles_does_not_keep_privileges(populated: TestClient) -> None:
    """A demoted admin's live token stops working: roles are re-read from the store."""
    admin_token = login(populated, "admin", ADMIN_PASSWORD)
    created = populated.post(
        "/v1/users",
        json={"username": "second", "password": CUSTOMER_PASSWORD, "roles": ["admin"]},
        headers=bearer(admin_token),
    )
    assert created.status_code == 201
    second_token = login(populated, "second", CUSTOMER_PASSWORD)
    assert populated.get("/v1/users", headers=bearer(second_token)).status_code == 200

    demote = populated.patch(
        "/v1/users/second", json={"roles": ["customer"]}, headers=bearer(admin_token)
    )
    assert demote.status_code == 200
    assert populated.get("/v1/users", headers=bearer(second_token)).status_code == 403


def test_a_customer_cannot_escalate_their_own_roles(populated: TestClient) -> None:
    token = login(populated, "ada", CUSTOMER_PASSWORD)
    response = populated.patch("/v1/users/ada", json={"roles": ["admin"]}, headers=bearer(token))
    assert response.status_code == 403
    assert populated.get("/v1/users/me", headers=bearer(token)).json()["roles"] == ["customer"]


def test_registration_always_creates_a_plain_customer(client: TestClient) -> None:
    response = client.post(
        "/v1/auth/register",
        json={
            "username": "sneaky",
            "password": CUSTOMER_PASSWORD,
            "displayName": "Sneaky",
            "roles": ["admin"],
        },
    )
    # Extra fields are rejected outright, so there is no path to a self-granted role.
    assert response.status_code == 400
    assert response.json()["error"]["code"] == "invalid_request"


# ---- disabled accounts ---------------------------------------------------------------------


def test_disabled_account_cannot_log_in(populated: TestClient) -> None:
    admin_token = login(populated, "admin", ADMIN_PASSWORD)
    populated.patch("/v1/users/ada", json={"disabled": True}, headers=bearer(admin_token))
    response = populated.post(
        "/v1/auth/login", json={"username": "ada", "password": CUSTOMER_PASSWORD}
    )
    assert response.status_code == 403
    assert response.json()["error"]["code"] == "account_disabled"


def test_disabled_accounts_existing_token_stops_working(populated: TestClient) -> None:
    token = login(populated, "ada", CUSTOMER_PASSWORD)
    admin_token = login(populated, "admin", ADMIN_PASSWORD)
    populated.patch("/v1/users/ada", json={"disabled": True}, headers=bearer(admin_token))
    response = populated.get("/v1/users/me", headers=bearer(token))
    assert response.status_code == 403
    assert response.json()["error"]["code"] == "account_disabled"


def test_disabled_account_via_headers_is_also_refused(populated: TestClient) -> None:
    admin_token = login(populated, "admin", ADMIN_PASSWORD)
    populated.patch("/v1/users/ada", json={"disabled": True}, headers=bearer(admin_token))
    assert populated.get("/v1/users/me", headers=as_user("ada", "customer")).status_code == 403


# ---- the works keeps an administrator ------------------------------------------------------


def test_the_last_admin_cannot_be_demoted(populated: TestClient) -> None:
    token = login(populated, "admin", ADMIN_PASSWORD)
    response = populated.patch(
        "/v1/users/admin", json={"roles": ["operator"]}, headers=bearer(token)
    )
    assert response.status_code == 409
    assert response.json()["error"]["code"] == "last_admin"


def test_the_last_admin_cannot_be_disabled(populated: TestClient) -> None:
    token = login(populated, "admin", ADMIN_PASSWORD)
    response = populated.patch("/v1/users/admin", json={"disabled": True}, headers=bearer(token))
    assert response.status_code == 409


def test_an_admin_cannot_delete_themselves(populated: TestClient) -> None:
    token = login(populated, "admin", ADMIN_PASSWORD)
    response = populated.delete("/v1/users/admin", headers=bearer(token))
    assert response.status_code == 409
    assert response.json()["error"]["code"] == "cannot_delete_self"


def test_a_second_admin_may_be_removed(populated: TestClient) -> None:
    token = login(populated, "admin", ADMIN_PASSWORD)
    populated.post(
        "/v1/users",
        json={"username": "deputy", "password": OPERATOR_PASSWORD, "roles": ["admin"]},
        headers=bearer(token),
    )
    assert populated.delete("/v1/users/deputy", headers=bearer(token)).status_code == 204
