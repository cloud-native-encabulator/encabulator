"""Authentication flows, user management, audit, and the platform endpoints."""

from __future__ import annotations

import jwt
from fastapi.testclient import TestClient

from detractor.config import AUDIENCE, ISSUER

from .conftest import ADMIN_PASSWORD, CUSTOMER_PASSWORD, bearer, login


def test_platform_endpoints(client: TestClient) -> None:
    assert client.get("/healthz").json() == {"status": "ok"}
    ready = client.get("/readyz")
    assert ready.status_code == 200
    assert ready.json()["checks"] == {"postgres": "ok"}
    assert "http_server_request_duration_seconds" in client.get("/metrics").text


def test_login_returns_the_documented_body(client: TestClient) -> None:
    response = client.post("/v1/auth/login", json={"username": "admin", "password": ADMIN_PASSWORD})
    assert response.status_code == 200
    body = response.json()
    assert set(body) == {"accessToken", "tokenType", "expiresIn", "user"}
    assert body["tokenType"] == "Bearer"
    assert body["expiresIn"] == 3600
    assert body["user"]["roles"] == ["admin"]
    assert "password" not in body["user"]
    assert "passwordHash" not in body["user"]


def test_issued_token_is_verifiable_against_the_published_jwks(client: TestClient) -> None:
    token = login(client, "admin", ADMIN_PASSWORD)
    jwks = client.get("/.well-known/jwks.json").json()
    key = jwt.PyJWKSet.from_dict(jwks).keys[0].key
    claims = jwt.decode(token, key, algorithms=["RS256"], audience=AUDIENCE, issuer=ISSUER)
    assert claims["sub"] == "admin"
    assert claims["roles"] == ["admin"]
    assert jwt.get_unverified_header(token)["kid"] == jwks["keys"][0]["kid"]


def test_bad_password_is_401_with_a_vague_message(client: TestClient) -> None:
    response = client.post("/v1/auth/login", json={"username": "admin", "password": "wrong-one"})
    assert response.status_code == 401
    assert response.json()["error"]["code"] == "invalid_credentials"
    assert response.json()["error"]["message"] == "Invalid username or password"


def test_unknown_user_looks_exactly_like_a_bad_password(client: TestClient) -> None:
    unknown = client.post("/v1/auth/login", json={"username": "nobody", "password": "wrong-one"})
    known = client.post("/v1/auth/login", json={"username": "admin", "password": "wrong-one"})
    assert unknown.status_code == known.status_code == 401
    assert unknown.json() == known.json()


def test_bootstrap_creates_admin_and_operator(client: TestClient) -> None:
    token = login(client, "admin", ADMIN_PASSWORD)
    usernames = {user["username"] for user in client.get("/v1/users", headers=bearer(token)).json()}
    assert usernames == {"admin", "operator"}


def test_registration_flow(client: TestClient) -> None:
    created = client.post(
        "/v1/auth/register",
        json={"username": "ada.lovelace", "password": CUSTOMER_PASSWORD, "displayName": "Ada"},
    )
    assert created.status_code == 201
    assert created.json()["roles"] == ["customer"]
    assert created.json()["displayName"] == "Ada"
    token = login(client, "ada.lovelace", CUSTOMER_PASSWORD)
    assert client.get("/v1/users/me", headers=bearer(token)).json()["username"] == "ada.lovelace"


def test_duplicate_registration_is_409(client: TestClient) -> None:
    body = {"username": "twice", "password": CUSTOMER_PASSWORD, "displayName": ""}
    assert client.post("/v1/auth/register", json=body).status_code == 201
    conflict = client.post("/v1/auth/register", json=body)
    assert conflict.status_code == 409
    assert conflict.json()["error"]["code"] == "username_taken"


def test_username_rules(client: TestClient) -> None:
    for bad in ("ab", "has space", "e" * 65, "wrong!chars"):
        response = client.post(
            "/v1/auth/register", json={"username": bad, "password": CUSTOMER_PASSWORD}
        )
        assert response.status_code == 400, bad
        assert response.json()["error"]["code"] in {"invalid_username", "invalid_request"}
    good = client.post(
        "/v1/auth/register", json={"username": "a.b-c_9", "password": CUSTOMER_PASSWORD}
    )
    assert good.status_code == 201


def test_usernames_are_normalised_to_lower_case(client: TestClient) -> None:
    created = client.post(
        "/v1/auth/register", json={"username": "  Ada.Byron ", "password": CUSTOMER_PASSWORD}
    )
    assert created.status_code == 201
    assert created.json()["username"] == "ada.byron"
    # And the same name in another case is the same account.
    again = client.post(
        "/v1/auth/register", json={"username": "ADA.BYRON", "password": CUSTOMER_PASSWORD}
    )
    assert again.status_code == 409


def test_short_password_is_rejected(client: TestClient) -> None:
    response = client.post("/v1/auth/register", json={"username": "shorty", "password": "abc"})
    assert response.status_code == 400
    assert response.json()["error"]["code"] == "invalid_password"


