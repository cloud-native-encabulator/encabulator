"""User management endpoints"""
from datetime import datetime
from typing import List
from fastapi import APIRouter, Depends, HTTPException, status, Request
from .models import User, UserCreate, UserUpdate, UserResponse, ActivityLog
from .auth import (
    get_current_active_user,
    require_role,
    get_password_hash,
    fake_users_db,
    activity_logs,
    log_activity
)

router = APIRouter(prefix="/api/v1/users", tags=["users"])


@router.get("/me", response_model=UserResponse)
async def get_current_user_info(
    request: Request,
    current_user: User = Depends(get_current_active_user)
):
    """Get current user info"""
    log_activity(
        username=current_user.username,
        action="get_current_user",
        ip_address=request.client.host if request.client else None
    )
    
    # Get full user data from database
    if current_user.username not in fake_users_db:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    return UserResponse(**fake_users_db[current_user.username])


@router.get("", response_model=List[UserResponse])
async def list_users(
    request: Request,
    current_user: User = Depends(require_role("admin"))
):
    """List all users (admin only)"""
    log_activity(
        username=current_user.username,
        action="list_users",
        ip_address=request.client.host if request.client else None
    )
    
    users = []
    for user_data in fake_users_db.values():
        users.append(UserResponse(**user_data))
    return users


@router.get("/{username}", response_model=UserResponse)
async def get_user(
    username: str,
    request: Request,
    current_user: User = Depends(get_current_active_user)
):
    """Get user by username (admin or self)"""
    # Users can view their own profile, admins can view anyone
    if current_user.username != username and "admin" not in current_user.roles:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Not authorized to view this user"
        )
    
    if username not in fake_users_db:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    log_activity(
        username=current_user.username,
        action="get_user",
        resource=username,
        ip_address=request.client.host if request.client else None
    )
    
    return UserResponse(**fake_users_db[username])


@router.post("", response_model=UserResponse, status_code=status.HTTP_201_CREATED)
async def create_user(
    user_create: UserCreate,
    request: Request,
    current_user: User = Depends(require_role("admin"))
):
    """Create a new user (admin only)"""
    if user_create.username in fake_users_db:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Username already exists"
        )
    
    # Validate roles
    valid_roles = ["admin", "operator", "viewer"]
    for role in user_create.roles:
        if role not in valid_roles:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=f"Invalid role: {role}. Must be one of {valid_roles}"
            )
    
    # Create user
    now = datetime.utcnow()
    user_data = {
        "username": user_create.username,
        "email": user_create.email,
        "full_name": user_create.full_name,
        "hashed_password": get_password_hash(user_create.password),
        "disabled": False,
        "roles": user_create.roles,
        "created_at": now,
        "updated_at": now,
        "last_login": None
    }
    
    fake_users_db[user_create.username] = user_data
    
    log_activity(
        username=current_user.username,
        action="create_user",
        resource=user_create.username,
        ip_address=request.client.host if request.client else None
    )
    
    return UserResponse(**user_data)


@router.put("/{username}", response_model=UserResponse)
async def update_user(
    username: str,
    user_update: UserUpdate,
    request: Request,
    current_user: User = Depends(get_current_active_user)
):
    """Update user (admin or self, but only admin can change roles)"""
    # Check authorization
    if current_user.username != username and "admin" not in current_user.roles:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Not authorized to update this user"
        )
    
    if username not in fake_users_db:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    # Only admins can change roles or disabled status
    if (user_update.roles is not None or user_update.disabled is not None) and "admin" not in current_user.roles:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only admins can change roles or disabled status"
        )
    
    # Validate roles if provided
    if user_update.roles is not None:
        valid_roles = ["admin", "operator", "viewer"]
        for role in user_update.roles:
            if role not in valid_roles:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail=f"Invalid role: {role}. Must be one of {valid_roles}"
                )
    
    # Update user
    user_data = fake_users_db[username]
    
    if user_update.email is not None:
        user_data["email"] = user_update.email
    if user_update.full_name is not None:
        user_data["full_name"] = user_update.full_name
    if user_update.password is not None:
        user_data["hashed_password"] = get_password_hash(user_update.password)
    if user_update.roles is not None:
        user_data["roles"] = user_update.roles
    if user_update.disabled is not None:
        user_data["disabled"] = user_update.disabled
    
    user_data["updated_at"] = datetime.utcnow()
    
    log_activity(
        username=current_user.username,
        action="update_user",
        resource=username,
        ip_address=request.client.host if request.client else None
    )
    
    return UserResponse(**user_data)


@router.delete("/{username}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_user(
    username: str,
    request: Request,
    current_user: User = Depends(require_role("admin"))
):
    """Delete user (admin only)"""
    if username not in fake_users_db:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    # Prevent deleting yourself
    if username == current_user.username:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot delete your own account"
        )
    
    del fake_users_db[username]
    
    log_activity(
        username=current_user.username,
        action="delete_user",
        resource=username,
        ip_address=request.client.host if request.client else None
    )
    
    return None


@router.get("/{username}/activity", response_model=List[ActivityLog])
async def get_user_activity(
    username: str,
    request: Request,
    limit: int = 100,
    current_user: User = Depends(get_current_active_user)
):
    """Get user activity logs (admin or self)"""
    # Users can view their own activity, admins can view anyone's
    if current_user.username != username and "admin" not in current_user.roles:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Not authorized to view this user's activity"
        )
    
    if username not in fake_users_db:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    # Filter logs for this user
    user_logs = [log for log in activity_logs if log["username"] == username]
    
    # Return most recent logs
    user_logs.sort(key=lambda x: x["timestamp"], reverse=True)
    
    log_activity(
        username=current_user.username,
        action="get_user_activity",
        resource=username,
        ip_address=request.client.host if request.client else None
    )
    
    return [ActivityLog(**log) for log in user_logs[:limit]]
