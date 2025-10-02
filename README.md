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

Start with comprehensive testing to validate the entire platform:

```bash
# Clone the repository
git clone <repository-url>
cd cloud-native-encabulator-microservices

# Run comprehensive tests (requires Docker/Finch + kind/minikube/k3d)
./scripts/test/comprehensive-test.sh

# Or test specific components
./scripts/test/local-test.sh            # Observability stack only

# Access services (updated service names)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80
```

See [TESTING.md](TESTING.md) for detailed testing guide.

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

# Deploy complete platform via Helm-first approach
./scripts/deploy/deploy-helm.sh

# Check deployment status
./scripts/deploy/deploy-helm.sh --validate
```

### 🌐 Access the Platform

After deployment, access the services:

```bash
# ArgoCD UI (GitOps dashboard)
kubectl port-forward -n argocd svc/argocd-server 8080:443
# Visit: https://localhost:8080 (admin/<get-password-below>)

# Get ArgoCD admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Observability Stack (Helm-based service names)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80                    # Grafana (admin/encabulator-dev-2024)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090     # Prometheus
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-state-metrics 8080:8080       # Kube State Metrics
```

### 🏗️ Platform Architecture (Current: Helm-First)

This project currently uses a **Helm-first approach** for rapid development and learning:

- **Direct Helm Deployment**: Fast iteration with immediate feedback
- **Official Helm Charts**: PostgreSQL and Redis via official community charts
- **Prometheus Stack**: Complete observability via `kube-prometheus-stack` Helm chart
- **Community Helm Charts**: Battle-tested charts for Redis, PostgreSQL, Grafana, and more
- **Custom Helm Charts**: Encabulator-specific services with standardized templates

Deploy infrastructure and applications using direct Helm commands:

```bash
# Deploy complete platform
./scripts/deploy/deploy-helm.sh

# Deploy specific components
./scripts/deploy/deploy-helm.sh --observability-only
```

**Evolution Path**: Main branch will evolve through GitOps (ArgoCD) to advanced resource composition (KRO), with educational branches preserving each phase.

## Documentation

- **[Testing Guide](TESTING.md)** - Local and development cluster testing
- **[Setup Guide](SETUP.md)** - Environment setup and prerequisites  
- **[Observability Platform](k8s/platform/observability/README.md)** - Complete monitoring stack via kube-prometheus-stack Helm chart
- **[Infrastructure Management](k8s/infrastructure/README.md)** - In-cluster service deployment via official Helm charts
- **[Helm Charts](helm/README.md)** - Custom and community Helm charts for all services
- [Architecture Documentation](docs/architecture/) *(coming soon)*
- [API Specifications](docs/api/) *(coming soon)*
- [Cloud Native Encabulator Glossary](docs/glossary.md) *(coming soon)*

## Deployment Evolution Strategy

This project demonstrates the evolution of cloud-native deployment strategies on the `main` branch:

- **Current Phase**: **Helm-First Development** - Direct Helm deployment for rapid development
- **Future Phase**: **GitOps Migration** - Add ArgoCD workflow (preserve Helm-first in `helm-first` branch)
- **Final Phase**: **KRO Platform Engineering** - Advanced resource composition (preserve GitOps in `argo-first` branch)

**Educational Branches:**
- **`helm-first`**: Preserved Helm-only implementation (created when GitOps added to main)
- **`argo-first`**: Preserved GitOps implementation (created when KRO added to main)

See [BRANCH_STRATEGY.md](BRANCH_STRATEGY.md) for detailed evolution plan and learning paths.

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