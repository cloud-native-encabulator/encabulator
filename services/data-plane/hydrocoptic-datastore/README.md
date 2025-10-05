# Hydrocoptic Data Store

The Hydrocoptic Data Store is a REST API wrapper around PostgreSQL that provides domain-specific operations for managing encabulation records in the Cloud Native Encabulator system.

## Features

- CRUD operations for encabulation records
- Pagination support for listing records
- Health and readiness endpoints
- Prometheus metrics exposure
- Structured JSON logging
- Graceful shutdown

## API Endpoints

### Health Endpoints
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe (checks database connectivity)
- `GET /metrics` - Prometheus metrics

### Encabulation Endpoints
- `POST /api/v1/encabulations` - Create a new encabulation
- `GET /api/v1/encabulations` - List encabulations (with pagination)
- `GET /api/v1/encabulations/{id}` - Get a specific encabulation
- `PUT /api/v1/encabulations/{id}` - Update an encabulation
- `DELETE /api/v1/encabulations/{id}` - Delete an encabulation

## Configuration

The service is configured via environment variables:

- `PORT` - HTTP server port (default: 8080)
- `DB_HOST` - PostgreSQL host (default: localhost)
- `DB_PORT` - PostgreSQL port (default: 5432)
- `DB_USER` - PostgreSQL user (default: postgres)
- `DB_PASSWORD` - PostgreSQL password (default: postgres)
- `DB_NAME` - PostgreSQL database name (default: hydrocoptic)

## Development

### Build

```bash
go build -o hydrocoptic-datastore ./cmd/server
```

### Run

```bash
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=postgres
export DB_NAME=hydrocoptic
./hydrocoptic-datastore
```

### Test

```bash
go test ./...
```

### Docker Build

```bash
docker build -t hydrocoptic-datastore:latest .
```

## Example Requests

### Create Encabulation

```bash
curl -X POST http://localhost:8080/api/v1/encabulations \
  -H "Content-Type: application/json" \
  -d '{
    "grammeters": 10.5,
    "prefabulation": true,
    "sinusoidalMode": "STANDARD"
  }'
```

### List Encabulations

```bash
curl http://localhost:8080/api/v1/encabulations?page=1&pageSize=10
```

### Get Encabulation

```bash
curl http://localhost:8080/api/v1/encabulations/{id}
```
