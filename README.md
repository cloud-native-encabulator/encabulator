# Cloud Native Encabulator Microservices Demo

A comprehensive educational project demonstrating Kubernetes microservices architecture using the humorous Cloud Native Encabulator narrative as a framework.

## Overview

The Cloud Native Encabulator Microservices Demo provides a hands-on learning experience for Kubernetes beginners and intermediate users, DevOps engineers, product managers, and technical educators. The project showcases modern microservices patterns, observability, resilience, and scalability within a Kubernetes environment.

## Architecture

The system is organized into three main planes:

- **Data Plane**: Handles data storage, persistence, and caching
- **Processing Plane**: Manages core business logic, message routing, and API gateway functionality  
- **Support Plane**: Provides cross-cutting concerns like authentication, configuration, and resource management

### Services

#### Data Plane
- **Hydrocoptic Data Store**: Primary persistence layer (PostgreSQL with custom wrapper)
- **Sinusoidal Database**: Time-series metrics storage
- **Prefabulated Cache**: In-memory data caching (Redis)

#### Processing Plane
- **Panametric Fam Processor**: Core business logic execution engine (Node.js)
- **Logarithmic Casing Service**: Message broker and event bus (NATS/RabbitMQ)
- **Dingle Arm API Gateway**: External API facade and entry point (Envoy/NGINX)

#### Support Plane
- **Cardinal Grammeter**: Authentication and authorization service (Python/FastAPI)
- **Differential Girdle**: Configuration management service (Go)
- **Side Fumbling Collector**: Resource optimization and cleanup service (Go/Kubernetes Operator)

## Quick Start

### 🧪 Testing First (Recommended)

Start with local testing to validate the platform before deploying to AWS:

```bash
# Clone the repository
git clone <repository-url>
cd cloud-native-encabulator-microservices

# Run local tests (requires Docker + kind/minikube/k3d)
./scripts/test/local-test.sh

# Access local services
kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000
```

See [TESTING.md](TESTING.md) for comprehensive testing guide.

### ☁️ Development Cluster Deployment

For full AWS integration testing:

#### Prerequisites
- EKS cluster (or any Kubernetes cluster)
- kubectl configured with cluster access
- AWS CLI configured
- Helm

#### Setup and Deploy
```bash
# Ensure your EKS cluster context is active
aws eks update-kubeconfig --region <region> --name <cluster-name>

# Verify setup and configure environment
./scripts/setup/eks-setup.sh

# Deploy complete platform with KRO and observability
./scripts/deploy/deploy-dev.sh

# Check deployment status
./scripts/deploy/status.sh
```

### 🌐 Access the Platform

After deployment, access the services:

```bash
# ArgoCD UI (GitOps dashboard)
kubectl port-forward -n argocd svc/argocd-server 8080:443
# Visit: https://localhost:8080 (admin/<get-password-below>)

# Get ArgoCD admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Observability Stack
kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000          # Grafana (admin/encabulator-dev-2024)
kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-metrics-prometheus 9090:9090  # Prometheus
kubectl port-forward -n encabulator-tracing svc/encabulator-observability-tracing-jaeger 16686:16686        # Jaeger
```

### 🏗️ Platform Architecture

This project uses a **KRO-based platform engineering approach**:

- **KRO (Kube Resource Orchestrator)**: Creates custom APIs for complex resource compositions
- **ArgoCD**: GitOps deployment and management
- **ACK (AWS Controllers for Kubernetes)**: Native AWS resource management
- **Custom ResourceGroups**: Simplified deployment of complex stacks

Instead of managing individual YAML files, deploy complete stacks with simple custom resources:

```yaml
apiVersion: observability.encabulator.io/v1alpha1
kind: ObservabilityStack
metadata:
  name: my-monitoring
spec:
  environment: development
  # Automatically deploys Prometheus, Grafana, Jaeger, Loki
```

## Documentation

- **[Testing Guide](TESTING.md)** - Local and development cluster testing
- **[Setup Guide](SETUP.md)** - Environment setup and prerequisites  
- **[Observability Platform](k8s/platform/observability/README.md)** - KRO-based monitoring stack
- **[Infrastructure Management](k8s/infrastructure/README.md)** - ACK and KRO resource orchestration
- [Architecture Documentation](docs/architecture/) *(coming soon)*
- [API Specifications](docs/api/) *(coming soon)*
- [Cloud Native Encabulator Glossary](docs/glossary.md) *(coming soon)*

## Technology Exploration Branches

Explore different Kubernetes ecosystem technologies:

- `gitops`: GitOps deployment with Flux/ArgoCD
- `karpenter`: Dynamic node provisioning with Karpenter
- `operators`: Custom Kubernetes Operators and CRDs
- `service-mesh`: Service mesh with Istio/Linkerd
- `serverless`: Serverless workloads with Knative

## Demo Scenarios

The project includes several predefined demo scenarios:

1. **Basic Operations**: System startup and data flow visualization
2. **Failure Recovery**: Service outage simulation and recovery
3. **Scaling**: Horizontal scaling under load demonstration
4. **Configuration**: Dynamic configuration updates

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Developer Certificate of Origin

This project requires all contributors to sign off on their commits using the Developer Certificate of Origin (DCO). Add `-s` to your git commit command:

```bash
git commit -s -m "Your commit message"
```

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by the classic Turbo Encabulator technical presentation (now reimagined for cloud-native architecture)
- Built with modern cloud-native technologies
- Designed for educational and demonstration purposes

## Support

- [Issues](https://github.com/your-org/cloud-native-encabulator-microservices/issues)
- [Discussions](https://github.com/your-org/cloud-native-encabulator-microservices/discussions)
- [Documentation](docs/)

---

*"The original machine had a base plate of pre-famulated amulite surmounted by a malleable logarithmic casing..."*