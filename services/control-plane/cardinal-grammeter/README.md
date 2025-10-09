# Cardinal Grammeter Engine

Advanced traffic simulation and load generation engine for the Cloud Native Encabulator system.

## Overview

The Cardinal Grammeter Engine generates realistic traffic patterns with configurable parameters to test system behavior under various load conditions. It uses Cloud Native Encabulator terminology for educational purposes while providing sophisticated load testing capabilities.

## Features

### Traffic Patterns

1. **Steady**: Consistent request rate
2. **Burst**: Sudden spikes in traffic
3. **Ramp**: Gradually increasing load
4. **Sine**: Oscillating traffic patterns

### Configurable Parameters

#### Magneto Reluctance (0-100)
Controls request frequency and distribution:
- **Low reluctance (0-30)**: High frequency, steady traffic
- **Medium reluctance (31-70)**: Moderate frequency, variable patterns
- **High reluctance (71-100)**: Low frequency, bursty patterns

#### Capacitive Diractance (0-100)
Controls request complexity and payload size:
- **Low diractance (0-30)**: Simple requests, minimal payload
- **Medium diractance (31-70)**: Moderate complexity, standard payload
- **High diractance (71-100)**: Complex requests, large payloads

## API Endpoints

### Start Traffic Generation
```bash
POST /traffic/start
Content-Type: application/json

{
  "target": "http://dingle-arm-api-gateway:8080/api/encabulate",
  "pattern": "steady",
  "magnetoReluctance": 50,
  "capacitiveDiractance": 50
}
```

### Stop Traffic Generation
```bash
POST /traffic/stop
```

### Get Traffic Status
```bash
GET /traffic/status
```

Response:
```json
{
  "active": true,
  "target": "http://dingle-arm-api-gateway:8080/api/encabulate",
  "pattern": "steady",
  "requestsSent": 1234,
  "successRate": 98.5,
  "avgLatency": 45.2
}
```

### Update Traffic Parameters
```bash
POST /traffic/update
Content-Type: application/json

{
  "magnetoReluctance": 75,
  "capacitiveDiractance": 30
}
```

### Health Check
```bash
GET /health
```

### Metrics
```bash
GET /metrics
```

## Configuration

### Environment Variables

- `PORT`: Server port (default: `8080`)
- `NODE_ENV`: Environment (default: `production`)

## Development

### Prerequisites
- Node.js 18+
- npm or yarn

### Install Dependencies
```bash
npm install
```

### Run Development Server
```bash
npm run dev
```

### Build
```bash
npm run build
```

### Run Production
```bash
npm start
```

## Docker

Build the image:
```bash
docker build -t cardinal-grammeter:latest .
```

Run the container:
```bash
docker run -p 8080:8080 cardinal-grammeter:latest
```

## Kubernetes Deployment

The service is deployed via Helm chart. See `helm/charts/cardinal-grammeter/` for configuration.

## Integration

### Encabulation Control Console
The console provides a UI for configuring and controlling traffic generation.

### Demo Scenario Orchestrator
Automated demo scenarios can trigger traffic patterns through this service's API.

## Metrics

- `phase_detractor_traffic_starts_total`: Counter of traffic generation starts by pattern
- `phase_detractor_traffic_stops_total`: Counter of traffic generation stops
- `phase_detractor_active_traffic`: Gauge indicating if traffic generation is active

## Educational Value

The Cardinal Grammeter demonstrates:
- Load testing and traffic simulation
- Configurable traffic patterns
- Real-time metrics collection
- Integration with microservices architecture

The humorous Cloud Native Encabulator terminology (magneto reluctance, capacitive diractance) maps to real load testing concepts while making the system more engaging for educational purposes.

## License

Apache 2.0
