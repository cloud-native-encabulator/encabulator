# Differential Girdle

Configuration management service with feature flags for the Cloud Native Encabulator platform.

## Features

- **Configuration Management**: Store and retrieve configuration values with metadata
- **Feature Flags**: Dynamic feature flag management with rollout percentages
- **Environment Support**: Multi-environment configuration support
- **REST API**: Simple HTTP API for all operations
- **Health Checks**: Built-in health and readiness endpoints
- **Metrics**: Prometheus metrics endpoint

## API Endpoints

### Health & Metrics

- `GET /health` - Health check endpoint
- `GET /ready` - Readiness check endpoint
- `GET /metrics` - Prometheus metrics

### Configuration Management

- `GET /api/v1/configs` - List all configurations
- `GET /api/v1/configs/:key` - Get a specific configuration
- `POST /api/v1/configs` - Create a new configuration
- `PUT /api/v1/configs/:key` - Update a configuration
- `DELETE /api/v1/configs/:key` - Delete a configuration

### Feature Flags

- `GET /api/v1/flags` - List all feature flags
- `GET /api/v1/flags/:name` - Get a specific feature flag
- `POST /api/v1/flags` - Create a new feature flag
- `PUT /api/v1/flags/:name` - Update a feature flag
- `DELETE /api/v1/flags/:name` - Delete a feature flag
- `POST /api/v1/flags/:name/evaluate` - Evaluate a feature flag for a user

## Configuration Value Format

```json
{
  "key": "encabulator.max_grammeters",
  "value": 100,
  "type": "integer",
  "environment": "production",
  "metadata": {
    "description": "Maximum number of grammeters"
  }
}
```

## Feature Flag Format

```json
{
  "name": "sinusoidal_mode",
  "enabled": true,
  "description": "Enable sinusoidal depleneration mode",
  "environment": "production",
  "rollout": 50,
  "metadata": {
    "owner": "platform-team"
  }
}
```

## Rollout Logic

Feature flags support gradual rollout using the `rollout` field (0-100):
- `0`: Feature is disabled for all users
- `100`: Feature is enabled for all users
- `1-99`: Feature is enabled for a percentage of users based on user ID hash

## Running Locally

```bash
# Install dependencies
go mod download

# Run the service
go run main.go

# Or build and run
go build -o differential-girdle
./differential-girdle
```

## Environment Variables

- `PORT` - Server port (default: 8080)
- `ENVIRONMENT` - Environment name (default: development)
- `GIN_MODE` - Gin mode: debug, release (default: release)

## Docker

```bash
# Build
docker build -t differential-girdle .

# Run
docker run -p 8080:8080 differential-girdle
```

## Testing

```bash
# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...
```

## Example Usage

### Create a configuration

```bash
curl -X POST http://localhost:8080/api/v1/configs \
  -H "Content-Type: application/json" \
  -d '{
    "key": "encabulator.timeout",
    "value": 30,
    "type": "integer",
    "environment": "production"
  }'
```

### Create a feature flag

```bash
curl -X POST http://localhost:8080/api/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "name": "new_feature",
    "enabled": true,
    "description": "New experimental feature",
    "environment": "production",
    "rollout": 25
  }'
```

### Evaluate a feature flag

```bash
curl -X POST http://localhost:8080/api/v1/flags/new_feature/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123"
  }'
```

## Architecture

The service uses an in-memory store for configuration and feature flags. In a production environment, this could be backed by:
- Kubernetes ConfigMaps/Secrets
- etcd
- Consul
- Redis
- Database

## License

Apache 2.0
