# Applications ArgoCD Applications

This directory contains ArgoCD Applications that manage the Cloud Native Encabulator microservices. These applications represent the core business logic organized into four service planes.

## Service Planes Architecture

The Cloud Native Encabulator follows a multi-plane architecture:

```
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│   Data Plane    │  │ Processing Plane│  │  Support Plane  │  │  Control Plane  │
├─────────────────┤  ├─────────────────┤  ├─────────────────┤  ├─────────────────┤
│• Hydrocoptic    │  │• Panametric Fam │  │• Cardinal       │  │• Encabulation   │
│  Data Store     │  │  Processor      │  │  Grammeter      │  │  Control Console│
│• Sinusoidal     │  │• Logarithmic    │  │• Differential   │  │• Malaise        │
│  Database       │  │  Casing Service │  │  Girdle         │  │  Injection      │
│• Prefabulated   │  │• Dingle Arm     │  │• Side Fumbling  │  │• Unilateral     │
│  Cache          │  │  API Gateway    │  │  Collector      │  │  Phase Detractor│
└─────────────────┘  └─────────────────┘  └─────────────────┘  └─────────────────┘
```

## Components

### `data-plane.yaml`
**Purpose**: Data storage and persistence services  
**Services**:
- **Hydrocoptic Data Store**: Primary PostgreSQL database wrapper
- **Sinusoidal Database**: Time-series metrics storage
- **Prefabulated Cache**: Redis caching service

**Dependencies**: AWS RDS and ElastiCache resources (from infrastructure)

**Source**: Git repository `k8s/applications/data-plane/`

**Target Namespace**: `encabulator-data`

### `processing-plane.yaml`
**Purpose**: Core business logic and API services  
**Services**:
- **Panametric Fam Processor**: Main business logic engine
- **Logarithmic Casing Service**: Message broker and event bus
- **Dingle Arm API Gateway**: External API facade and routing

**Dependencies**: Data plane services

**Source**: Git repository `k8s/applications/processing-plane/`

**Target Namespace**: `encabulator-system`

### `support-plane.yaml`
**Purpose**: Cross-cutting concerns and system services  
**Services**:
- **Cardinal Grammeter**: Authentication and authorization
- **Differential Girdle**: Configuration management
- **Side Fumbling Collector**: Resource optimization and cleanup

**Dependencies**: Data and processing plane services

**Source**: Git repository `k8s/applications/support-plane/`

**Target Namespace**: `encabulator-system`

### `control-plane.yaml`
**Purpose**: System control, monitoring, and demonstration  
**Services**:
- **Encabulation Control Console**: Web interface for system control
- **Malaise Injection Engine**: Chaos engineering and failure injection
- **Unilateral Phase Detractor Engine**: Advanced traffic simulation
- **Demo Scenario Orchestrator**: Pre-built demonstration workflows

**Dependencies**: All other planes

**Source**: Git repository `k8s/applications/control-plane/`

**Target Namespace**: `encabulator-control`

## Dependencies

Applications deploy in dependency order:

1. **Infrastructure** (ACK, AWS resources) ← Must be deployed first
2. **Platform** (monitoring, networking, namespaces) ← Must be deployed second
3. **Data Plane** ← First application layer
4. **Processing Plane** ← Depends on data plane
5. **Support Plane** ← Depends on data and processing planes
6. **Control Plane** ← Depends on all other planes

## Technology Stack

### Data Plane
- **Languages**: Go (wrappers), SQL (PostgreSQL)
- **Databases**: PostgreSQL (RDS), Redis (ElastiCache), Time-series DB
- **Patterns**: Repository pattern, connection pooling

### Processing Plane
- **Languages**: Node.js (Express), Go (message broker)
- **Patterns**: Event-driven architecture, API gateway pattern
- **Communication**: REST APIs, message queues

### Support Plane
- **Languages**: Python (FastAPI), Go (operators)
- **Patterns**: Microservices, Kubernetes operators
- **Integration**: RBAC, configuration management

### Control Plane
- **Languages**: React/TypeScript (frontend), Go (backend)
- **Patterns**: Real-time WebSocket, chaos engineering
- **Features**: Interactive UI, traffic simulation

## Configuration

### Prerequisites

1. **Infrastructure and platform deployed**: All dependencies must be running
2. **Repository configured**: Run `./scripts/setup/configure-repository.sh`
3. **AWS resources available**: Database and cache instances ready

### Environment Configuration

Applications support environment-specific configuration:

```
k8s/applications/
├── base/              # Base configurations
├── overlays/
│   ├── development/   # Dev settings (single replicas, minimal resources)
│   ├── staging/       # Staging settings (multi-replica, production-like)
│   └── production/    # Production settings (HA, scaling, monitoring)
```

## Deployment

### Via GitOps (Recommended)
```bash
# Deploy complete system including applications
./scripts/platform/bootstrap-gitops.sh
```

### Manual Deployment
```bash
# Deploy all applications (after infrastructure and platform)
kubectl apply -k argocd/applications/

# Monitor deployment
kubectl get applications -n argocd -l app.kubernetes.io/component=applications
```

