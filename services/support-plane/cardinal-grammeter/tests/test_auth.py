import pytest
from app.auth import verify_password, get_password_hash, authenticate_user, create_access_token
from jose import jwt
from app.config import settings


def test_password_hashing():
    password = "testpassword123"
    hashed = get_password_hash(password)
    assert verify_password(password, hashed)
    assert not verify_password("wrongpassword", hashed)


def test_authenticate_user():
    # Test with valid credentials (using mock user)
    user = authenticate_user("admin", "secret")
    assert user is not None
    assert user.username == "admin"
    
    # Test with invalid password
    user = authenticate_user("admin", "wrongpassword")
    assert user is None
    
    # Test with non-existent user
    user = authenticate_user("nonexistent", "password")
    assert user is None


def test_create_access_token():
    data = {"sub": "testuser", "roles": ["user"]}
    token = create_access_token(data)
    
    # Decode and verify
    payload = jwt.decode(token, settings.secret_key, algorithms=[settings.algorithm])
    assert payload["sub"] == "testuser"
    assert payload["roles"] == ["user"]
    assert "exp" in payload
