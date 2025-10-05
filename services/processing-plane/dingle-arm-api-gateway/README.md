# Dingle Arm API Gateway

The Dingle Arm API Gateway is the external entry point for the Cloud Native Encabulator system, providing reverse proxy, rate limiting, and request routing.

## Features

- Reverse proxy to backend services
- Per-IP rate limiting
- Request/response logging
- Health and readiness endpoints
- Prometheus metrics exposure
- Structured JSON logging
- Graceful shutdown
- Configurable rate limits

## Technology Stack

- **Language**: Go 1.21
- **HTTP Framework**: Gorilla Mux
- **Rate Limiting**: golang.org/x/time/rate
- **Logging**: Zerolog
- **Metrics**: Prometheus client

## API Endpoints

### Gateway Routes
- `/*` - All requests under `/api` are proxied to backend service

### Health Endpoints
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe
- `GET /metrics` - Prometheus metrics

## Configuration

The service is configured via environment variables:

- `PORT` - HTTP server port (default: 8080)
- `BACKEND_URL` - Backend service URL (default: http://panametric-fam-processor:8080)
- `RATE_LIMIT_RPS` - Requests per second limit (default: 100.0)
- `RATE_LIMIT_BURST` - Burst limit (default: 200)

## Development

### Build

```bash
go build -o dingle-arm-api-gateway ./cmd/server
```

### Run

```bash
export BACKEND_URL=http://localhost:8080
export RATE_LIMIT_RPS=10
./dingle-arm-api-gateway
```

### Test

```bash
go test ./...
```

### Docker Build

```bash
docker build -t dingle-arm-api-gateway:latest .
```

## Example Usage

### Through Gateway

```bash
# Request goes through gateway with rate limiting
curl -X POST http://localhost:8080/api/v1/encabulations \
  -H "Content-Type: application/json" \
  -d '{
    "grammeters": 10.5,
    "prefabulation": true,
    "sinusoidalMode": "STANDARD"
  }'
```

### Rate Limit Response

When rate limit is exceeded:

```json
{
  "error": "Too Many Requests",
  "code": "RATE_LIMIT_EXCEEDED",
  "message": "Rate limit exceeded"
}
```

## Architecture

The gateway follows a middleware-based architecture:

```
cmd/server/           # Application entry point
internal/
├── middleware/       # HTTP middleware (rate limiting, logging)
└── proxy/            # Reverse proxy implementation
pkg/health/           # Health check handlers
```

## Request Flow

1. **Client Request** - External client sends request to gateway
2. **Logging Middleware** - Log request details
3. **Rate Limiting** - Check per-IP rate limits
4. **Reverse Proxy** - Forward to backend service
5. **Response** - Return backend response to client

## Rate Limiting

The gateway implements token bucket rate limiting:

- **Per-IP Limiting** - Each IP address has its own rate limit
- **Configurable Rates** - RPS and burst limits are configurable
- **Automatic Cleanup** - Limiters are created on-demand

Rate limit algorithm:
- Requests per second (RPS): Sustained rate
- Burst: Maximum requests in a short period

## Middleware

### Logging Middleware
- Logs all HTTP requests
- Captures method, path, status, duration
- Records bytes written

### Rate Limiting Middleware
- Per-IP token bucket algorithm
- Returns 429 Too Many Requests when exceeded
- Bypassed for health endpoints

## Monitoring

The gateway exposes Prometheus metrics:

- HTTP request duration
- HTTP request count
- Rate limit hits
- Proxy errors

## Error Handling

- Rate limit exceeded: 429 Too Many Requests
- Backend unavailable: 502 Bad Gateway
- All errors are logged with context

## Deployment Notes

- Should be deployed as the external entry point
- Configure backend URL to point to PanametricFamProcessor
- Adjust rate limits based on expected traffic
- Can be scaled horizontally (rate limits are per-instance)

## Security Considerations

- Rate limiting prevents abuse
- Headers added for traceability
- No authentication (handled by CardinalGrammeter in future)
- HTTPS termination should be handled by ingress/load balancer
