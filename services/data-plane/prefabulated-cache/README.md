# Prefabulated Cache

The Prefabulated Cache is a REST API wrapper around Redis that provides caching operations for the Cloud Native Encabulator system.

## Features

- Set/Get/Delete cache operations
- TTL support for cache entries
- Cache statistics
- Health and readiness endpoints
- Prometheus metrics exposure
- Structured JSON logging
- Graceful shutdown

## API Endpoints

### Health Endpoints
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe (checks Redis connectivity)
- `GET /metrics` - Prometheus metrics

### Cache Endpoints
- `POST /api/v1/cache` - Set a cache entry
- `GET /api/v1/cache/{key}` - Get a cache entry
- `DELETE /api/v1/cache/{key}` - Delete a cache entry
- `GET /api/v1/cache/stats` - Get cache statistics

## Configuration

The service is configured via environment variables:

- `PORT` - HTTP server port (default: 8080)
- `REDIS_ADDR` - Redis address (default: localhost:6379)
- `REDIS_PASSWORD` - Redis password (default: empty)

## Development

### Build

```bash
go build -o prefabulated-cache ./cmd/server
```

### Run

```bash
export REDIS_ADDR=localhost:6379
./prefabulated-cache
```

### Test

```bash
go test ./...
```

### Docker Build

```bash
docker build -t prefabulated-cache:latest .
```

## Example Requests

### Set Cache Entry

```bash
curl -X POST http://localhost:8080/api/v1/cache \
  -H "Content-Type: application/json" \
  -d '{
    "key": "test-key",
    "value": {"data": "test-value"},
    "ttl": 3600
  }'
```

### Get Cache Entry

```bash
curl http://localhost:8080/api/v1/cache/test-key
```

### Get Cache Stats

```bash
curl http://localhost:8080/api/v1/cache/stats
```
