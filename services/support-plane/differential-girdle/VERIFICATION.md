# Differential Girdle Verification

## Quick Start

```bash
# Build and run
go build -o differential-girdle .
./differential-girdle
```

The service will start on port 8080.

## Manual Testing

### 1. Health Check

```bash
curl http://localhost:8080/health
# Expected: {"status":"healthy"}

curl http://localhost:8080/ready
# Expected: {"status":"ready"}
```

### 2. List Default Configurations

```bash
curl http://localhost:8080/api/v1/configs
# Expected: JSON with default configs
```

### 3. Get a Specific Configuration

```bash
curl http://localhost:8080/api/v1/configs/encabulator.max_grammeters
# Expected: Configuration details
```

### 4. Create a New Configuration

```bash
curl -X POST http://localhost:8080/api/v1/configs \
  -H "Content-Type: application/json" \
  -d '{
    "key": "test.timeout",
    "value": 30,
    "type": "integer",
    "environment": "development"
  }'
# Expected: 201 Created with config details
```

### 5. Update a Configuration

```bash
curl -X PUT http://localhost:8080/api/v1/configs/test.timeout \
  -H "Content-Type: application/json" \
  -d '{
    "key": "test.timeout",
    "value": 60,
    "type": "integer",
    "environment": "development"
  }'
# Expected: 200 OK with updated config
```

### 6. List Feature Flags

```bash
curl http://localhost:8080/api/v1/flags
# Expected: JSON with default flags
```

### 7. Create a Feature Flag

```bash
curl -X POST http://localhost:8080/api/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "name": "experimental_feature",
    "enabled": true,
    "description": "New experimental feature",
    "environment": "development",
    "rollout": 50
  }'
# Expected: 201 Created with flag details
```

### 8. Evaluate a Feature Flag

```bash
curl -X POST http://localhost:8080/api/v1/flags/experimental_feature/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123"
  }'
# Expected: {"enabled":true/false,"flag":"experimental_feature","user_id":"user123"}
```

### 9. Update a Feature Flag (Gradual Rollout)

```bash
# Start with 0% rollout
curl -X PUT http://localhost:8080/api/v1/flags/experimental_feature \
  -H "Content-Type: application/json" \
  -d '{
    "name": "experimental_feature",
    "enabled": true,
    "description": "New experimental feature",
    "environment": "development",
    "rollout": 0
  }'

# Increase to 25%
curl -X PUT http://localhost:8080/api/v1/flags/experimental_feature \
  -H "Content-Type: application/json" \
  -d '{
    "name": "experimental_feature",
    "enabled": true,
    "description": "New experimental feature",
    "environment": "development",
    "rollout": 25
  }'

# Full rollout
curl -X PUT http://localhost:8080/api/v1/flags/experimental_feature \
  -H "Content-Type: application/json" \
  -d '{
    "name": "experimental_feature",
    "enabled": true,
    "description": "New experimental feature",
    "environment": "development",
    "rollout": 100
  }'
```

### 10. Delete Resources

```bash
# Delete configuration
curl -X DELETE http://localhost:8080/api/v1/configs/test.timeout
# Expected: {"message":"configuration deleted"}

# Delete feature flag
curl -X DELETE http://localhost:8080/api/v1/flags/experimental_feature
# Expected: {"message":"feature flag deleted"}
```

### 11. Check Prometheus Metrics

```bash
curl http://localhost:8080/metrics
# Expected: Prometheus metrics output
```

## Automated Testing

```bash
# Run unit tests
go test ./... -v

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Docker Testing

```bash
# Build Docker image
docker build -t differential-girdle .

# Run container
docker run -p 8080:8080 differential-girdle

# Test from another terminal
curl http://localhost:8080/health
```

## Load Testing

```bash
# Install hey if not already installed
# go install github.com/rakyll/hey@latest

# Test health endpoint
hey -n 1000 -c 10 http://localhost:8080/health

# Test config listing
hey -n 1000 -c 10 http://localhost:8080/api/v1/configs

# Test flag evaluation
hey -n 1000 -c 10 -m POST \
  -H "Content-Type: application/json" \
  -d '{"user_id":"user123"}' \
  http://localhost:8080/api/v1/flags/sinusoidal_mode/evaluate
```

## Expected Behavior

1. **Health endpoints** should always return 200 OK
2. **Configuration CRUD** should work for all operations
3. **Feature flag CRUD** should work for all operations
4. **Flag evaluation** should respect rollout percentages
5. **Metrics endpoint** should expose Prometheus metrics
6. **Concurrent requests** should be handled safely (thread-safe store)

## Common Issues

### Port Already in Use

```bash
# Find process using port 8080
lsof -i :8080

# Kill the process
kill -9 <PID>

# Or use a different port
PORT=8081 ./differential-girdle
```

### Build Errors

```bash
# Clean and rebuild
go clean
go mod tidy
go build -o differential-girdle .
```

## Success Criteria

- ✅ All unit tests pass
- ✅ Service starts without errors
- ✅ Health endpoints respond correctly
- ✅ Configuration CRUD operations work
- ✅ Feature flag CRUD operations work
- ✅ Flag evaluation respects rollout percentages
- ✅ Metrics endpoint exposes data
- ✅ Docker image builds and runs successfully
