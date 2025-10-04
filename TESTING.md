# Cloud Native Encabulator - Testing Guide

This guide provides comprehensive testing strategies for the Cloud Native Encabulator platform, from local development to full EKS deployment.

## 🧪 Testing Approaches

### 1. Comprehensive Testing (Recommended)
Test all platform components together - infrastructure, observability, and integration.

### 2. Infrastructure Testing
Test in-cluster services and infrastructure components.

### 3. Observability Testing
Test the Prometheus-based observability stack on your local machine using kind, minikube, or k3d.

### 4. Development Cluster Testing
Deploy to a real EKS cluster for full integration testing with AWS services.

## 🎯 Comprehensive Testing (Recommended)

### Prerequisites
- Docker or Finch
- kubectl
- One of: kind, minikube, k3d, or EKS cluster

### Quick Start
```bash
# Run complete platform test suite
./scripts/test/comprehensive-test.sh

# Run quick test (infrastructure + observability)
./scripts/test/comprehensive-test.sh --quick

# Clean up when done (if using local cluster)
./scripts/test/local-test.sh --cleanup
```

### What Comprehensive Testing Covers
- ✅ **Phase 1**: Infrastructure foundation (Helm repositories, charts)
- ✅ **Phase 2**: Service deployment (PostgreSQL, Redis via Helm)
- ✅ **Phase 3**: Observability platform (Prometheus, metrics, dashboards)
- ✅ **Phase 4**: Service connectivity and health checks
- ✅ **Phase 5**: End-to-end integration validation

## 🏗️ Infrastructure Testing

### Prerequisites
- kubectl
- Kubernetes cluster access

### Quick Start
```bash
# Test observability components only
./scripts/test/local-test.sh

# Test complete platform
./scripts/test/comprehensive-test.sh
```

### What Infrastructure Testing Covers
- ✅ Helm chart repositories and accessibility
- ✅ PostgreSQL, Redis, and InfluxDB deployment via community charts
- ✅ In-cluster database and cache services
- ✅ Resource management and dependency handling
- ✅ ArgoCD integration for GitOps deployment

## 🏠 Observability Testing

### Prerequisites
- Docker or Finch
- kubectl
- One of: kind, minikube, or k3d

### Quick Start
```bash
# Run observability-focused test suite
./scripts/test/local-test.sh

# Clean up when done
./scripts/test/local-test.sh --cleanup
```

### What Observability Testing Covers
- ✅ Prometheus installation and functionality
- ✅ Observability components (Prometheus, metrics collection)
- ✅ Monitoring stack deployment
- ✅ Prometheus deployment with Kubernetes service discovery
- ✅ Service connectivity and health checks
- ✅ Basic functionality validation

### Comprehensive Test Results
After successful comprehensive testing, you'll have:
- **Infrastructure Layer**: Helm charts, in-cluster services, infrastructure management
- **Database Services**: PostgreSQL, Redis, and InfluxDB via community Helm charts
- **Resource Instances**: Development cluster with database and cache components
- **Observability Stack**: Complete monitoring, tracing, and logging via KRO
- **Integration**: All components working together with proper monitoring

### Infrastructure Test Results
After successful infrastructure testing, you'll have:
- Helm charts for in-cluster service management
- Database services deployed via official Helm charts
- Infrastructure components with proper dependencies
- Example infrastructure instances (development cluster)
- ArgoCD applications for GitOps management

### Observability Test Results
After successful observability testing, you'll have:
- A local Kubernetes cluster with Prometheus installed
- Observability components deployed
- A complete monitoring stack running
- Access to Prometheus UI

### Access Services
```bash
# Prometheus (metrics)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090

# Grafana (dashboards) - admin/encabulator-dev-2024
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80

# AlertManager
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-alertmanager 9093:9093
```

## ☁️ Development Cluster Testing

### Prerequisites
- AWS CLI configured
- kubectl
- helm
- EKS cluster access

### Setup EKS Environment
```bash
# Configure EKS access
aws eks update-kubeconfig --region us-west-2 --name your-cluster-name

# Verify and setup development environment
./scripts/setup/eks-setup.sh
```

### Deploy to Development Cluster
```bash
# Full platform deployment
./scripts/deploy/deploy-dev.sh

# Check deployment status
./scripts/deploy/status.sh
```

### What Development Testing Covers
- ✅ ArgoCD GitOps deployment
- ✅ In-cluster database services (PostgreSQL, Redis)
- ✅ Official Helm chart integration
- ✅ Complete observability platform
- ✅ Infrastructure resources (Database, Cache)
- ✅ Multi-namespace deployment
- ✅ Service mesh readiness

### Development Cluster Results
After successful deployment, you'll have:
- ArgoCD managing all deployments
- Helm charts for in-cluster service management
- PostgreSQL and Redis lifecycle management
- Complete observability stack with persistent storage
- Infrastructure resources ready for service deployment

