import pytest
from fastapi.testclient import TestClient
from app.main import app

client = TestClient(app)


def test_health_endpoint():
    response = client.get("/health")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert "timestamp" in data


def test_ready_endpoint():
    response = client.get("/ready")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"


def test_login_success():
    response = client.post(
        "/api/v1/login",
        json={"username": "admin", "password": "secret"}
    )
    assert response.status_code == 200
    data = response.json()
    assert "access_token" in data
    assert data["token_type"] == "bearer"


def test_login_failure():
    response = client.post(
        "/api/v1/login",
        json={"username": "admin", "password": "wrongpassword"}
    )
    assert response.status_code == 401


def test_verify_token():
    # First login to get token
    login_response = client.post(
        "/api/v1/login",
        json={"username": "admin", "password": "secret"}
    )
    token = login_response.json()["access_token"]
    
    # Verify token
    response = client.get(
        "/api/v1/verify",
        headers={"Authorization": f"Bearer {token}"}
    )
    assert response.status_code == 200
    data = response.json()
    assert data["valid"] is True
    assert data["username"] == "admin"


def test_get_current_user():
    # First login to get token
    login_response = client.post(
        "/api/v1/login",
        json={"username": "admin", "password": "secret"}
    )
    token = login_response.json()["access_token"]
    
    # Get current user
    response = client.get(
        "/api/v1/users/me",
        headers={"Authorization": f"Bearer {token}"}
    )
    assert response.status_code == 200
    data = response.json()
    assert data["username"] == "admin"
    assert "admin" in data["roles"]
