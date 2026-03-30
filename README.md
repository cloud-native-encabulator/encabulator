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

## The Encabulation Workload

### What is an Encabulation?

An **encabulation** is the core data processing unit in this system. Each encabulation represents a request that flows through the entire microservices architecture, demonstrating real distributed system behavior.

**Encabulation Structure:**

```json
{
  "grammeters": 42,
  "prefabulation": true,
  "sinusoidalMode": "STANDARD"
}
```

**Parameters:**

- `grammeters` - Cardinal grammeter count (affects processing complexity)
- `prefabulation` - Whether to use cached prefabulated results
- `sinusoidalMode` - Processing algorithm variant:
  - `STANDARD` - Normal processing path
  - `INVERSE` - Reverse-phase processing (higher latency)
  - `LOGARITHMIC` - Compressed processing (lower latency)

### Processing Flow

1. **Traffic Generation** → Cardinal Grammeter generates encabulation requests
2. **API Gateway** → Dingle Arm API Gateway receives and routes requests
3. **Processing** → Panametric Fam Processor handles the core logic:
   - Validates the encabulation parameters
   - Checks Prefabulated Cache (Redis) for cached results
   - Processes the encabulation (simulated work)
   - Stores result in Hydrocoptic Data Store (PostgreSQL)
   - Records metrics in Sinusoidal Database (InfluxDB)
4. **Observability** → Metrics flow to Prometheus for monitoring

### Observability Metrics

The system collects real metrics for each encabulation:

- **Throughput** - Encabulations processed per second
- **Processing Latency** - Time to process each request
- **Success Rate** - Percentage of successful encabulations
- **Cache Hit Rate** - Prefabulation cache effectiveness
- **Error Rate** - Failed encabulations per second

### Interactive Control

Use the **Encabulation Control Console** to:

- **Synchronize Cardinal Grammeters** - Start/stop traffic generation with fine-grained control:
  - **Fluorescent Score Motion** (0-100) - Controls throughput (0.5 to 1000 TPS)
  - **Magneto Reluctance** (0-100) - Injects latency (0-500ms) for testing resilience
  - **Capacitive Diractance** (0-100) - Adjusts payload complexity and metadata
- **Traffic Patterns** - Choose steady, burst, ramp, or sine wave patterns
- **Chaos Engineering** - Inject failures and test system resilience
- **Real-time Visualization** - View system topology with live traffic flow
- **Data Explorer** - Browse processed encabulations in PostgreSQL

This workload demonstrates real distributed system patterns: caching, persistence, async messaging, rate limiting, and observability—all with actual data flowing through the system.

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

### ☁️ Cluster Deployment (GitOps)

**Current Method:** ArgoCD GitOps (automated, Git-driven deployments)

#### Prerequisites

- EKS cluster (or any Kubernetes cluster)
- kubectl configured with cluster access
- ArgoCD installed (see [docs/GITOPS_DEPLOYMENT.md](docs/GITOPS_DEPLOYMENT.md))
- GitHub personal access token (for private repos)

#### Quick Start

```bash
# 1. Ensure your cluster context is active
aws eks update-kubeconfig --region <region> --name <cluster-name>

# 2. Deploy root application (manages all services)
kubectl apply -f argocd/root-app.yaml

# 3. ArgoCD automatically deploys all 13 services
argocd app list

# 4. Access ArgoCD UI
kubectl port-forward svc/argocd-server -n argocd 8080:443
# Open https://localhost:8080
```

**Full Documentation:** [docs/GITOPS_DEPLOYMENT.md](docs/GITOPS_DEPLOYMENT.md)

#### Legacy Helm-First Deployment

The previous Helm-first deployment method is preserved in the `helm-first` branch:

```bash
# Switch to helm-first branch
git checkout helm-first

# Deploy via Helm
./scripts/deploy/deploy-helm.sh
```

### 🌐 Access the Platform

After deployment, access the services:

