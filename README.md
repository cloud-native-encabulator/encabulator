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

### Prerequisites

- EKS cluster (or any Kubernetes cluster)
- kubectl configured with cluster access
- AWS CLI configured
- Helm (recommended)

### GitOps Development Setup

```bash
# Clone the repository
git clone <repository-url>
cd cloud-native-encabulator-microservices

# Ensure your EKS cluster context is active
aws eks update-kubeconfig --region <region> --name <cluster-name>

# Verify setup and configure environment
./scripts/setup/eks-setup.sh

# Bootstrap ArgoCD for GitOps deployment
./scripts/deploy/bootstrap-argocd.sh
```

### Access the Demo

Once ArgoCD is bootstrapped, access the GitOps dashboard:

```bash
# ArgoCD UI
kubectl port-forward -n argocd svc/argocd-server 8080:443
# Visit: https://localhost:8080 (admin/<generated-password>)

# Get ArgoCD admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
```

After services are deployed via ArgoCD, access them via port forwarding:

```bash
# Grafana Dashboard (when deployed)
kubectl port-forward -n encabulator-monitoring svc/grafana 3000:3000

# Prometheus Metrics (when deployed)
kubectl port-forward -n encabulator-monitoring svc/prometheus 9090:9090

# Jaeger Tracing (when deployed)
kubectl port-forward -n encabulator-monitoring svc/jaeger 16686:16686
```

### Alternative Local Setup

For local development without EKS, you can manually create a Kubernetes cluster (kind, k3d, minikube) and then:

```bash
# Deploy to any Kubernetes cluster
./scripts/deploy/deploy-eks.sh
```

## Documentation

- [Architecture Documentation](docs/architecture/)
- [Deployment Guides](docs/deployment/)
- [API Specifications](docs/api/)
- [Troubleshooting Guide](docs/troubleshooting/)
- [Cloud Native Encabulator Glossary](docs/glossary.md)

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