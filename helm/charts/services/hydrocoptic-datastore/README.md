# Hydrocoptic DataStore Helm Chart

A Helm chart for deploying the Hydrocoptic Data Store service with embedded PostgreSQL database for the Cloud Native Encabulator platform.

## Overview

The Hydrocoptic DataStore is a critical component of the Cloud Native Encabulator's data plane, providing persistent storage capabilities with built-in monitoring and high availability features.

## Features

- **Embedded PostgreSQL**: Includes PostgreSQL database as a dependency
- **Service Connectivity**: Automatic credential management and service discovery
- **Monitoring**: Built-in Prometheus metrics and ServiceMonitor
- **High Availability**: Configurable replicas with Pod Disruption Budget
- **Auto-scaling**: Horizontal Pod Autoscaler support
- **Security**: Network policies and security contexts
- **Health Checks**: Liveness and readiness probes

## Installation

### Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

### Install with embedded PostgreSQL (default)

```bash
helm install hydrocoptic-datastore ./helm/charts/hydrocoptic-datastore \
  --namespace encabulator-data \
  --create-namespace
```

### Install with external PostgreSQL

```bash
helm install hydrocoptic-datastore ./helm/charts/hydrocoptic-datastore \
  --namespace encabulator-data \
  --create-namespace \
  --set postgresql.enabled=false \
  --set externalDatabase.host=my-postgres-host \
  --set externalDatabase.username=myuser \
  --set externalDatabase.password=mypassword \
  --set externalDatabase.database=mydatabase
```

## Configuration

### Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `postgresql.enabled` | Enable embedded PostgreSQL | `true` |
| `postgresql.auth.database` | PostgreSQL database name | `hydrocoptic_db` |
| `postgresql.auth.username` | PostgreSQL username | `hydrocoptic` |
| `postgresql.auth.password` | PostgreSQL password | `hydrocoptic-password` |
| `replicaCount` | Number of service replicas | `2` |
| `autoscaling.enabled` | Enable HPA | `true` |
| `monitoring.serviceMonitor.enabled` | Enable Prometheus monitoring | `true` |
| `networkPolicy.enabled` | Enable network policies | `false` |

### Service Connectivity

The chart automatically creates a secret with database connection details:

- **Secret Name**: `{release-name}-credentials`
- **Keys Available**:
  - `postgres-host`: Database hostname
  - `postgres-port`: Database port
  - `postgres-database`: Database name
  - `postgres-username`: Database username
  - `postgres-password`: Database password
  - `database-url`: Complete connection string

### Environment Variables

The service pod receives these environment variables:

- `DATABASE_HOST`: PostgreSQL hostname
- `DATABASE_PORT`: PostgreSQL port
- `DATABASE_NAME`: Database name
- `DATABASE_USER`: Database username
- `DATABASE_PASSWORD`: Database password (from secret)
- `DATABASE_URL`: Complete connection string (from secret)

## Monitoring

The chart includes built-in monitoring capabilities:

- **Metrics Endpoint**: `/metrics` on port 8080
- **Health Endpoint**: `/health` for liveness probes
- **Ready Endpoint**: `/ready` for readiness probes
- **ServiceMonitor**: Automatic Prometheus scraping configuration

## Security

Security features include:

- **Security Context**: Non-root user, read-only filesystem
- **Network Policies**: Configurable ingress/egress rules
- **Service Account**: Dedicated service account with minimal permissions
- **Secret Management**: Secure credential storage and injection

## Testing

Test the deployment with the provided test script:

```bash
./scripts/test/test-data-services.sh --hydrocoptic
```

## Troubleshooting

### Common Issues

1. **PostgreSQL Connection Failed**
   - Check if PostgreSQL pod is running: `kubectl get pods -n encabulator-data -l app.kubernetes.io/name=postgresql`
   - Verify credentials in secret: `kubectl get secret {release-name}-credentials -o yaml`

2. **Service Not Accessible**
   - Check service status: `kubectl get svc -n encabulator-data`
   - Verify network policies if enabled

3. **Monitoring Not Working**
   - Ensure ServiceMonitor is created: `kubectl get servicemonitor -n encabulator-data`
   - Check Prometheus configuration for namespace discovery

### Logs

View service logs:
```bash
kubectl logs -n encabulator-data -l app.kubernetes.io/name=hydrocoptic-datastore
```

View PostgreSQL logs:
```bash
kubectl logs -n encabulator-data -l app.kubernetes.io/name=postgresql
```

## Development

For local development and testing:

1. Install dependencies: `helm dependency update`
2. Lint the chart: `helm lint .`
3. Test template rendering: `helm template test . --debug`
4. Deploy to test cluster: `helm install test . --dry-run`

## License

Apache License 2.0