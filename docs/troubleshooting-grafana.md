# Grafana Troubleshooting Guide

This guide helps resolve common Grafana deployment and stability issues in the Cloud Native Encabulator platform.

## Common Grafana Issues

### 1. Grafana Pod Failing to Start

**Symptoms:**
- Grafana pod in `CrashLoopBackOff` or `Pending` state
- Grafana deployment shows 0/1 ready replicas

**Diagnosis:**
```bash
# Check pod status
kubectl get pods -n encabulator-monitoring | grep grafana

# Check pod logs
kubectl logs -n encabulator-monitoring deployment/prometheus-stack-grafana

# Check pod events
kubectl describe pod -n encabulator-monitoring -l app.kubernetes.io/name=grafana
```

**Common Causes & Solutions:**

#### Resource Constraints
```bash
# Check node resources
kubectl top nodes
kubectl describe nodes

# Solution: Increase Grafana resource limits
# Edit helm/values/observability/values-prometheus.yaml:
grafana:
  resources:
    requests:
      cpu: 100m
      memory: 512Mi  # Increased from 256Mi
    limits:
      cpu: 500m      # Increased from 200m
      memory: 1Gi    # Increased from 512Mi
```

#### Persistent Volume Issues
```bash
# Check PVC status
kubectl get pvc -n encabulator-monitoring

# Check storage classes
kubectl get storageclass

# Check PVC events for timeout issues
kubectl describe pvc -n encabulator-monitoring

# Solution 1: Ensure using default storage class (recommended)
# Edit helm/values/observability/values-prometheus.yaml:
grafana:
  persistence:
    enabled: true
    # No explicit storageClassName - uses cluster default
    size: 1Gi

# Solution 2: Disable persistence for development (if PVC issues persist)
grafana:
  persistence:
    enabled: false
```

#### Security Context Issues
```bash
# Check security policies
kubectl get psp  # Pod Security Policies
kubectl get securitycontextconstraints  # OpenShift

# Solution: Configure proper security context
# Edit helm/values/observability/values-prometheus.yaml:
grafana:
  securityContext:
    runAsNonRoot: true
    runAsUser: 472
    fsGroup: 472
```

### 2. Grafana Service Not Accessible

**Symptoms:**
- Port-forward fails or times out
- Grafana UI not loading

**Diagnosis:**
```bash
# Check service
kubectl get svc -n encabulator-monitoring | grep grafana

# Check endpoints
kubectl get endpoints -n encabulator-monitoring prometheus-stack-grafana

# Test connectivity
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80
curl http://localhost:3000/api/health
```

**Solutions:**

#### Service Configuration
```bash
# Verify service is pointing to correct pods
kubectl describe svc prometheus-stack-grafana -n encabulator-monitoring

# Check if pods have correct labels
kubectl get pods -n encabulator-monitoring --show-labels | grep grafana
```

#### Network Policies
```bash
# Check for restrictive network policies
kubectl get networkpolicy -n encabulator-monitoring

# Temporarily disable network policies for testing
kubectl delete networkpolicy --all -n encabulator-monitoring
```

### 3. Grafana Configuration Issues

**Symptoms:**
- Grafana starts but shows configuration errors
- Datasources not working
- Dashboards not loading

**Diagnosis:**
```bash
# Check Grafana configuration
kubectl get configmap -n encabulator-monitoring | grep grafana
kubectl describe configmap prometheus-stack-grafana -n encabulator-monitoring

# Check secrets
kubectl get secrets -n encabulator-monitoring | grep grafana
```

**Solutions:**

#### Datasource Configuration
```bash
# Verify Prometheus datasource
kubectl exec -n encabulator-monitoring deployment/prometheus-stack-grafana -- \
  curl -s http://localhost:3000/api/datasources

# Check Prometheus service
kubectl get svc -n encabulator-monitoring prometheus-stack-kube-prom-prometheus
```

#### Admin Password Issues
```bash
# Reset admin password
kubectl patch secret prometheus-stack-grafana -n encabulator-monitoring \
  -p '{"data":{"admin-password":"'$(echo -n "encabulator-dev-2024" | base64)'"}}'

# Restart Grafana
kubectl rollout restart deployment/prometheus-stack-grafana -n encabulator-monitoring
```

### 4. Grafana Performance Issues

**Symptoms:**
- Slow dashboard loading
- High memory usage
- Frequent restarts

**Solutions:**

#### Resource Optimization
```yaml
# Increase resources in helm/values/observability/values-prometheus.yaml
grafana:
  resources:
    requests:
      cpu: 200m
      memory: 1Gi
    limits:
      cpu: 1000m
      memory: 2Gi
```

#### Database Optimization
```yaml
# Use external database for production
grafana:
  database:
    type: postgres
    host: your-postgres-host
    name: grafana
    user: grafana
    password: your-password
```

## Quick Fixes

### Restart Grafana
```bash
kubectl rollout restart deployment/prometheus-stack-grafana -n encabulator-monitoring
kubectl rollout status deployment/prometheus-stack-grafana -n encabulator-monitoring
```

### Reinstall Grafana
```bash
# Uninstall
helm uninstall prometheus-stack -n encabulator-monitoring

# Reinstall with fixed configuration
helm upgrade --install prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace encabulator-monitoring \
  --values helm/values/observability/values-prometheus.yaml \
  --wait --timeout=900s
```

### Check Grafana Health
```bash
# Port forward
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80 &

# Health check
curl -s http://localhost:3000/api/health | jq

# Kill port forward
pkill -f "kubectl port-forward.*grafana"
```

## Prevention

### 1. Resource Planning
- Always allocate sufficient CPU and memory
- Monitor resource usage with `kubectl top pods`
- Set appropriate resource requests and limits

### 2. Configuration Validation
- Test Helm values with `helm template` before deployment
- Use `--dry-run` to validate configurations
- Keep configuration simple for development environments

### 3. Monitoring
- Set up alerts for Grafana pod restarts
- Monitor Grafana metrics and logs
- Use Grafana's built-in health endpoints

## Getting Help

### Useful Commands
```bash
# Complete Grafana status
kubectl get all -n encabulator-monitoring -l app.kubernetes.io/name=grafana

# Grafana logs (last 100 lines)
kubectl logs -n encabulator-monitoring deployment/prometheus-stack-grafana --tail=100

# Grafana configuration
kubectl get configmap prometheus-stack-grafana -n encabulator-monitoring -o yaml

# Helm release status
helm status prometheus-stack -n encabulator-monitoring
```

### Log Analysis
Look for these common error patterns in Grafana logs:
- `failed to start grafana`: Configuration or permission issues
- `database locked`: SQLite database issues (use external DB)
- `plugin not found`: Missing or misconfigured plugins
- `datasource not found`: Prometheus connectivity issues

### Support Resources
- [Grafana Documentation](https://grafana.com/docs/)
- [kube-prometheus-stack Chart](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
- [Grafana Troubleshooting](https://grafana.com/docs/grafana/latest/troubleshooting/)