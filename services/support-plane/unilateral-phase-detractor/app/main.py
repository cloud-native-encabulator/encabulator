from datetime import datetime, timedelta
from fastapi import FastAPI, Depends, HTTPException, status, Request
from fastapi.security import OAuth2PasswordRequestForm
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from fastapi.responses import Response
import time

from .models import Token, User, LoginRequest, HealthCheck
from .auth import authenticate_user, create_access_token, get_current_active_user, fake_users_db, log_activity
from .config import settings
from .users import router as users_router

app = FastAPI(title=settings.app_name, version="0.1.0")

# Include user management router
app.include_router(users_router)

# Prometheus metrics
login_attempts = Counter("login_attempts_total", "Total login attempts", ["status"])
token_validations = Counter("token_validations_total", "Total token validations", ["status"])
request_duration = Histogram("http_request_duration_seconds", "HTTP request duration")


@app.middleware("http")
async def add_process_time_header(request, call_next):
    start_time = time.time()
    response = await call_next(request)
    process_time = time.time() - start_time
    request_duration.observe(process_time)
    response.headers["X-Process-Time"] = str(process_time)
    return response


@app.get("/health", response_model=HealthCheck)
async def health():
    return HealthCheck(status="healthy", timestamp=datetime.utcnow())


@app.get("/ready", response_model=HealthCheck)
async def ready():
    return HealthCheck(status="healthy", timestamp=datetime.utcnow())


@app.get("/metrics")
async def metrics():
    return Response(content=generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.post("/token", response_model=Token)
async def login(request: Request, form_data: OAuth2PasswordRequestForm = Depends()):
    user = authenticate_user(form_data.username, form_data.password)
    if not user:
        login_attempts.labels(status="failed").inc()
        log_activity(
            username=form_data.username,
            action="login",
            ip_address=request.client.host if request.client else None,
            success=False
        )
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Incorrect username or password",
            headers={"WWW-Authenticate": "Bearer"},
        )
    
    # Update last login
    if form_data.username in fake_users_db:
        fake_users_db[form_data.username]["last_login"] = datetime.utcnow()
    
    login_attempts.labels(status="success").inc()
    log_activity(
        username=form_data.username,
        action="login",
        ip_address=request.client.host if request.client else None,
        success=True
    )
    
    access_token_expires = timedelta(minutes=settings.access_token_expire_minutes)
    access_token = create_access_token(
        data={"sub": user.username, "roles": user.roles}, expires_delta=access_token_expires
    )
    return Token(access_token=access_token, token_type="bearer")


@app.post("/api/v1/login", response_model=Token)
async def login_json(request: Request, login_req: LoginRequest):
    user = authenticate_user(login_req.username, login_req.password)
    if not user:
        login_attempts.labels(status="failed").inc()
        log_activity(
            username=login_req.username,
            action="login",
            ip_address=request.client.host if request.client else None,
            success=False
        )
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Incorrect username or password",
        )
    
    # Update last login
    if login_req.username in fake_users_db:
        fake_users_db[login_req.username]["last_login"] = datetime.utcnow()
    
    login_attempts.labels(status="success").inc()
    log_activity(
        username=login_req.username,
        action="login",
        ip_address=request.client.host if request.client else None,
        success=True
    )
    
    access_token_expires = timedelta(minutes=settings.access_token_expire_minutes)
    access_token = create_access_token(
        data={"sub": user.username, "roles": user.roles}, expires_delta=access_token_expires
    )
    return Token(access_token=access_token, token_type="bearer")


@app.get("/api/v1/users/me", response_model=User)
async def read_users_me(current_user: User = Depends(get_current_active_user)):
    token_validations.labels(status="success").inc()
    return current_user


@app.get("/api/v1/verify")
async def verify_token(current_user: User = Depends(get_current_active_user)):
    token_validations.labels(status="success").inc()
    return {"valid": True, "username": current_user.username, "roles": current_user.roles}
