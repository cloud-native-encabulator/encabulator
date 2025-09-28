# Cloud Native Encabulator - Testing Guide

This guide provides comprehensive testing strategies for the Cloud Native Encabulator platform, from local development to full EKS deployment.

## 🧪 Testing Approaches

### 1. Local Testing (Recommended First Step)
Test the KRO ResourceGroups and observability stack on your local machine using kind, minikube, or k3d.

### 2. Development Cluster Testing
Deploy to a real EKS cluster for full integration testing with AWS services.

## 🏠 Local Testing

### Prerequisites
- Docker
- kubectl
- One of: kind, minikube, or k3d

### Quick Start
```bash
# Run complete local test suite
./scripts/test/local-test.sh

# Clean up when done
./scripts/test/local-test.sh --cleanup
```

### What Local Testing Covers
- ✅ KRO controller installation and functionality
- ✅ Custom Resource Definition creation
- ✅ ObservabilityStack ResourceGroup deployment
- ✅ Prometheus, Jaeger, Loki, Grafana deployment via KRO
- ✅ Service connectivity and health checks
- ✅ Basic functionality validation

### Local Test Results
After successful local testing, you'll have:
- A local Kubernetes cluster with KRO installed
- All observability ResourceGroups deployed
- A complete observability stack running
- Access to Prometheus, Grafana, and Jaeger UIs

### Access Local Services
```bash
# Prometheus (metrics)
kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-metrics-prometheus 9090:9090

# Grafana (dashboards) - admin/encabulator-dev-2024
kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000

# Jaeger (tracing)
kubectl port-forward -n encabulator-tracing svc/encabulator-observability-tracing-jaeger 16686:16686
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
- ✅ AWS Controllers for Kubernetes (ACK)
- ✅ KRO with full AWS integration
- ✅ Complete observability platform
- ✅ Infrastructure ResourceGroups (Database, Cache, Cluster)
- ✅ Multi-namespace deployment
- ✅ Service mesh readiness

### Development Cluster Results
After successful deployment, you'll have:
- ArgoCD managing all deployments
- ACK controllers for AWS resource management
- KRO orchestrating complex resource compositions
- Complete observability stack with persistent storage
- Infrastructure ResourceGroups ready for service deployment

### Access Development Services
```bash
# ArgoCD UI
kubectl port-forward -n argocd svc/argocd-server 8080:443
# Access: https://localhost:8080 (admin/[get-password-command])

# Prometheus
kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-metrics-prometheus 9090:9090

# Grafana - admin/encabulator-dev-2024
kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000

# Jaeger
kubectl port-forward -n encabulator-tracing svc/encabulator-observability-tracing-jaeger 16686:16686
```

## 🔍 Validation Checklist

### Local Testing Validation
- [ ] KRO controller is running
- [ ] Custom Resource Definitions are created
- [ ] ObservabilityStack resource is deployed
- [ ] Prometheus is collecting metrics
- [ ] Grafana shows connected datasources
- [ ] Jaeger UI is accessible
- [ ] Loki is receiving logs

### Development Cluster Validation
- [ ] ArgoCD applications are synced and healthy
- [ ] ACK controllers are running
- [ ] KRO controller is managing resources
- [ ] All observability components are running
- [ ] Infrastructure ResourceGroups are available
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
kubectl get crd | grep encabulator
kubectl describe resourcegroup observability-stack -n kro-system
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

**ACK Controllers Not Working**
```bash
kubectl logs -n ack-system deployment/ack-rds-controller
kubectl get serviceaccount -n ack-system
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