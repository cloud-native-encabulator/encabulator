# Helm Charts Inventory

## Overview

This document provides a comprehensive inventory of all Helm charts used in the Cloud Native Encabulator project, including both custom service charts and upstream dependency charts.

## Custom Service Charts

All custom service charts are located in `helm/charts/` and use locally built container images.

### Data Plane (3 services)

| Service               | Chart                               | Image                          | Language | Purpose                                      |
| --------------------- | ----------------------------------- | ------------------------------ | -------- | -------------------------------------------- |
| Hydrocoptic DataStore | `helm/charts/hydrocoptic-datastore` | `hydrocoptic-datastore:latest` | Go       | PostgreSQL wrapper with domain-specific CRUD |
| Prefabulated Cache    | `helm/charts/prefabulated-cache`    | `prefabulated-cache:latest`    | Go       | Redis wrapper with TTL support               |
| Sinusoidal Database   | `helm/charts/sinusoidal-database`   | `sinusoidal-database:latest`   | Go       | InfluxDB wrapper for time-series data        |

### Processing Plane (3 services)

| Service                    | Chart                                    | Image                               | Language           | Purpose                            |
| -------------------------- | ---------------------------------------- | ----------------------------------- | ------------------ | ---------------------------------- |
| Panametric Fam Processor   | `helm/charts/panametric-fam-processor`   | `panametric-fam-processor:latest`   | TypeScript/Node.js | Core business logic and processing |
| Logarithmic Casing Service | `helm/charts/logarithmic-casing-service` | `logarithmic-casing-service:latest` | Go                 | Message broker and event bus       |
| Dingle Arm API Gateway     | `helm/charts/dingle-arm-api-gateway`     | `dingle-arm-api-gateway:latest`     | Go                 | API gateway with rate limiting     |

### Support Plane (3 services)

| Service                    | Chart                                    | Image                               | Language       | Purpose                          |
| -------------------------- | ---------------------------------------- | ----------------------------------- | -------------- | -------------------------------- |
| Unilateral Phase Detractor | `helm/charts/unilateral-phase-detractor` | `unilateral-phase-detractor:latest` | Python/FastAPI | Authentication and authorization |
| Differential Girdle        | `helm/charts/differential-girdle`        | `differential-girdle:latest`        | Go             | Feature flags and configuration  |
| Side Fumbling Collector    | `helm/charts/side-fumbling-collector`    | `side-fumbling-collector:latest`    | Python/FastAPI | Telemetry and observability data |

### Control Plane (4 services)

| Service                      | Chart                                      | Image                                 | Language                    | Purpose                                  |
| ---------------------------- | ------------------------------------------ | ------------------------------------- | --------------------------- | ---------------------------------------- |
| Encabulation Control Console | `helm/charts/encabulation-control-console` | `encabulation-control-console:latest` | React + TypeScript, Node.js | Interactive web UI and control interface |
| Malaise Injection Engine     | `helm/charts/malaise-injection-engine`     | `malaise-injection-engine:latest`     | Go                          | Chaos engineering and failure injection  |
| Cardinal Grammeter           | `helm/charts/cardinal-grammeter`           | `cardinal-grammeter:latest`           | TypeScript/Node.js          | Traffic generation and load testing      |
| Demo Scenario Orchestrator   | `helm/charts/demo-scenario-orchestrator`   | `demo-scenario-orchestrator:latest`   | Python/FastAPI              | Automated demonstration scenarios        |

## Upstream Dependency Charts

These charts are deployed via community Helm repositories and use official container images.

### Data Plane Backends

| Service    | Chart Source           | Repository                                 | Image                   | Purpose                           |
| ---------- | ---------------------- | ------------------------------------------ | ----------------------- | --------------------------------- |
| PostgreSQL | `groundhog2k/postgres` | https://groundhog2k.github.io/helm-charts/ | `docker.io/postgres:17` | Backend for Hydrocoptic DataStore |
| Redis      | `groundhog2k/redis`    | https://groundhog2k.github.io/helm-charts/ | `docker.io/redis:8`     | Backend for Prefabulated Cache    |
| InfluxDB   | `influxdata/influxdb2` | https://helm.influxdata.com/               | Official InfluxDB image | Backend for Sinusoidal Database   |

**Values Files:**

- PostgreSQL: `helm/values/data-services/values-postgresql.yaml`
- Redis: `helm/values/data-services/values-redis.yaml`
- InfluxDB: `helm/values/data-services/values-influxdb.yaml`

### Observability Stack

| Service               | Chart Source                                 | Repository                                         | Purpose                                                    |
| --------------------- | -------------------------------------------- | -------------------------------------------------- | ---------------------------------------------------------- |
| kube-prometheus-stack | `prometheus-community/kube-prometheus-stack` | https://prometheus-community.github.io/helm-charts | Complete observability (Prometheus, Grafana, Alertmanager) |

**Values File:** `helm/values/observability/values-prometheus.yaml`

**Includes:**

- Prometheus Operator
- Prometheus Server
- Grafana
- Alertmanager
- Node Exporter
- Kube State Metrics
- Prometheus Adapter

