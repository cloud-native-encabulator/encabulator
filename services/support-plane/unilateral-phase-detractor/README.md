# Unilateral Phase Detractor

The Unilateral Phase Detractor is an authentication and authorization service for the Cloud Native Encabulator system, providing JWT-based authentication and role-based access control.

## Features

- JWT token-based authentication
- OAuth2 password flow support
- Role-based access control (RBAC)
- User management
- Token validation endpoint
- Health and readiness probes
- Prometheus metrics
- FastAPI framework

## Technology Stack

- **Language**: Python 3.11
- **Framework**: FastAPI
- **Authentication**: python-jose (JWT)
- **Password Hashing**: passlib with bcrypt
- **Server**: Uvicorn
- **Testing**: pytest

## API Endpoints

### Authentication Endpoints
- `POST /token` - OAuth2 password flow login
- `POST /api/v1/login` - JSON login
- `GET /api/v1/users/me` - Get current user info
- `GET /api/v1/verify` - Verify token validity

### Health Endpoints
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe
- `GET /metrics` - Prometheus metrics

## Configuration

Environment variables:

- `SECRET_KEY` - JWT secret key (change in production!)
- `ALGORITHM` - JWT algorithm (default: HS256)
- `ACCESS_TOKEN_EXPIRE_MINUTES` - Token expiration (default: 30)

## Development

### Install Dependencies

```bash
pip install -r requirements.txt
```

### Run

```bash
uvicorn app.main:app --reload --port 8000
```

### Test

```bash
pytest tests/ -v
```

### Docker Build

```bash
docker build -t unilateral-phase-detractor:latest .
```

## Example Usage

### Login

```bash
curl -X POST http://localhost:8000/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "secret"}'
```

### Response

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "bearer"
}
```

### Verify Token

```bash
curl -X GET http://localhost:8000/api/v1/verify \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Get Current User

```bash
curl -X GET http://localhost:8000/api/v1/users/me \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Default Users

For development/testing:

- **Username**: admin, **Password**: secret, **Roles**: admin, user
- **Username**: user, **Password**: secret, **Roles**: user

**⚠️ Change these in production!**

## Security Notes

- Change `SECRET_KEY` in production
- Use HTTPS in production
- Implement proper user database
- Add password complexity requirements
- Implement rate limiting
- Add account lockout after failed attempts
