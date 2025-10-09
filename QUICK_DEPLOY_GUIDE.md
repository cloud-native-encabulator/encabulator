# Quick Deploy Guide - Cloud Native Encabulator

## 🚀 Fast Track Deployment

### Prerequisites
```bash
# Create namespaces
kubectl create namespace encabulator-control
kubectl create namespace encabulator-support

# Create secret for auth service
kubectl create secret generic unilateral-phase-detractor-secrets \
  -n encabulator-support \
  --from-literal=secret-key="$(openssl rand -base64 32)"
```

### Build Updated Images (if needed)
```bash
cd services/support-plane/side-fumbling-collector && make image-publish IMAGE_TAG=0.0.1
cd ../unilateral-phase-detractor && make image-publish IMAGE_TAG=0.0.1
cd ../../..
```

### Deploy All Services (One Command)
```bash
# Support Plane
helm install differential-girdle ./helm/charts/differential-girdle -n encabulator-support && \
helm install side-fumbling-collector ./helm/charts/side-fumbling-collector -n encabulator-support && \
helm install unilateral-phase-detractor ./helm/charts/unilateral-phase-detractor -n encabulator-support && \

# Control Plane
helm install cardinal-grammeter ./helm/charts/cardinal-grammeter -n encabulator-control && \
helm install malaise-injection-engine ./helm/charts/malaise-injection-engine -n encabulator-control && \
sleep 10 && \
helm install demo-scenario-orchestrator ./helm/charts/demo-scenario-orchestrator -n encabulator-control && \
helm install encabulation-control-console ./helm/charts/encabulation-control-console -n encabulator-control
```

### Quick Status Check
```bash
watch -n 2 'kubectl get pods -n encabulator-support && echo && kubectl get pods -n encabulator-control'
```

### Quick Health Check
```bash
# Check all services are healthy
for ns in encabulator-support encabulator-control; do
  echo "=== $ns ==="
  kubectl get pods -n $ns -o wide
done
```

---

## 📊 Service Overview

| Service | Namespace | Port | Type |
|---------|-----------|------|------|
| differential-girdle | encabulator-support | 8080 | Config Management |
| side-fumbling-collector | encabulator-support | 8080 | Resource Optimization |
| unilateral-phase-detractor | encabulator-support | 8080 | Authentication |
| cardinal-grammeter | encabulator-control | 8080 | Traffic Generation |
| malaise-injection-engine | encabulator-control | 8080 | Chaos Engineering |
| demo-scenario-orchestrator | encabulator-control | 8080 | Demo Orchestration |
| encabulation-control-console | encabulator-control | 8080 | Control Console |

---

## ✅ Verification Commands

```bash
# All pods running?
kubectl get pods -A | grep encabulator

# All services created?
kubectl get svc -n encabulator-support
kubectl get svc -n encabulator-control

# Check health (example)
kubectl port-forward -n encabulator-control svc/cardinal-grammeter 8080:8080
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

---

## 🔧 Troubleshooting

```bash
# Pod not starting?
kubectl describe pod <pod-name> -n <namespace>
kubectl logs <pod-name> -n <namespace>

# Readiness failing?
kubectl exec -it <pod-name> -n <namespace> -- curl localhost:8080/ready

# Delete and redeploy
helm uninstall <service-name> -n <namespace>
helm install <service-name> ./helm/charts/<service-name> -n <namespace>
```

---

## 📝 What Changed

- ✅ Fixed panametric-fam-processor port (3000 → 8080)
- ✅ Fixed side-fumbling-collector port (8000 → 8080)
- ✅ Fixed unilateral-phase-detractor port (8000 → 8080)
- ✅ Fixed readiness probes to use `/ready` instead of `/health`
- ✅ All services now standardized on port 8080

---

## 🎯 Success Criteria

- [ ] 7 new pods running
- [ ] All health checks passing
- [ ] No errors in logs
- [ ] Prometheus scraping metrics
