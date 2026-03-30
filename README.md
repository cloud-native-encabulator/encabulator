# Cloud Native Encabulator

For a number of years now, work has been proceeding in order to bring perfection to the crudely conceived idea of a cloud-native microservices platform that would not only supply inverse reactive DevOps for use in unilateral phase cultural transformation, but would also be capable of automatically synchronizing cardinal grammeters. Such an instrument is the Cloud Native Encabulator.

The original platform was prelaminated on self-managed clusters forming a substrate of pre-famulated amulite surmounted by a malleable logarithmic casing in such a way that the virtual spurving instances were directly aligned with the panametric fam. The latter consisted simply of six hydrocoptic marzlevane services, so fitted to the ambifacient lunar waneshaft broker that side fumbling was effectively prevented.

The Cloud Native Encabulator has now reached a high level of development, and it's being successfully used in the operation of Kubernetes and related novertrunnions. Moreover, whenever a forescent service request is cross-injected to the system, it may also be employed in conjunction with a drawn reciprocation dingle arm, to reduce sinusoidal repleneration.

### About the Encabulator Meme

The [Turbo Encabulator](https://en.wikipedia.org/wiki/Turbo_encabulator) is a classic engineering parody that originated in the 1940s, featuring elaborate technical jargon describing a completely fictional machine. It has been reimagined numerous times across different industries as a humorous way to poke fun at overly complex technical presentations. This project continues that tradition by applying the Encabulator's absurdist naming conventions to real cloud-native technologies—creating a fully functional Kubernetes microservices platform with nonsensical service names like "Hydrocoptic Data Store" and "Panametric Fam Processor."

While the names are whimsical, the platform is real. It demonstrates the evolution of a cloud-native project through a series of stepwise releases, each building on the last.

## Project Status

The Cloud Native Encabulator evolves through deliberate release milestones, each adding a layer of capability:

| Release | Theme | Status |
|---------|-------|--------|
| **0.1.0** | Initial service scaffolding, Helm charts, versioning, and image publishing | ✅ Complete |
| **0.2.0** | Real-time traffic visualization, resource optimization, UI responsiveness | ✅ Complete |
| **0.3.0** | Cleanup — dependency updates, Dockerfile hardening, CI pipelines, docs consolidation | 🔧 In Progress |
| **Future** | Cloud Native Buildpacks, production hardening, advanced deployment patterns | 📋 Planned |

Current version: **0.2.0** — see [CHANGELOG.md](CHANGELOG.md) for full release history.

## Getting Started

Prerequisites: [Docker](https://www.docker.com/) (or [Finch](https://github.com/runfinch/finch)), [kind](https://kind.sigs.k8s.io/), and `make`.

```bash
git clone <repository-url>
cd cloud-native-encabulator

# Build all 13 services
make build

# Run tests
make test

# Build container images
make image
```

For local cluster deployment, environment setup, and testing details, see the [Development Guide](docs/DEVELOPMENT.md).

## Architecture

The system is a polyglot monorepo with 13 microservices organized into four planes:

| Plane | Services | Languages | Purpose |
|-------|----------|-----------|---------|
| **Data** | Hydrocoptic Data Store, Prefabulated Cache, Sinusoidal Database | Go | Storage, persistence, caching (PostgreSQL, Redis, InfluxDB) |
| **Processing** | Panametric Fam Processor, Logarithmic Casing Service, Dingle Arm API Gateway | Go, TypeScript | Core logic, message routing, API gateway |
| **Support** | Differential Girdle, Unilateral Phase Detractor, Side Fumbling Collector | Go, Python | Feature flags, auth, telemetry |
| **Control** | Encabulation Control Console, Malaise Injection Engine, Cardinal Grammeter, Demo Scenario Orchestrator | Go, TypeScript, Python | Interactive UI, chaos engineering, traffic simulation |

Traffic flows from the Cardinal Grammeter through the API Gateway into the Panametric Fam Processor, which coordinates with data services for persistence, caching, and metrics collection. The Control Console provides a real-time visualization of the entire topology.

## Documentation

- [Development Guide](docs/DEVELOPMENT.md) — environment setup, building, and testing
- [GitOps Deployment](docs/GITOPS_DEPLOYMENT.md) — deploying with ArgoCD
- [Image Publishing](docs/IMAGE_PUBLISHING.md) — building and publishing container images
- [Versioning Strategy](docs/VERSIONING.md) — semantic versioning and release management
- [Makefile Reference](docs/MAKEFILE_REFERENCE.md) — all available make targets
- [Helm Charts](helm/README.md) — chart documentation
- Service plane READMEs: [Data](services/data-plane/README.md) · [Processing](services/processing-plane/README.md) · [Support](services/support-plane/README.md) · [Control](services/control-plane/README.md)

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines, including the DCO sign-off requirement:

```bash
git commit -s -m "Your commit message"
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).