### GitOps Platform

| Service | Chart Source   | Repository                           | Purpose                    |
| ------- | -------------- | ------------------------------------ | -------------------------- |
| ArgoCD  | `argo/argo-cd` | https://argoproj.github.io/argo-helm | GitOps continuous delivery |

**Values File:** `helm/values/argocd/values-development.yaml`

## Image Pull Policy

All custom service charts use `imagePullPolicy: IfNotPresent` which:

- Uses local images if available (for development)
- Pulls from registry if not present locally
- Suitable for both local development and production deployments

## Chart Versioning

### Custom Service Charts

- **Version**: 0.1.0 or 1.0.0
- **App Version**: Matches chart version
- **Update Strategy**: Manual version bumps in Chart.yaml

### Upstream Charts

- **Version**: Specified in deployment scripts or locked in Chart.lock
- **Update Strategy**: Manual updates via `helm repo update` and version specification

## Deployment Methods

### Local Development

```bash
# Build images locally
make image

# Deploy with local images
helm upgrade --install <service-name> ./helm/charts/<service-name> \
  --namespace <namespace> \
  --create-namespace
```

### Production

```bash
# Push images to registry
docker tag <service-name>:latest <registry>/<service-name>:v1.0.0
docker push <registry>/<service-name>:v1.0.0

# Deploy with registry images
helm upgrade --install <service-name> ./helm/charts/<service-name> \
  --namespace <namespace> \
  --set image.repository=<registry>/<service-name> \
  --set image.tag=v1.0.0 \
  --set image.pullPolicy=Always
```

## Registry Considerations

### Current State (Development)

- **Custom Services**: Local images (no registry)
- **Upstream Services**: Official registries (docker.io, influxdata, etc.)

### Production Recommendations

1. **Push custom images to a container registry:**

   - AWS ECR (for EKS deployments)
   - Docker Hub
   - GitHub Container Registry (ghcr.io)
   - Harbor (self-hosted)

2. **Update image repositories in values:**

   ```yaml
   image:
     repository: <registry>/<service-name>
     tag: v1.0.0
     pullPolicy: Always
   ```

3. **Configure image pull secrets if using private registry:**
   ```yaml
   imagePullSecrets:
     - name: registry-credentials
   ```

## Chart Dependencies

### Custom Service Charts

- No external chart dependencies
- All charts are standalone
- Dependencies are runtime service-to-service connections

### Upstream Charts

- kube-prometheus-stack has multiple sub-charts
- ArgoCD has optional dependencies (Redis for HA)
- Data backend charts are standalone

## Monitoring Integration

All custom service charts include:

- ServiceMonitor resources for Prometheus scraping
- Health and readiness probes
- Prometheus metrics endpoints at `/metrics`
- Grafana dashboard annotations

## Security Considerations

### Image Security

- **Official Images**: Using official upstream images (postgres, redis)
- **Custom Images**: Built from source with multi-stage Dockerfiles
- **Scanning**: Recommend scanning images with Trivy or similar tools

### Chart Security

- Security contexts defined for all pods
- Non-root users where possible
- Read-only root filesystems where applicable
- Network policies can be enabled via values

## Maintenance

### Updating Custom Charts

1. Update service code
2. Build new image: `make image`
3. Update Chart.yaml version if needed
4. Test deployment
5. Commit changes

### Updating Upstream Charts

1. Check for updates: `helm repo update`
2. Review changelog for breaking changes
3. Update version in deployment scripts
4. Test in development environment
5. Update values files if needed
6. Deploy to production

## Chart Structure

All custom charts follow this structure:

```
helm/charts/<service-name>/
├── Chart.yaml              # Chart metadata
├── values.yaml             # Default values
├── templates/
│   ├── _helpers.tpl       # Template helpers
│   ├── deployment.yaml    # Deployment resource
│   ├── service.yaml       # Service resource
│   ├── servicemonitor.yaml # Prometheus ServiceMonitor
│   └── NOTES.txt          # Post-install notes
└── README.md              # Chart documentation (optional)
```

## Testing Charts

### Lint Charts

```bash
helm lint helm/charts/<service-name>
```

### Template Rendering

```bash
helm template <service-name> helm/charts/<service-name>
```

### Dry Run

```bash
helm install <service-name> helm/charts/<service-name> --dry-run --debug
```

### Full Deployment Test

```bash
helm upgrade --install <service-name> helm/charts/<service-name> \
  --namespace test \
  --create-namespace \
  --wait
```

## Related Documentation

- [Helm Charts README](./README.md) - Overview and usage
- [Data Services Values](./values/data-services/README.md) - Backend configuration
- [Deployment Scripts](../scripts/README.md) - Automated deployment
- [Build Guide](../BUILD.md) - Building container images

## Summary

- **Total Custom Charts**: 13 (all service planes)
- **Total Upstream Charts**: 5 (data backends + observability + GitOps)
- **Image Registries**: docker.io (official images), local (custom images)
- **Deployment Tool**: Helm 3.x
- **GitOps Tool**: ArgoCD (optional)
- **Monitoring**: Prometheus + Grafana via kube-prometheus-stack
