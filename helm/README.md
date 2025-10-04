# Cloud Native Encabulator - Helm Configuration

This directory contains Helm values files and configurations for deploying all infrastructure components using upstream community charts with custom configuration.

## Architecture

```
helm/
├── values/                # Helm values files for upstream charts
│   ├── data-services/    # Database backend configurations
│   ├── observability/    # Monitoring stack configurations
│   └── scaling/          # Auto-scaling configurations
└── README.md             # This file
```

## Upstream Charts Used

### Data Services (Task 2.1)

- **PostgreSQL**: `groundhog2k/postgres` (Community) - Hydrocoptic Data Store backend
- **Redis**: `groundhog2k/redis` (Community) - Prefabulated Cache backend
- **InfluxDB**: `influxdata/influxdb2` (InfluxData) - Sinusoidal Database backend

### Observability Services (Task 3)

- **Prometheus Stack**: `prometheus-community/kube-prometheus-stack`
- **Grafana**: Included in kube-prometheus-stack
- **AlertManager**: Included in kube-prometheus-stack

### Future Enhancements (Task 6+)

#### GitOps (Task 6.1)

- **ArgoCD**: `argo/argo-cd`
- **Repository**: https://argoproj.github.io/argo-helm
- **Benefits**: Official chart, well-maintained, production-ready GitOps

#### AWS Controllers (Task 6.2)

- **RDS**: `oci://public.ecr.aws/aws-controllers-k8s/rds-chart`
- **ElastiCache**: `oci://public.ecr.aws/aws-controllers-k8s/elasticache-chart`
- **IAM**: `oci://public.ecr.aws/aws-controllers-k8s/iam-chart`
- **Benefits**: Official AWS OCI charts, managed AWS services

#### Additional Observability

- **Jaeger**: `jaegertracing/jaeger` (distributed tracing)
- **Loki**: `grafana/loki-stack` (log aggregation)

## Implementation Status

1. **Task 1**: Project infrastructure ✅
2. **Task 2**: Database backends via community charts ✅
3. **Task 3**: Observability stack via kube-prometheus-stack ✅
4. **Task 4**: Service applications (next - will use custom charts) ⏳
5. **Task 6**: GitOps evolution (future) ⏳
6. **Task 6+**: AWS managed services migration (future) ⏳

## Deployment

Deploy database backends using upstream charts:

```bash
# Deploy all data services
./scripts/deploy/deploy-helm.sh --data-plane-only

# Or deploy individually
helm upgrade --install hydrocoptic-postgresql groundhog2k/postgres \
  --namespace encabulator-data \
  --values helm/values/data-services/values-postgresql.yaml

helm upgrade --install prefabulated-cache groundhog2k/redis \
  --namespace encabulator-data \
  --values helm/values/data-services/values-redis.yaml

helm upgrade --install sinusoidal-database influxdata/influxdb2 \
  --namespace encabulator-data \
  --values helm/values/data-services/values-influxdb.yaml
```

## Benefits of Helm-First Approach

### Current Implementation (Tasks 1-4)

- **Battle-Tested Charts**: Leverage community-maintained charts with thousands of deployments
- **Rapid Development**: Direct Helm deployment for fast iteration and testing
- **Clean Architecture**: Clear separation between infrastructure and application layers
- **No Vendor Lock-in**: Avoid proprietary charts and monetized images

### Infrastructure Layer (Task 2 ✅)

- **Community Charts**: PostgreSQL, Redis, InfluxDB via open-source charts
- **Official Images**: Direct use of upstream Docker images (postgres, redis, influxdb)
- **Security**: Built-in security best practices and regular updates
- **Maintenance**: Reduced overhead - no custom database chart maintenance

### Application Layer (Task 4 ⏳)

- **Custom Business Logic**: Focus on Encabulator service APIs and functionality
- **Service Wrappers**: REST APIs that provide business logic on top of databases
- **Monitoring Integration**: Built-in ServiceMonitors and health endpoints
- **Reusability**: Service charts can be deployed across environments

### Future Evolution (Task 6+)

- **GitOps Migration**: Seamless transition to ArgoCD-managed deployments
- **AWS Integration**: Migration to managed services (RDS, ElastiCache)
- **Platform Engineering**: Advanced patterns with KRO and custom operators

## Values File Structure

Each service has a dedicated values file with:

- **Resource Management**: CPU/memory requests and limits
- **Security Context**: Non-root users, dropped capabilities
- **Persistence**: Storage configuration with default storage class
- **Monitoring**: ServiceMonitor configuration for Prometheus
- **Authentication**: Service-specific credentials and configuration

See `helm/values/data-services/README.md` for detailed configuration options.
