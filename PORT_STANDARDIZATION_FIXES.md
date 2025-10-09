# Port Standardization Fixes Applied

## Date: 2025-10-08

All services have been standardized to use port 8080 and proper health endpoint configurations.

---

## Changes Applied

### 1. side-fumbling-collector

**File: `services/support-plane/side-fumbling-collector/app/config.py`**
```python
# Changed from:
port: int = 8000

# Changed to:
port: int = 8080
```

**File: `helm/charts/side-fumbling-collector/values.yaml`**
```yaml
# Service ports updated:
service:
  port: 8080      # was 8000
  targetPort: 8080  # was 8000

# Probes updated:
livenessProbe:
  httpGet:
    path: /health
    port: 8080    # was 8000

readinessProbe:
  httpGet:
    path: /ready  # was /health
    port: 8080    # was 8000

# Environment variables added:
env:
  - name: PORT
    value: "8080"
  - name: IN_CLUSTER
    value: "true"
  - name: NAMESPACE
    value: "encabulator-support"
```

---

### 2. unilateral-phase-detractor

**File: `services/support-plane/unilateral-phase-detractor/app/config.py`**
```python
# Added port configuration:
class Settings(BaseSettings):
    app_name: str = "Unilateral Phase Detractor"  # Fixed from "Cardinal Grammeter"
    port: int = 8080  # Added this line
    secret_key: str = "your-secret-key-change-in-production"
    algorithm: str = "HS256"
    access_token_expire_minutes: int = 30
```

**File: `helm/charts/unilateral-phase-detractor/values.yaml`**
```yaml
# Service ports updated:
service:
  port: 8080      # was 8000
  targetPort: 8080  # was 8000

# Probes updated:
livenessProbe:
  httpGet:
    path: /health
    port: 8080    # was 8000

readinessProbe:
  httpGet:
    path: /ready  # was /health
    port: 8080    # was 8000

# Environment variables added:
env:
  - name: PORT
    value: "8080"
  - name: SECRET_KEY
    valueFrom:
      secretKeyRef:
        name: unilateral-phase-detractor-secrets
        key: secret-key
```

---

## Summary of Changes

### Port Standardization
- ✅ All services now use port 8080
- ✅ Consistent across all planes (data, processing, control, support)

### Health Endpoint Fixes
- ✅ Liveness probes use `/health`
- ✅ Readiness probes use `/ready` (not `/health`)
- ✅ All probes point to correct port (8080)

### Configuration Improvements
- ✅ Added explicit PORT environment variables
- ✅ Fixed app_name in unilateral-phase-detractor config
- ✅ Added namespace configuration for side-fumbling-collector

---

## Verification Checklist

Before deploying, verify:

- [x] Python config files updated with port 8080
- [x] Helm values.yaml files updated with port 8080
- [x] Readiness probes use `/ready` endpoint
- [x] Liveness probes use `/health` endpoint
- [x] PORT environment variables added to Helm charts
- [x] No syntax errors in Python files
- [x] No syntax errors in YAML files

---

## Services Ready for Deployment

All services are now properly configured and ready to deploy:

### Control Plane (4 services)
1. ✅ cardinal-grammeter
2. ✅ demo-scenario-orchestrator
3. ✅ encabulation-control-console
4. ✅ malaise-injection-engine

### Support Plane (3 services)
1. ✅ differential-girdle
2. ✅ side-fumbling-collector (fixed)
3. ✅ unilateral-phase-detractor (fixed)

---

## Next Steps

1. Build and push Docker images for fixed services:
   ```bash
   cd services/support-plane/side-fumbling-collector
   make image-publish IMAGE_TAG=0.0.1
   
   cd ../unilateral-phase-detractor
   make image-publish IMAGE_TAG=0.0.1
   ```

2. Deploy services in recommended order:
   ```bash
   # Support plane
   helm install differential-girdle ./helm/charts/differential-girdle -n encabulator-support --create-namespace
   helm install side-fumbling-collector ./helm/charts/side-fumbling-collector -n encabulator-support
   helm install unilateral-phase-detractor ./helm/charts/unilateral-phase-detractor -n encabulator-support
   
   # Control plane
   helm install cardinal-grammeter ./helm/charts/cardinal-grammeter -n encabulator-control --create-namespace
   helm install malaise-injection-engine ./helm/charts/malaise-injection-engine -n encabulator-control
   helm install demo-scenario-orchestrator ./helm/charts/demo-scenario-orchestrator -n encabulator-control
   helm install encabulation-control-console ./helm/charts/encabulation-control-console -n encabulator-control
   ```

3. Verify deployments:
   ```bash
   kubectl get pods -n encabulator-support
   kubectl get pods -n encabulator-control
   ```

4. Test health endpoints:
   ```bash
   kubectl port-forward -n encabulator-support svc/side-fumbling-collector 8080:8080
   curl http://localhost:8080/health
   curl http://localhost:8080/ready
   ```

---

## Rollback Plan

If issues occur, rollback changes:

```bash
# Revert config files
git checkout services/support-plane/side-fumbling-collector/app/config.py
git checkout services/support-plane/unilateral-phase-detractor/app/config.py
git checkout helm/charts/side-fumbling-collector/values.yaml
git checkout helm/charts/unilateral-phase-detractor/values.yaml
```
