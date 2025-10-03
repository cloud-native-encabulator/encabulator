# Data Services Configuration

This directory contains Helm values files for the Cloud Native Encabulator data plane backend services.

## Overview

The data plane consists of three backend databases that provide persistent storage for the Encabulator microservices:

1. **PostgreSQL** - Relational database for the Hydrocoptic Data Store
2. **Redis** - In-memory cache for the Prefabulated Cache  
3. **InfluxDB** - Time-series database for the Sinusoidal Database

## Configuration Files

### `values-postgresql.yaml`
- **Purpose**: Backend database for Hydrocoptic Data Store service
- **Chart**: `postgresql/postgresql` (Bitnami)
- **Instance**: `hydrocoptic-postgresql`
- **Database**: `hydrocoptic_db`
- **User**: `hydrocoptic`
- **Resources**: 200m CPU, 256Mi memory (requests)
- **Storage**: 8Gi persistent volume

### `values-redis.yaml`
- **Purpose**: Backend cache for Prefabulated Cache service
- **Chart**: `redis/redis` (Bitnami)
- **Mode**: Standalone (no replicas for development)
- **Resources**: 100m CPU, 128Mi memory (requests)
- **Storage**: 4Gi persistent volume
- **Authentication**: Password-based

### `values-influxdb.yaml`
- **Purpose**: Backend time-series database for Sinusoidal Database service
- **Chart**: `influxdata/influxdb2` (InfluxData)
- **Organization**: `encabulator`
- **Bucket**: `sinusoidal-data`
- **Resources**: 100m CPU, 256Mi memory (requests)
- **Storage**: 8Gi persistent volume

## Deployment

These values files are used by the deployment script:

```bash
# Deploy all data backends
./scripts/deploy/deploy-helm.sh --data-plane-only

# Or deploy individually
helm upgrade --install hydrocoptic-postgresql postgresql/postgresql \
  --namespace encabulator-data \
  --values helm/values/data-services/values-postgresql.yaml

helm upgrade --install prefabulated-cache redis/redis \
  --namespace encabulator-data \
  --values helm/values/data-services/values-redis.yaml

helm upgrade --install sinusoidal-database influxdata/influxdb2 \
  --namespace encabulator-data \
  --values helm/values/data-services/values-influxdb.yaml
```

## Testing

Test connectivity to all data backends:

```bash
./scripts/test/test-data-services.sh
```

Test individual services:

```bash
./scripts/test/test-data-services.sh --postgresql
./scripts/test/test-data-services.sh --redis  
./scripts/test/test-data-services.sh --influxdb
```

## Security

All databases are configured with:
- **Authentication**: Username/password or token-based
- **Security Context**: Non-root users, dropped capabilities
- **Network Policies**: Configurable (disabled by default)
- **Resource Limits**: CPU and memory constraints

## Monitoring

All databases include:
- **Prometheus Metrics**: Exposed via ServiceMonitor
- **Health Checks**: Liveness and readiness probes
- **Resource Monitoring**: CPU, memory, and storage metrics

## Credentials

Default development credentials (change for production):

- **PostgreSQL**: `hydrocoptic` / `hydrocoptic-password`
- **Redis**: Password: `prefabulated-cache-password`
- **InfluxDB**: `sinusoidal-admin` / `sinusoidal-password`

## Storage

All databases use:
- **Storage Class**: Default cluster storage class
- **Persistence**: Enabled with appropriate sizes
- **Backup**: Not configured (add for production)

## Scaling

Development configuration uses:
- **Single instances**: No replicas or clustering
- **Resource requests**: Minimal for development
- **Pod Disruption Budgets**: Configured for availability

For production, consider:
- **High Availability**: Enable replicas and clustering
- **Resource Scaling**: Increase CPU/memory limits
- **Storage Scaling**: Use larger persistent volumes
- **Backup Strategy**: Implement regular backups