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
- **PostgreSQL**: `postgresql/postgresql` (Bitnami) - Hydrocoptic Data Store backend
- **Redis**: `redis/redis` (Bitnami) - Prefabulated Cache backend  
- **InfluxDB**: `influxdata/influxdb2` (InfluxData) - Sinusoidal Database backend

### Infrastructure Services

#### ArgoCD
- **Chart**: `argo/argo-cd`
- **Repository**: https://argoproj.github.io/argo-helm
- **Benefits**: Official chart, well-maintained, production-ready

#### ACK Controllers (Official AWS Charts)
- **RDS**: `oci://public.ecr.aws/aws-controllers-k8s/rds-chart`
- **ElastiCache**: `oci://public.ecr.aws/aws-controllers-k8s/elasticache-chart`
- **IAM**: `oci://public.ecr.aws/aws-controllers-k8s/iam-chart`
- **Benefits**: Official AWS OCI charts, per-service installation

#### Observability Stack
- **Prometheus Stack**: `prometheus-community/kube-prometheus-stack`
- **Grafana**: `grafana/grafana` (standalone option)
- **Redis**: `redis/redis`
- **PostgreSQL**: `postgresql/postgresql`

#### Future Enhancements
- **Jaeger**: `jaegertracing/jaeger`
- **Loki**: `grafana/loki-stack`

## Implementation Status

1. **Task 1**: Project infrastructure ✅
2. **Task 2**: Database backends via upstream charts ⏳
3. **Task 3**: Observability stack via kube-prometheus-stack ✅
4. **Task 4**: Service applications (future - will use custom charts) ⏳
5. **Task 6**: GitOps evolution (future) ⏳

## Deployment

Deploy database backends using upstream charts:

```bash
# Deploy all data services
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

## Benefits of Upstream Charts Approach

### For Infrastructure (Task 2)
- **Battle-Tested**: Leverage community-maintained charts with thousands of deployments
- **Security**: Built-in security best practices and regular security updates
- **Maintenance**: Reduced maintenance overhead - no custom database chart code
- **Standards**: Follow Kubernetes and Helm best practices automatically

### For Applications (Task 4 - Future)
- **Custom Business Logic**: Focus on service APIs and business logic
- **Clean Separation**: Infrastructure (upstream) vs Applications (custom)
- **Reusability**: Service charts can be reused across environments
- **Integration**: Easy integration with upstream database backends

### For Operations
- **Version Management**: Easy upgrades and rollbacks with semantic versioning
- **Configuration Management**: Environment-specific values without chart duplication
- **Observability**: Built-in monitoring integration with ServiceMonitors
- **Scaling**: HPA and resource management built into upstream charts

## Values File Structure

Each service has a dedicated values file with:
- **Resource Management**: CPU/memory requests and limits
- **Security Context**: Non-root users, dropped capabilities
- **Persistence**: Storage configuration with default storage class
- **Monitoring**: ServiceMonitor configuration for Prometheus
- **Authentication**: Service-specific credentials and configuration

See `helm/values/data-services/README.md` for detailed configuration options.