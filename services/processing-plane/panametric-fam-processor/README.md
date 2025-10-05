# Panametric Fam Processor

The Panametric Fam Processor is the core business logic engine for the Cloud Native Encabulator system. It orchestrates encabulation operations by coordinating with Data Plane services.

## Features

- REST API for encabulation processing
- Integration with all Data Plane services
- Caching for performance optimization
- Metrics collection and reporting
- Health and readiness endpoints
- Prometheus metrics exposure
- Structured JSON logging
- Graceful shutdown

## Technology Stack

- **Language**: TypeScript/Node.js 20
- **Framework**: Express.js
- **HTTP Client**: Axios
- **Logging**: Winston
- **Metrics**: prom-client
- **Testing**: Jest

## API Endpoints

### Processing Endpoints
- `POST /api/v1/encabulations` - Process encabulation request

### Health Endpoints
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe (checks Data Plane connectivity)
- `GET /metrics` - Prometheus metrics

## Configuration

The service is configured via environment variables:

- `PORT` - HTTP server port (default: 8080)
- `LOG_LEVEL` - Logging level (default: info)
- `HYDROCOPTIC_URL` - Hydrocoptic Data Store URL (default: http://hydrocoptic-datastore:8080)
- `CACHE_URL` - Prefabulated Cache URL (default: http://prefabulated-cache:8080)
- `SINUSOIDAL_URL` - Sinusoidal Database URL (default: http://sinusoidal-database:8080)

## Development

### Install Dependencies

```bash
npm install
```

### Build

```bash
npm run build
```

### Run

```bash
npm start
```

### Development Mode

```bash
npm run dev
```

### Test

```bash
npm test
```

### Docker Build

```bash
docker build -t panametric-fam-processor:latest .
```

## Example Request

### Process Encabulation

```bash
curl -X POST http://localhost:8080/api/v1/encabulations \
  -H "Content-Type: application/json" \
  -d '{
    "grammeters": 10.5,
    "prefabulation": true,
    "sinusoidalMode": "STANDARD",
    "priority": "HIGH"
  }'
```

### Response

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "requestId": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "status": "SUCCESS",
  "encabulationFactor": 15.75,
  "dingleArmPosition": 7.875,
  "prefabulationRate": 26.25,
  "processingTime": 145,
  "timestamp": "2024-01-01T12:00:00.000Z"
}
```

## Architecture

The processor follows a layered architecture:

```
src/
├── index.ts              # Application entry point
├── routes/               # HTTP route handlers
│   ├── encabulation.ts   # Encabulation endpoints
│   └── health.ts         # Health endpoints
├── services/             # Business logic
│   ├── processor.ts      # Core processing logic
│   └── dataPlaneClient.ts # Data Plane integration
├── types/                # TypeScript type definitions
│   └── index.ts
└── utils/                # Utilities
    ├── logger.ts         # Logging configuration
    └── metrics.ts        # Prometheus metrics
```

## Processing Flow

1. **Request Validation** - Validate input parameters
2. **Cache Check** - Check if result is cached
3. **Calculation** - Perform encabulation calculations
4. **Data Storage** - Store result in Hydrocoptic Data Store
5. **Cache Update** - Cache result for future requests
6. **Metrics Recording** - Write metrics to Sinusoidal Database
7. **Response** - Return result to client

## Metrics

The service exposes the following Prometheus metrics:

- `http_requests_total` - Total HTTP requests
- `http_request_duration_seconds` - HTTP request duration
- `encabulations_processed_total` - Total encabulations processed
- `encabulation_processing_duration_seconds` - Processing duration
- `cache_hit_rate` - Cache hit rate

## Error Handling

The service implements comprehensive error handling:

- Input validation errors return 400 Bad Request
- Processing errors return 500 Internal Server Error
- All errors are logged with context
- Errors include standardized error codes

## Dependencies

### Data Plane Services

- **Hydrocoptic Data Store** - Primary data persistence
- **Prefabulated Cache** - Result caching
- **Sinusoidal Database** - Metrics storage

All dependencies are checked during readiness probe.
