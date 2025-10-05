# Logarithmic Casing Service

The Logarithmic Casing Service is a message broker wrapper that provides REST API access to NATS messaging for the Cloud Native Encabulator system.

## Features

- REST API for message publishing
- NATS message broker integration
- Topic-based pub/sub messaging
- Message routing and delivery
- Broker statistics and monitoring
- Health and readiness endpoints
- Prometheus metrics exposure
- Structured JSON logging
- Graceful shutdown
- Automatic reconnection

## Technology Stack

- **Language**: Go 1.21
- **Message Broker**: NATS
- **HTTP Framework**: Gorilla Mux
- **Logging**: Zerolog
- **Metrics**: Prometheus client

## API Endpoints

### Messaging Endpoints
- `POST /api/v1/messages` - Publish a message to a topic
- `GET /api/v1/stats` - Get broker statistics
- `GET /api/v1/topics/{topic}/stats` - Get topic-specific statistics

### Health Endpoints
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe (checks NATS connectivity)
- `GET /metrics` - Prometheus metrics

## Configuration

The service is configured via environment variables:

- `PORT` - HTTP server port (default: 8080)
- `NATS_URL` - NATS server URL (default: nats://localhost:4222)

## Development

### Build

```bash
go build -o logarithmic-casing-service ./cmd/server
```

### Run

```bash
export NATS_URL=nats://localhost:4222
./logarithmic-casing-service
```

### Test

```bash
go test ./...
```

### Docker Build

```bash
docker build -t logarithmic-casing-service:latest .
```

## Example Requests

### Publish Message

```bash
curl -X POST http://localhost:8080/api/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "encabulation.events",
    "payload": {
      "eventType": "encabulation.completed",
      "encabulationId": "550e8400-e29b-41d4-a716-446655440000",
      "status": "SUCCESS"
    },
    "headers": {
      "source": "panametric-fam-processor",
      "priority": "high"
    }
  }'
```

### Response

```json
{
  "messageId": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "topic": "encabulation.events",
  "timestamp": "2024-01-01T12:00:00.000Z"
}
```

### Get Broker Stats

```bash
curl http://localhost:8080/api/v1/stats
```

### Response

```json
{
  "totalMessages": 1523,
  "totalTopics": 5,
  "totalSubscribers": 8,
  "topics": [
    {
      "topic": "encabulation.events",
      "messagesCount": 842,
      "subscriberCount": 3
    }
  ]
}
```

## Architecture

The service follows a layered architecture:

```
cmd/server/           # Application entry point
internal/
├── api/              # HTTP handlers
├── broker/           # NATS broker wrapper
└── models/           # Data models
pkg/health/           # Health check utilities
```

## Message Flow

1. **HTTP Request** - Client sends message via REST API
2. **Validation** - Validate topic and payload
3. **Message Creation** - Create message with ID and timestamp
4. **NATS Publish** - Publish to NATS broker
5. **Stats Update** - Update broker statistics
6. **Response** - Return message ID to client

## NATS Integration

The service wraps NATS functionality:

- **Connection Management** - Automatic reconnection with exponential backoff
- **Pub/Sub** - Topic-based message routing
- **Headers** - Support for message headers
- **Statistics** - Track messages per topic

## Monitoring

The service exposes Prometheus metrics:

- HTTP request metrics
- Message publish metrics
- NATS connection status
- Topic statistics

## Error Handling

- Input validation errors return 400 Bad Request
- NATS connection errors return 503 Service Unavailable
- Publishing errors return 500 Internal Server Error
- All errors are logged with context

## Dependencies

### External Services

- **NATS Server** - Message broker backend

The NATS connection is checked during readiness probe.

## Deployment Notes

- Requires NATS server to be running
- Supports NATS clustering for high availability
- Automatic reconnection on connection loss
- Graceful shutdown ensures message delivery
