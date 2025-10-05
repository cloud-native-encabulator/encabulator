# Side Fumbling Collector

Resource optimization and cleanup service for the Cloud Native Encabulator platform.

## Features

- **Resource Metrics Collection**: Gather resource usage data from Kubernetes
- **Automated Cleanup**: Scheduled cleanup of old and unused resources
- **Resource Analysis**: Data-driven analysis of resource utilization
- **Optimization Recommendations**: AI-powered suggestions for resource optimization
- **Scheduled Tasks**: Configurable periodic cleanup and analysis
- **REST API**: Simple HTTP API for all operations
- **Health Checks**: Built-in health and readiness endpoints
- **Metrics**: Prometheus metrics endpoint

## API Endpoints

### Health & Metrics

- `GET /health` - Health check endpoint
- `GET /ready` - Readiness check endpoint
- `GET /metrics` - Prometheus metrics

### Resource Metrics

- `GET /api/v1/resources/metrics` - Get current resource metrics
  - Query params: `namespace` (optional)

### Cleanup Operations

- `POST /api/v1/cleanup` - Manually trigger resource cleanup
- `GET /api/v1/cleanup/history` - Get cleanup history
  - Query params: `limit` (default: 100, max: 1000)

### Analysis & Recommendations

- `GET /api/v1/analysis/report` - Get resource analysis report
- `GET /api/v1/analysis/recommendations` - Get optimization recommendations
- `POST /api/v1/analysis/run` - Manually trigger resource analysis

## Cleanup Request Format

```json
{
  "resource_type": "Pod",
  "namespace": "encabulator-system",
  "name": "optional-specific-resource",
  "dry_run": true,
  "force": false
}
```

## Analysis Report Format

```json
{
  "timestamp": "2025-01-05T12:00:00Z",
  "total_resources": 150,
  "underutilized_resources": 25,
  "overutilized_resources": 10,
  "cleanup_candidates": 15,
  "recommendations": [
    "Found 25 underutilized resources - consider scaling down or removing",
    "Found 10 overutilized resources - consider scaling up"
  ],
  "metrics_summary": {
    "avg_cpu_usage": 0.45,
    "avg_memory_usage": 0.62,
    "max_cpu_usage": 0.95,
    "max_memory_usage": 0.88,
    "avg_age_hours": 36.5
  }
}
```

## Configuration

Environment variables:

- `PORT` - Server port (default: 8000)
- `ENVIRONMENT` - Environment name (default: development)
- `IN_CLUSTER` - Running in Kubernetes cluster (default: false)
- `NAMESPACE` - Target Kubernetes namespace (default: encabulator-system)
- `CLEANUP_ENABLED` - Enable automated cleanup (default: true)
- `CLEANUP_INTERVAL_MINUTES` - Cleanup interval (default: 60)
- `RESOURCE_AGE_THRESHOLD_HOURS` - Age threshold for cleanup (default: 24)
- `ANALYSIS_ENABLED` - Enable automated analysis (default: true)
- `ANALYSIS_INTERVAL_MINUTES` - Analysis interval (default: 30)

## Running Locally

```bash
# Install dependencies
pip install -r requirements.txt

# Run the service
python -m app.main

# Or with uvicorn
uvicorn app.main:app --reload
```

## Docker

```bash
# Build
docker build -t side-fumbling-collector .

# Run
docker run -p 8000:8000 side-fumbling-collector
```

## Scheduled Tasks

The service automatically runs two scheduled tasks:

1. **Cleanup Task**: Runs every 60 minutes (configurable)
   - Identifies resources older than threshold
   - Removes unused resources (if cleanup enabled)
   - Logs cleanup operations

2. **Analysis Task**: Runs every 30 minutes (configurable)
   - Collects resource metrics
   - Analyzes utilization patterns
   - Generates optimization recommendations

## Resource Analysis

The analyzer evaluates resources based on:

- **CPU Usage**: Low (<10%), Normal (10-80%), High (>80%)
- **Memory Usage**: Low (<20%), Normal (20-85%), High (>85%)
- **Age**: Resources older than threshold are cleanup candidates
- **Utilization Patterns**: Historical data analysis for trends

## Optimization Recommendations

The service provides recommendations for:

- **Scale Down**: Underutilized resources wasting capacity
- **Scale Up**: Overutilized resources needing more capacity
- **Cleanup**: Old, unused resources that can be removed
- **Cost Savings**: Estimated savings from optimizations

## Example Usage

### Get resource metrics

```bash
curl http://localhost:8000/api/v1/resources/metrics
```

### Trigger cleanup (dry run)

```bash
curl -X POST http://localhost:8000/api/v1/cleanup \
  -H "Content-Type: application/json" \
  -d '{
    "resource_type": "Pod",
    "namespace": "encabulator-system",
    "dry_run": true
  }'
```

### Get analysis report

```bash
curl http://localhost:8000/api/v1/analysis/report
```

### Get optimization recommendations

```bash
curl http://localhost:8000/api/v1/analysis/recommendations
```

## Data Analysis

The service uses pandas and numpy for data analysis:

- Time-series analysis of resource usage
- Statistical analysis of utilization patterns
- Trend detection for capacity planning
- Anomaly detection for unusual resource behavior

## Kubernetes Integration

In production, the service can:

- Use Kubernetes API to collect real metrics
- Delete resources via Kubernetes API
- Watch for resource changes
- Integrate with cluster autoscaler
- Export metrics to Prometheus

## Safety Features

- **Dry Run Mode**: Test cleanup operations without actual deletion
- **Age Threshold**: Only cleanup resources older than threshold
- **Namespace Isolation**: Operate within specific namespaces
- **Audit Trail**: Complete history of all cleanup operations
- **Manual Override**: Disable automated cleanup if needed

## License

Apache 2.0