def test_registration_is_rate_limited_per_ip(client: TestClient) -> None:
    statuses = [
        client.post(
            "/v1/auth/register",
            json={"username": f"crowd{index}", "password": CUSTOMER_PASSWORD},
        ).status_code
        for index in range(12)
    ]
    assert statuses.count(201) == 10  # register_rate_per_minute
    assert 429 in statuses
    limited = client.post(
        "/v1/auth/register", json={"username": "onemore", "password": CUSTOMER_PASSWORD}
    )
    assert limited.status_code == 429
    assert limited.json()["error"]["code"] == "rate_limited"
    assert int(limited.headers["Retry-After"]) >= 1


def test_user_lifecycle(client: TestClient) -> None:
    token = login(client, "admin", ADMIN_PASSWORD)
    created = client.post(
        "/v1/users",
        json={
            "username": "grace",
            "password": CUSTOMER_PASSWORD,
            "displayName": "Grace",
            "roles": ["operator", "viewer"],
        },
        headers=bearer(token),
    )
    assert created.status_code == 201
    assert created.json()["roles"] == ["operator", "viewer"]

    patched = client.patch(
        "/v1/users/grace",
        json={"displayName": "Grace H", "roles": ["viewer"]},
        headers=bearer(token),
    )
    assert patched.status_code == 200
    assert patched.json()["displayName"] == "Grace H"
    assert patched.json()["roles"] == ["viewer"]

    assert client.delete("/v1/users/grace", headers=bearer(token)).status_code == 204
    remaining = {user["username"] for user in client.get("/v1/users", headers=bearer(token)).json()}
    assert "grace" not in remaining
    assert client.delete("/v1/users/grace", headers=bearer(token)).status_code == 404


def test_password_rotation_takes_effect(client: TestClient) -> None:
    token = login(client, "admin", ADMIN_PASSWORD)
    client.post(
        "/v1/users",
        json={"username": "rotate", "password": CUSTOMER_PASSWORD},
        headers=bearer(token),
    )
    assert (
        client.patch(
            "/v1/users/rotate", json={"password": "a-brand-new-winding"}, headers=bearer(token)
        ).status_code
        == 200
    )
    assert (
        client.post(
            "/v1/auth/login", json={"username": "rotate", "password": CUSTOMER_PASSWORD}
        ).status_code
        == 401
    )
    assert (
        client.post(
            "/v1/auth/login", json={"username": "rotate", "password": "a-brand-new-winding"}
        ).status_code
        == 200
    )


def test_unknown_roles_are_rejected(client: TestClient) -> None:
    token = login(client, "admin", ADMIN_PASSWORD)
    response = client.post(
        "/v1/users",
        json={"username": "weird", "password": CUSTOMER_PASSWORD, "roles": ["superuser"]},
        headers=bearer(token),
    )
    assert response.status_code == 400
    assert response.json()["error"]["code"] == "invalid_roles"


def test_patching_an_unknown_user_is_404(client: TestClient) -> None:
    token = login(client, "admin", ADMIN_PASSWORD)
    response = client.patch("/v1/users/ghost", json={"disabled": True}, headers=bearer(token))
    assert response.status_code == 404
    assert response.json()["error"]["code"] == "user_not_found"


def test_audit_records_logins_and_changes(client: TestClient) -> None:
    client.post("/v1/auth/login", json={"username": "admin", "password": "not-the-password"})
    token = login(client, "admin", ADMIN_PASSWORD)
    client.post(
        "/v1/users",
        json={"username": "audited", "password": CUSTOMER_PASSWORD},
        headers=bearer(token),
    )
    entries = client.get("/v1/audit?limit=100", headers=bearer(token)).json()
    actions = [entry["action"] for entry in entries]
    assert "login.failure" in actions
    assert "login.success" in actions
    assert "user.created" in actions
    assert "user.bootstrapped" in actions
    # Newest first, and nothing sensitive is recorded.
    assert entries == sorted(entries, key=lambda entry: entry["id"], reverse=True)
    serialized = str(entries)
    assert ADMIN_PASSWORD not in serialized
    assert CUSTOMER_PASSWORD not in serialized


def test_audit_limit_is_honoured(client: TestClient) -> None:
    token = login(client, "admin", ADMIN_PASSWORD)
    assert len(client.get("/v1/audit?limit=1", headers=bearer(token)).json()) == 1
    assert client.get("/v1/audit?limit=0", headers=bearer(token)).status_code == 400


def test_login_metrics_are_counted(client: TestClient) -> None:
    client.post("/v1/auth/login", json={"username": "admin", "password": ADMIN_PASSWORD})
    client.post("/v1/auth/login", json={"username": "admin", "password": "nope"})
    metrics = client.get("/metrics").text
    assert 'encabulator_logins_total{result="success"}' in metrics
    assert 'encabulator_logins_total{result="failure"}' in metrics
