"""Tests for user management API endpoints"""
import pytest
from fastapi.testclient import TestClient
from app.main import app
from app.auth import fake_users_db, activity_logs

client = TestClient(app)


@pytest.fixture(autouse=True)
def reset_test_data():
    """Reset test data before each test"""
    # Store original users
    original_users = dict(fake_users_db)
    
    yield
    
    # Restore original users and clear any test users
    fake_users_db.clear()
    fake_users_db.update(original_users)
    
    # Clear activity logs
    activity_logs.clear()


def get_admin_token():
    """Helper to get admin token"""
    response = client.post(
        "/api/v1/login",
        json={"username": "admin", "password": "secret"}
    )
    return response.json()["access_token"]


def get_viewer_token():
    """Helper to get viewer token"""
    response = client.post(
        "/api/v1/login",
        json={"username": "viewer", "password": "secret"}
    )
    return response.json()["access_token"]


def get_operator_token():
    """Helper to get operator token"""
    response = client.post(
        "/api/v1/login",
        json={"username": "operator", "password": "secret"}
    )
    return response.json()["access_token"]


class TestListUsers:
    def test_list_users_as_admin(self):
        """Admin can list all users"""
        token = get_admin_token()
        response = client.get(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 200
        users = response.json()
        assert len(users) >= 3  # admin, operator, viewer
        assert any(u["username"] == "admin" for u in users)
    
    def test_list_users_as_non_admin(self):
        """Non-admin cannot list users"""
        token = get_viewer_token()
        response = client.get(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 403
        assert "required role" in response.json()["detail"]
    
    def test_list_users_without_auth(self):
        """Cannot list users without authentication"""
        response = client.get("/api/v1/users")
        assert response.status_code == 401


class TestGetUser:
    def test_get_user_as_admin(self):
        """Admin can get any user"""
        token = get_admin_token()
        response = client.get(
            "/api/v1/users/viewer",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 200
        user = response.json()
        assert user["username"] == "viewer"
        assert "viewer" in user["roles"]
    
    def test_get_own_user(self):
        """User can get their own profile"""
        token = get_viewer_token()
        response = client.get(
            "/api/v1/users/viewer",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 200
        user = response.json()
        assert user["username"] == "viewer"
    
    def test_get_other_user_as_non_admin(self):
        """Non-admin cannot get other users"""
        token = get_viewer_token()
        response = client.get(
            "/api/v1/users/admin",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 403
    
    def test_get_nonexistent_user(self):
        """Getting nonexistent user returns 404"""
        token = get_admin_token()
        response = client.get(
            "/api/v1/users/nonexistent",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404


class TestCreateUser:
    def test_create_user_as_admin(self):
        """Admin can create new user"""
        token = get_admin_token()
        response = client.post(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "username": "newuser",
                "email": "newuser@encabulate.net",
                "password": "password123",
                "full_name": "New User",
                "roles": ["operator"]
            }
        )
        assert response.status_code == 201
        user = response.json()
        assert user["username"] == "newuser"
        assert user["email"] == "newuser@encabulate.net"
        assert user["roles"] == ["operator"]
        assert "hashed_password" not in user  # Password should not be returned
    
    def test_create_user_duplicate_username(self):
        """Cannot create user with duplicate username"""
        token = get_admin_token()
        response = client.post(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "username": "admin",  # Already exists
                "email": "another@encabulate.net",
                "password": "password123",
                "roles": ["viewer"]
            }
        )
        assert response.status_code == 400
        assert "already exists" in response.json()["detail"]
    
    def test_create_user_invalid_role(self):
        """Cannot create user with invalid role"""
        token = get_admin_token()
        response = client.post(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "username": "newuser",
                "email": "newuser@encabulate.net",
                "password": "password123",
                "roles": ["invalid_role"]
            }
        )
        assert response.status_code == 400
        assert "Invalid role" in response.json()["detail"]
    
    def test_create_user_as_non_admin(self):
        """Non-admin cannot create users"""
        token = get_operator_token()
        response = client.post(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "username": "newuser",
                "email": "newuser@encabulate.net",
                "password": "password123",
                "roles": ["viewer"]
            }
        )
        assert response.status_code == 403
    
    def test_create_user_short_password(self):
        """Cannot create user with short password"""
        token = get_admin_token()
        response = client.post(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "username": "newuser",
                "email": "newuser@encabulate.net",
                "password": "short",  # Too short
                "roles": ["viewer"]
            }
        )
        assert response.status_code == 422  # Validation error


class TestUpdateUser:
    def test_update_own_profile(self):
        """User can update their own profile"""
        token = get_viewer_token()
        response = client.put(
            "/api/v1/users/viewer",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "full_name": "Updated Name",
                "email": "updated@encabulate.net"
            }
        )
        assert response.status_code == 200
        user = response.json()
        assert user["full_name"] == "Updated Name"
        assert user["email"] == "updated@encabulate.net"
    
    def test_update_user_as_admin(self):
        """Admin can update any user"""
        token = get_admin_token()
        response = client.put(
            "/api/v1/users/viewer",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "roles": ["operator"],
                "disabled": False
            }
        )
        assert response.status_code == 200
        user = response.json()
        assert user["roles"] == ["operator"]
    
    def test_update_roles_as_non_admin(self):
        """Non-admin cannot update roles"""
        token = get_viewer_token()
        response = client.put(
            "/api/v1/users/viewer",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "roles": ["admin"]  # Trying to escalate privileges
            }
        )
        assert response.status_code == 403
        assert "Only admins" in response.json()["detail"]
    
    def test_update_other_user_as_non_admin(self):
        """Non-admin cannot update other users"""
        token = get_viewer_token()
        response = client.put(
            "/api/v1/users/operator",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "full_name": "Hacked Name"
            }
        )
        assert response.status_code == 403
    
    def test_update_nonexistent_user(self):
        """Updating nonexistent user returns 404"""
        token = get_admin_token()
        response = client.put(
            "/api/v1/users/nonexistent",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "full_name": "Test"
            }
        )
        assert response.status_code == 404


