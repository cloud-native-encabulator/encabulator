# Sinusoidal Database

The Sinusoidal Database is a REST API wrapper around InfluxDB that provides time-series data operations for the Cloud Native Encabulator system.

## Features

- Write time-series metrics
- Query metrics by time range
- Tag-based filtering
- Health and readiness endpoints
- Prometheus metrics exposure
- Structured JSON logging
- Graceful shutdown

## API Endpoints

### Health Endpoints
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe (checks InfluxDB connectivity)
- `GET /metrics` - Prometheus metrics

### Metrics Endpoints
- `POST /api/v1/metrics` - Write time-series data points
- `POST /api/v1/metrics/query` - Query metrics with filters
- `GET /api/v1/metrics/range` - Query metrics by time range

## Configuration

The service is configured via environment variables:

- `PORT` - HTTP server port (default: 8080)
- `INFLUX_URL` - InfluxDB URL (default: http://localhost:8086)
- `INFLUX_TOKEN` - InfluxDB authentication token
- `INFLUX_ORG` - InfluxDB organization (default: encabulator)
- `INFLUX_BUCKET` - InfluxDB bucket (default: sinusoidal)

## Development

### Build

```bash
go build -o sinusoidal-database ./cmd/server
```

### Run

```bash
export INFLUX_URL=http://localhost:8086
export INFLUX_TOKEN=your-token
export INFLUX_ORG=encabulator
export INFLUX_BUCKET=sinusoidal
./sinusoidal-database
```

### Test

```bash
go test ./...
```

### Docker Build

```bash
docker build -t sinusoidal-database:latest .
```

## Example Requests

### Write Metrics

```bash
curl -X POST http://localhost:8080/api/v1/metrics \
  -H "Content-Type: application/json" \
  -d '{
    "points": [
      {
        "measurement": "encabulation_rate",
        "tags": {"service": "processor", "host": "server1"},
        "fields": {"value": 42.5, "efficiency": 0.95},
        "timestamp": "2024-01-01T00:00:00Z"
      }
    ]
  }'
```

### Query Metrics

```bash
curl -X POST http://localhost:8080/api/v1/metrics/query \
  -H "Content-Type: application/json" \
  -d '{
    "measurement": "encabulation_rate",
    "start": "2024-01-01T00:00:00Z",
    "end": "2024-01-01T23:59:59Z",
    "tags": {"service": "processor"}
  }'
```

### Query by Range

```bash
curl "http://localhost:8080/api/v1/metrics/range?measurement=encabulation_rate&start=2024-01-01T00:00:00Z&end=2024-01-01T23:59:59Z"
```