### Individual Service Planes
```bash
# Deploy in dependency order
kubectl apply -f argocd/applications/data-plane.yaml
kubectl apply -f argocd/applications/processing-plane.yaml
kubectl apply -f argocd/applications/support-plane.yaml
kubectl apply -f argocd/applications/control-plane.yaml
```

## Monitoring

### Check Application Status
```bash
# List all application ArgoCD apps
kubectl get applications -n argocd -l app.kubernetes.io/component=applications

# Check specific plane
kubectl describe application data-plane -n argocd
```

### Check Service Health
```bash
# Data plane services
kubectl get pods -n encabulator-data
kubectl get services -n encabulator-data

# Processing plane services
kubectl get pods -n encabulator-system
kubectl get services -n encabulator-system

# Control plane services
kubectl get pods -n encabulator-control
kubectl get services -n encabulator-control
```

### Application Metrics
```bash
# Check service endpoints
kubectl get endpoints --all-namespaces

# View service logs
kubectl logs -n encabulator-system deployment/panametric-fam-processor

# Check service metrics (if Prometheus is deployed)
kubectl port-forward -n encabulator-monitoring svc/prometheus 9090:9090
```

## Access Applications

### Control Console (Main UI)
```bash
# Access the Encabulation Control Console
kubectl port-forward -n encabulator-control svc/encabulation-control-console 8080:80
# Visit: http://localhost:8080
```

### API Gateway
```bash
# Access the Dingle Arm API Gateway
kubectl port-forward -n encabulator-system svc/dingle-arm-api-gateway 8081:80
# API Base URL: http://localhost:8081/api/v1
```

### Individual Services
```bash
# Direct access to services (for debugging)
kubectl port-forward -n encabulator-data svc/hydrocoptic-datastore 5432:5432
kubectl port-forward -n encabulator-data svc/prefabulated-cache 6379:6379
```

## Demo Scenarios

The Control Plane provides built-in demonstration scenarios:

### Traffic Simulation
- **Unilateral Phase Detractor Engine**: Generate various traffic patterns
- **Magneto Reluctance**: Control request frequency and distribution
- **Capacitive Diractants**: Simulate different client behaviors

### Chaos Engineering
- **Malaise Injection Engine**: Inject failures and test resilience
- **Service outages**: Test recovery mechanisms
- **Network partitions**: Validate partition tolerance

### System Demonstrations
- **Startup sequence**: Visualize system initialization
- **Scaling events**: Demonstrate horizontal pod autoscaling
- **Configuration changes**: Show dynamic configuration updates

## Troubleshooting

### Common Issues

1. **Services not starting**:
   - Check dependencies are deployed and healthy
   - Verify resource limits and cluster capacity
   - Check image pull issues and registry access

2. **Database connection failures**:
   - Verify AWS RDS instance is available
   - Check security groups and network connectivity
   - Validate credentials and connection strings

3. **Service discovery issues**:
   - Check Kubernetes DNS resolution
   - Verify service and endpoint configurations
   - Test inter-service connectivity

### Debugging Commands
```bash
# Check application deployment status
kubectl get applications -n argocd
kubectl describe application <app-name> -n argocd

# Check pod status and logs
kubectl get pods --all-namespaces
kubectl logs -n <namespace> <pod-name>

# Test service connectivity
kubectl exec -it <pod-name> -n <namespace> -- curl <service-url>

# Check resource usage
kubectl top pods --all-namespaces
kubectl describe node <node-name>
```

### Health Checks
```bash
# Run comprehensive validation
./scripts/infrastructure/validate-ack-setup.sh

# Check service health endpoints
curl http://localhost:8080/health  # (via port-forward)
```

## Scaling and Performance

### Horizontal Pod Autoscaling
- **Processing Plane**: Scales based on CPU and request rate
- **Data Plane**: Cache services scale with memory usage
- **Control Plane**: UI scales with concurrent users

### Resource Management
- **Requests and Limits**: Defined for all services
- **Quality of Service**: Guaranteed for critical services
- **Node Affinity**: Optimized placement for performance

### Performance Testing
- **Traffic Simulation**: Use Unilateral Phase Detractor Engine
- **Load Testing**: Built-in scenarios for various load patterns
- **Monitoring**: Real-time metrics and alerting

## Security

### Authentication and Authorization
- **Cardinal Grammeter**: Centralized auth service
- **RBAC Integration**: Kubernetes role-based access
- **JWT Tokens**: Secure service-to-service communication

### Network Security
- **Network Policies**: Restrict inter-service communication
- **TLS Encryption**: All external and internal communications
- **Service Mesh**: Optional mTLS for enhanced security

### Secrets Management
- **Kubernetes Secrets**: Database credentials, API keys
- **External Secrets**: Integration with AWS Secrets Manager
- **Rotation**: Automated secret rotation capabilities

## Next Steps

After application deployment:

1. **Access the Control Console** to explore the system
2. **Run demo scenarios** to see the system in action
3. **Monitor metrics and logs** through the observability stack
4. **Experiment with scaling** and resilience features
5. **Customize configuration** for your specific needs