### Access Development Services
```bash
# ArgoCD UI
kubectl port-forward -n argocd svc/argocd-server 8080:443
# Access: https://localhost:8080 (admin/[get-password-command])

# Prometheus
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090

# Grafana - admin/encabulator-dev-2024
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80

# AlertManager
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-alertmanager 9093:9093
```

## 🔍 Validation Checklist

### Comprehensive Testing Validation
- [ ] **Infrastructure Foundation**: ACK and KRO controllers running
- [ ] **Custom Resources**: All infrastructure and observability CRDs created
- [ ] **Resource Orchestration**: Composite resources creating dependencies
- [ ] **Observability Platform**: Complete monitoring stack deployed
- [ ] **Service Connectivity**: All services healthy and accessible
- [ ] **Integration**: Infrastructure and observability working together

### Infrastructure Testing Validation
- [ ] ACK controllers are running (RDS, ElastiCache)
- [ ] ACK controllers are managing AWS resources
- [ ] Infrastructure CRDs are available (DBInstance, ReplicationGroup)
- [ ] Resource creation is working (Database + Cache)
- [ ] ArgoCD applications are managing infrastructure
- [ ] Example instances are deployed and healthy

### Observability Testing Validation
- [ ] KRO controller is running
- [ ] Observability CRDs are created (Metrics, Tracing, Logs, Stack)
- [ ] ObservabilityStack resource is deployed
- [ ] Prometheus is collecting metrics
- [ ] Grafana shows connected datasources
- [ ] Jaeger UI is accessible
- [ ] Loki is receiving logs

### Development Cluster Validation
- [ ] ArgoCD applications are synced and healthy
- [ ] ACK controllers are running
- [ ] Prometheus is collecting metrics
- [ ] All observability components are running
- [ ] Infrastructure resources are available
- [ ] AWS resources can be created via ACK
- [ ] GitOps workflow is functional

## 🐛 Troubleshooting

### Common Local Issues

**KRO Controller Not Starting**
```bash
kubectl logs -n kro-system deployment/kro-controller
kubectl describe deployment kro-controller -n kro-system
```

**Custom Resources Not Available**
```bash
kubectl get crd | grep aws
kubectl get dbinstance -n encabulator-data
kubectl get replicationgroup -n encabulator-data
```

**Services Not Accessible**
```bash
kubectl get all -n encabulator-monitoring
kubectl describe pod -n encabulator-monitoring
```

### Common Development Cluster Issues

**ArgoCD Applications Not Syncing**
```bash
kubectl get applications -n argocd
kubectl describe application observability-stack -n argocd
```



**Resource Creation Failing**
```bash
kubectl get events --sort-by=.metadata.creationTimestamp
kubectl logs -n kro-system deployment/kro-controller
```

## 📊 Monitoring Your Tests

### Key Metrics to Watch
- **Resource Creation Time**: How long KRO takes to create resources
- **Service Startup Time**: How quickly services become ready
- **Memory Usage**: Resource consumption of the platform
- **Error Rates**: Failed resource creations or service errors

### Useful Commands
```bash
# Watch resource creation
kubectl get observabilitystack -A -w

# Monitor pod status
kubectl get pods -A -l app.kubernetes.io/managed-by=kro

# Check resource usage
kubectl top nodes
kubectl top pods -A

# View recent events
kubectl get events --sort-by=.metadata.creationTimestamp -A
```

## 🚀 Next Steps After Testing

### If Local Testing Succeeds
1. Move to development cluster testing
2. Validate AWS integration
3. Test GitOps workflows
4. Begin service plane development

### If Development Testing Succeeds
1. Create production-ready configurations
2. Implement CI/CD pipelines
3. Add monitoring and alerting
4. Begin building actual microservices

### Performance Benchmarks
- **Local Cluster**: ObservabilityStack should deploy in < 5 minutes
- **Development Cluster**: Full platform should deploy in < 10 minutes
- **Resource Usage**: Platform should use < 2GB RAM on local cluster
- **Service Response**: All health endpoints should respond in < 1 second

## 🔄 Continuous Testing

### Automated Testing
Consider setting up automated tests that:
- Deploy the platform to a fresh cluster
- Validate all components are healthy
- Run basic functionality tests
- Clean up resources

### Integration with CI/CD
```bash
# Example CI pipeline step
./scripts/test/local-test.sh
if [ $? -eq 0 ]; then
    echo "Local tests passed, proceeding to deployment"
    ./scripts/deploy/deploy-dev.sh
fi
```

## 📝 Test Results Documentation

Keep track of your testing results:
- Platform deployment times
- Resource usage patterns
- Common failure modes
- Performance characteristics
- User experience feedback

This information will be valuable for optimizing the platform and troubleshooting production issues.

---

**Ready to test?** Start with local testing to validate the core platform, then move to development cluster testing for full AWS integration!