```bash
# Direct Helm deployment - no additional UI needed

# Control Console (Main UI)
kubectl port-forward -n encabulator-control svc/encabulation-control-console 8080:8080                 # Control Console
# Open http://localhost:8080 in your browser

# Observability Stack (Helm-based service names)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80                    # Grafana (admin/prom-operator)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090     # Prometheus
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-state-metrics 8081:8080       # Kube State Metrics
```

## ✨ Key Features

### Real-Time Visualization
- **Interactive System Topology** - Live visualization of all 13 microservices with animated traffic flow
- **Traffic Particles** - Visual representation of requests flowing through the system (1-15 particles based on actual TPS)
- **Pod Indicators** - See individual pods within each service, with color-coded health status
- **Auto-Scaling Visualization** - Watch services grow and shrink as HPA responds to load

### Traffic Control
- **Dual Mode Operation** - Toggle between simulated (demo) and real traffic modes
- **Fine-Grained Control** - Adjust throughput (0.5-1000 TPS), latency (0-500ms), and payload complexity
- **Traffic Patterns** - Steady, burst, ramp, or sine wave patterns for different test scenarios
- **Real-Time Metrics** - See actual TPS, latency, error rates updated every 2 seconds

### Complete Data Flow
- **End-to-End Visibility** - Traffic flows from Cardinal Grammeter → API Gateway → Processor → Data Services
- **Multi-Database Architecture** - PostgreSQL (persistence), Redis (caching), InfluxDB (metrics)
- **Real Data Persistence** - Actual encabulations stored and queryable via REST API
- **Cache Visualization** - See cache hits/misses in real-time (1.5x TPS for frequent checks)

### Resource Efficiency
- **Optimized for Dev/Demo** - 70% reduction in idle resource usage (1.5 cores vs 4.75 cores)
- **Auto-Scaling** - Services scale from 1 to 10-20 replicas based on actual load
- **Fast Response** - 2-4 second response time for traffic start, metrics update every 2 seconds

### Admin Portal
- **User Management** - Create and manage users with role-based access control (admin, operator, viewer)
- **Grafana Integration** - Embedded dashboards for deep observability
- **Action History** - Audit log of all control actions with timestamps
- **JWT Authentication** - Secure access with token-based auth

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
- **[Image Publishing Guide](docs/IMAGE_PUBLISHING.md)** - Building and publishing container images to ECR
- **[Versioning Strategy](docs/VERSIONING.md)** - Semantic versioning and release management
- **[Makefile Reference](docs/MAKEFILE_REFERENCE.md)** - Complete reference for all Makefile targets

### Architecture

- **[Architecture Summary](ARCHITECTURE_SUMMARY.md)** - High-level architecture overview
- **[Cloud Native Infrastructure](docs/architecture/cloud-native-infrastructure.md)** - Infrastructure patterns
- [API Specifications](docs/api/) _(coming soon)_
- [Cloud Native Encabulator Glossary](docs/glossary.md) _(coming soon)_

## Deployment Evolution Strategy

This project demonstrates the evolution of cloud-native deployment strategies on the `main` branch:

- ✅ **Phase 1 Complete**: **Helm-First Development** - Direct Helm deployment (preserved in `helm-first` branch)
- ✅ **Phase 2 Complete**: **GitOps with ArgoCD** - Automated Git-driven deployments (current on `main`)
- ⏭️ **Phase 3 Planned**: **AWS Managed Services (ACK)** - Migrate to RDS, ElastiCache, etc.
- ⏭️ **Phase 4 Planned**: **KRO Platform Engineering** - Advanced resource composition (preserve GitOps in `argo-first` branch)

**Educational Branches:**

- **`helm-first`**: Preserved Helm-only implementation (v0.2.0-helm-first)
- **`main`**: Current GitOps implementation with ArgoCD
- **`argo-first`**: Will preserve GitOps implementation (created when KRO added to main)

**Current Deployment Method:** GitOps with ArgoCD - See [docs/GITOPS_DEPLOYMENT.md](docs/GITOPS_DEPLOYMENT.md)

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
