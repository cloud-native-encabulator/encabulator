# Cloud Native Encabulator Microservices Demo

For a number of years now, work has been proceeding in order to bring perfection to the crudely conceived idea of a cloud-native microservices platform that would not only supply inverse reactive DevOps for use in unilateral phase cultural transformation, but would also be capable of automatically synchronizing cardinal grammeters. Such an instrument is the Cloud Native Encabulator.

The original platform was prelaminated on self-managed clusters forming a substrate of pre-famulated amulite surmounted by a malleable logarithmic casing in such a way that the virtual spurving instances were directly aligned with the panametric fam. The latter consisted simply of six hydrocoptic marzlevane services, so fitted to the ambifacient lunar waneshaft broker that side fumbling was effectively prevented.

The Cloud Native Encabulator has now reached a high level of development, and it's being successfully used in the operation of Kubernetes and related novertrunnions. Moreover, whenever a forescent service request is cross-injected to the system, it may also be employed in conjunction with a drawn reciprocation dingle arm, to reduce sinusoidal repleneration.

### About the Encabulator Meme

The [Turbo Encabulator](https://en.wikipedia.org/wiki/Turbo_encabulator) is a classic engineering parody that originated in the 1940s, featuring elaborate technical jargon describing a completely fictional machine. It has been reimagined numerous times across different industries as a humorous way to poke fun at overly complex technical presentations. This project continues that tradition by applying the Encabulator's absurdist naming conventions to real cloud-native technologies—creating a fully functional Kubernetes microservices platform with nonsensical service names like "Hydrocoptic Data Store" and "Panametric Fam Processor."

While the names are whimsical, the demonstration is meant to be useful, with plans to show the evolution of such a project through a design evolution. Over time, its goal is to become a production-quality demonstration of modern microservices architecture, observability, chaos engineering, and GitOps practices.

## Overview

The Cloud Native Encabulator Microservices is a demonstration platform that provides a hands-on learning experience for Kubernetes beginners and intermediate users, DevOps engineers, product managers, and technical educators. The project showcases modern microservices patterns in a polyglot monorepo. These services don't do anything useful other than to demonstrate themselves with the backdrop of observability, resilience, and scalability within a Kubernetes environment.

## Architecture

The system is organized into four main tiers with 13 microservices:

- **Data Plane** (3 services): Handles data storage, persistence, and caching
- **Processing Plane** (3 services): Manages core business logic, message routing, and API gateway functionality
- **Support Plane** (3 services): Provides cross-cutting concerns like feature flags, metrics, and telemetry
- **Control Plane** (4 services): Interactive control, chaos engineering, and demonstration capabilities

### Services

#### Data Plane (3 services - Go)

- **Hydrocoptic Data Store**: PostgreSQL wrapper with domain-specific CRUD operations
- **Prefabulated Cache**: Redis wrapper with TTL support and caching operations
- **Sinusoidal Database**: InfluxDB wrapper for time-series data operations

[Data Plane Documentation](./services/data-plane/README.md)

#### Processing Plane (3 services - 2 Go, 1 TypeScript)

- **Panametric Fam Processor**: Core encabulation processing engine with caching (TypeScript/Node.js)
- **Logarithmic Casing Service**: Message broker and event bus for async communication (Go)
- **Dingle Arm API Gateway**: API gateway with rate limiting and routing (Go)

[Processing Plane Documentation](./services/processing-plane/README.md)

#### Support Plane (3 services - 1 Go, 2 Python)

- **Differential Girdle**: Feature flags and configuration management (Go)
- **Unilateral Phase Detractor**: Authentication and authorization service (Python/FastAPI)
- **Side Fumbling Collector**: Telemetry and observability data collection (Python/FastAPI)

[Support Plane Documentation](./services/support-plane/README.md)

#### Control Plane (4 services - 1 Go, 2 TypeScript, 1 Python)

- **Encabulation Control Console**: Interactive web UI for system control (React + TypeScript, Node.js backend)
- **Malaise Injection Engine**: Chaos engineering and failure injection (Go)
- **Cardinal Grammeter**: Advanced traffic simulation and load generation (TypeScript/Node.js)
- **Demo Scenario Orchestrator**: Automated demonstration scenario management (Python/FastAPI)

[Control Plane Documentation](./services/control-plane/README.md)

## Quick Start

### 🏗️ Build and Test Locally

Build and test all services before deploying:

```bash
# Clone the repository
git clone <repository-url>
cd cloud-native-encabulator-microservices

# Build all services
make build

# Run all tests
make test

# Build container images
make image
```

### 🧪 Comprehensive Platform Testing

Validate the entire platform with integration tests:

```bash
# Run comprehensive tests (requires Docker/Finch + kind/minikube/k3d)
./scripts/test/comprehensive-test.sh

# Or test specific components
./scripts/test/comprehensive-test.sh --quick              # Quick test
./scripts/test/comprehensive-test.sh --observability-only # Observability only
./scripts/test/test-data-services.sh                      # Data services only
```

See [TESTING.md](TESTING.md) for detailed testing guide and [scripts/README.md](scripts/README.md) for all available scripts.

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
# Direct Helm deployment - no additional UI needed

# Observability Stack (Helm-based service names)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80                    # Grafana (admin/encabulator-dev-2024)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090     # Prometheus
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-state-metrics 8080:8080       # Kube State Metrics
```

### 🏗️ Platform Architecture (Current: Helm-First)

This project currently uses a **Helm-first approach** for rapid development and learning:

- **Direct Helm Deployment**: Fast iteration with immediate feedback
- **Community Helm Charts**: PostgreSQL, Redis, and InfluxDB via community charts
- **Prometheus Stack**: Complete observability via `kube-prometheus-stack` Helm chart
- **Community Helm Charts**: Battle-tested charts for PostgreSQL, Redis, InfluxDB, Grafana, and more
- **Custom Helm Charts**: Encabulator-specific services with standardized templates

Deploy infrastructure and applications using direct Helm commands:

```bash
# Deploy complete platform
./scripts/deploy/deploy-helm.sh

# Deploy specific components
./scripts/deploy/deploy-helm.sh --observability-only
```

**Evolution Path**: Main branch will evolve through GitOps to advanced resource composition, with educational branches preserving each phase.

## Documentation

### Getting Started

- **[Setup Guide](SETUP.md)** - Environment setup and prerequisites
- **[Build Guide](BUILD.md)** - Building services and container images
- **[Testing Guide](TESTING.md)** - Local and development cluster testing
- **[Scripts Documentation](scripts/README.md)** - Build, test, and deployment scripts

### Service Planes

- **[Data Plane](./services/data-plane/README.md)** - Storage and persistence services (3 Go services)
- **[Processing Plane](./services/processing-plane/README.md)** - Business logic and API gateway (2 Go, 1 TypeScript)
- **[Support Plane](./services/support-plane/README.md)** - Feature flags, metrics, telemetry (1 Go, 2 Python)
- **[Control Plane](./services/control-plane/README.md)** - Interactive control and chaos engineering (1 Go, 2 TypeScript, 1 Python)

### Infrastructure & Deployment

- **[Helm Charts](helm/README.md)** - Custom and community Helm charts for all services
- **[Observability Platform](k8s/platform/observability/README.md)** - Complete monitoring stack via kube-prometheus-stack
- **[Infrastructure Management](k8s/infrastructure/README.md)** - In-cluster service deployment

### Architecture

- **[Architecture Summary](ARCHITECTURE_SUMMARY.md)** - High-level architecture overview
- **[Cloud Native Infrastructure](docs/architecture/cloud-native-infrastructure.md)** - Infrastructure patterns
- [API Specifications](docs/api/) _(coming soon)_
- [Cloud Native Encabulator Glossary](docs/glossary.md) _(coming soon)_

## Deployment Evolution Strategy

This project demonstrates the evolution of cloud-native deployment strategies on the `main` branch:

- **Current Phase**: **Helm-First Development** - Direct Helm deployment for rapid development
- **Future Phase**: **GitOps Migration** - Add GitOps workflow (preserve Helm-first in `helm-first` branch)
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