class TestDeleteUser:
    def test_delete_user_as_admin(self):
        """Admin can delete users"""
        # First create a user
        token = get_admin_token()
        client.post(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "username": "todelete",
                "email": "delete@encabulate.net",
                "password": "password123",
                "roles": ["viewer"]
            }
        )
        
        # Then delete it
        response = client.delete(
            "/api/v1/users/todelete",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 204
        
        # Verify it's gone
        response = client.get(
            "/api/v1/users/todelete",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
    
    def test_delete_self(self):
        """Cannot delete own account"""
        token = get_admin_token()
        response = client.delete(
            "/api/v1/users/admin",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 400
        assert "Cannot delete your own account" in response.json()["detail"]
    
    def test_delete_user_as_non_admin(self):
        """Non-admin cannot delete users"""
        token = get_viewer_token()
        response = client.delete(
            "/api/v1/users/operator",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 403
    
    def test_delete_nonexistent_user(self):
        """Deleting nonexistent user returns 404"""
        token = get_admin_token()
        response = client.delete(
            "/api/v1/users/nonexistent",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404


class TestActivityLog:
    def test_get_own_activity(self):
        """User can view their own activity"""
        token = get_viewer_token()
        
        # Perform some actions
        client.get(
            "/api/v1/users/viewer",
            headers={"Authorization": f"Bearer {token}"}
        )
        
        # Get activity
        response = client.get(
            "/api/v1/users/viewer/activity",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 200
        logs = response.json()
        assert len(logs) > 0
        assert all(log["username"] == "viewer" for log in logs)
    
    def test_get_other_activity_as_admin(self):
        """Admin can view any user's activity"""
        # Viewer performs action
        viewer_token = get_viewer_token()
        client.get(
            "/api/v1/users/viewer",
            headers={"Authorization": f"Bearer {viewer_token}"}
        )
        
        # Admin views viewer's activity
        admin_token = get_admin_token()
        response = client.get(
            "/api/v1/users/viewer/activity",
            headers={"Authorization": f"Bearer {admin_token}"}
        )
        assert response.status_code == 200
        logs = response.json()
        assert any(log["username"] == "viewer" for log in logs)
    
    def test_get_other_activity_as_non_admin(self):
        """Non-admin cannot view other users' activity"""
        token = get_viewer_token()
        response = client.get(
            "/api/v1/users/admin/activity",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 403
    
    def test_activity_log_tracks_actions(self):
        """Activity log tracks various actions"""
        token = get_admin_token()
        
        # Create user
        client.post(
            "/api/v1/users",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "username": "testlog",
                "email": "testlog@encabulate.net",
                "password": "password123",
                "roles": ["viewer"]
            }
        )
        
        # Get activity
        response = client.get(
            "/api/v1/users/admin/activity",
            headers={"Authorization": f"Bearer {token}"}
        )
        logs = response.json()
        
        # Should have login and create_user actions
        actions = [log["action"] for log in logs]
        assert "login" in actions
        assert "create_user" in actions


class TestAuthentication:
    def test_last_login_updated(self):
        """Last login timestamp is updated on login"""
        # Login
        response = client.post(
            "/api/v1/login",
            json={"username": "admin", "password": "secret"}
        )
        token = response.json()["access_token"]
        
        # Check user has last_login
        response = client.get(
            "/api/v1/users/admin",
            headers={"Authorization": f"Bearer {token}"}
        )
        user = response.json()
        assert user["last_login"] is not None
    
    def test_failed_login_logged(self):
        """Failed login attempts are logged"""
        # Attempt failed login
        client.post(
            "/api/v1/login",
            json={"username": "admin", "password": "wrongpassword"}
        )
        
        # Login successfully to check logs
        token = get_admin_token()
        response = client.get(
            "/api/v1/users/admin/activity",
            headers={"Authorization": f"Bearer {token}"}
        )
        logs = response.json()
        
        # Should have a failed login
        failed_logins = [log for log in logs if log["action"] == "login" and not log["success"]]
        assert len(failed_logins) > 0
