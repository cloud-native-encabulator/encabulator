# Detailed Health Endpoint Audit - Undeployed Services

## Audit Date: 2025-10-08

This document provides a detailed audit of all undeployed services, checking for health endpoint implementation and configuration alignment.

---

## ✅ Control Plane Services - All Verified

### 1. cardinal-grammeter
**Status:** ✅ Ready to deploy

**Implementation:**
- File: `services/control-plane/cardinal-grammeter/src/index.ts`
- Port: 8080 (from PORT env var)
- Health endpoints:
  - `GET /health` - Returns `{ status: 'healthy', timestamp: ISO8601 }`
  - `GET /ready` - Returns `{ status: 'ready' }`
  - `GET /metrics` - Prometheus metrics

**Helm Configuration:**
- File: `helm/charts/cardinal-grammeter/values.yaml`
- Service port: 8080 ✅
- Target port: 8080 ✅
- Liveness probe: `/health` on port 8080 ✅
- Readiness probe: `/ready` on port 8080 ✅
- Metrics: Prometheus annotations configured ✅

**Verdict:** ✅ Perfect alignment - ready to deploy

---

### 2. demo-scenario-orchestrator
**Status:** ✅ Ready to deploy

**Implementation:**
- File: `services/control-plane/demo-scenario-orchestrator/main.py`
- Port: 8080 (from PORT env var)
- Health endpoints:
  - `GET /health` - Returns `{ status: 'healthy', timestamp: float }`
  - `GET /ready` - Returns `{ status: 'ready' }` (checks K8s API)
  - `GET /metrics` - Prometheus metrics

**Helm Configuration:**
- File: `helm/charts/demo-scenario-orchestrator/values.yaml`
- Service port: 8080 ✅
- Target port: 8080 ✅
- Liveness probe: `/health` on port 8080 ✅
- Readiness probe: `/ready` on port 8080 ✅
- RBAC: Configured for K8s API access ✅
- Service account: Created ✅

**Dependencies:**
- cardinal-grammeter (TRAFFIC_ENGINE_URL)
- malaise-injection-engine (MALAISE_ENGINE_URL)

**Verdict:** ✅ Perfect alignment - ready to deploy (after dependencies)

---

### 3. encabulation-control-console
**Status:** ✅ Ready to deploy

**Implementation:**
- File: `services/control-plane/encabulation-control-console/server/src/index.ts`
- Port: 8080 (from PORT env var)
- Health endpoints:
  - `GET /health` - Returns `{ status: 'healthy', timestamp: ISO8601 }`
  - `GET /ready` - Returns `{ status: 'ready' }` (checks K8s API)
  - `GET /metrics` - Prometheus metrics
- WebSocket support: Socket.io enabled

**Helm Configuration:**
- File: `helm/charts/encabulation-control-console/values.yaml`
- Service port: 8080 ✅
- Target port: 8080 ✅
- Liveness probe: `/health` on port 8080 ✅
- Readiness probe: `/ready` on port 8080 ✅
- RBAC: Configured for K8s API access ✅
- Service account: Created ✅
- Ingress: Disabled by default (can be enabled)

**Verdict:** ✅ Perfect alignment - ready to deploy

---

### 4. malaise-injection-engine
**Status:** ✅ Ready to deploy

**Implementation:**
- File: `services/control-plane/malaise-injection-engine/main.go`
- Port: 8080 (from PORT env var)
- Health endpoints:
  - `GET /health` - Returns `{ status: 'healthy', timestamp: RFC3339 }`
  - `GET /ready` - Returns `{ status: 'ready' }` (checks K8s API)
  - `GET /metrics` - Prometheus metrics
- Chaos endpoints:
  - `POST /inject` - Inject failures

**Helm Configuration:**
- File: `helm/charts/malaise-injection-engine/values.yaml`
- Service port: 8080 ✅
- Target port: 8080 ✅
- Liveness probe: `/health` on port 8080 ✅
- Readiness probe: `/ready` on port 8080 ✅
- RBAC: Configured for pod deletion ✅
- Service account: Created ✅

**Verdict:** ✅ Perfect alignment - ready to deploy

---

## ⚠️ Support Plane Services - Issues Found

### 5. differential-girdle
**Status:** ✅ Ready to deploy

**Implementation:**
- File: `services/support-plane/differential-girdle/main.go`
- Routes: `services/support-plane/differential-girdle/internal/api/routes.go`
- Port: 8080 (from PORT env var)
- Health endpoints:
  - `GET /health` - Returns `{ status: 'healthy' }`
  - `GET /ready` - Returns `{ status: 'ready' }`
  - `GET /metrics` - Prometheus metrics

**Helm Configuration:**
- File: `helm/charts/differential-girdle/values.yaml`
- Service port: 8080 ✅
- Target port: 8080 ✅
- Liveness probe: `/health` on port 8080 ✅
- Readiness probe: `/ready` on port 8080 ✅

**Verdict:** ✅ Perfect alignment - ready to deploy

---

### 6. side-fumbling-collector
**Status:** ⚠️ Port mismatch - needs fix

**Implementation:**
- File: `services/support-plane/side-fumbling-collector/app/main.py`
- Config: `services/support-plane/side-fumbling-collector/app/config.py`
- Port: **8000** (hardcoded in config.py)
- Health endpoints:
  - `GET /health` - Returns `HealthCheck` model
  - `GET /ready` - Returns `HealthCheck` model
  - `GET /metrics` - Prometheus metrics

**Helm Configuration:**
- File: `helm/charts/side-fumbling-collector/values.yaml`
- Service port: **8000** ⚠️
- Target port: **8000** ⚠️
- Liveness probe: `/health` on port **8000** ⚠️
- Readiness probe: `/health` on port **8000** ⚠️ (should be `/ready`)

**Issues Found:**
1. ⚠️ Port 8000 instead of standard 8080
2. ⚠️ Readiness probe uses `/health` instead of `/ready`

**Recommendations:**
1. Update `app/config.py` to use port 8080 for consistency
2. Update Helm values to use `/ready` for readiness probe
3. OR accept port 8000 but fix readiness probe path

**Verdict:** ⚠️ Needs configuration updates before deployment

---

### 7. unilateral-phase-detractor
**Status:** ⚠️ Port mismatch and missing config - needs fix

**Implementation:**
- File: `services/support-plane/unilateral-phase-detractor/app/main.py`
- Config: `services/support-plane/unilateral-phase-detractor/app/config.py`
- Port: **Not configured** (no port in config.py, needs PORT env var)
- Health endpoints:
  - `GET /health` - Returns `HealthCheck` model
  - `GET /ready` - Returns `HealthCheck` model
  - `GET /metrics` - Prometheus metrics

**Helm Configuration:**
- File: `helm/charts/unilateral-phase-detractor/values.yaml`
- Service port: **8000** ⚠️
- Target port: **8000** ⚠️
- Liveness probe: `/health` on port **8000** ⚠️
- Readiness probe: `/health` on port **8000** ⚠️ (should be `/ready`)

**Issues Found:**
1. ⚠️ No port configuration in config.py (relies on uvicorn default or env var)
2. ⚠️ Helm chart assumes port 8000
3. ⚠️ Readiness probe uses `/health` instead of `/ready`
4. ⚠️ No PORT env var in Helm values

**Recommendations:**
1. Add port configuration to `app/config.py` with default 8080
2. Update Helm values to use port 8080
3. Add PORT env var to Helm values
4. Update readiness probe to use `/ready`

**Verdict:** ⚠️ Needs configuration updates before deployment

---

## Summary of Issues

### Critical Issues (Must Fix Before Deploy)

1. **side-fumbling-collector**
   - Readiness probe path: `/health` → should be `/ready`
   - Consider standardizing port to 8080

2. **unilateral-phase-detractor**
   - Missing PORT env var in Helm values
   - Readiness probe path: `/health` → should be `/ready`
   - Consider adding port config to config.py
   - Consider standardizing port to 8080

### Non-Critical Issues (Recommendations)

1. **Port Standardization**
   - Most services use 8080
   - side-fumbling-collector and unilateral-phase-detractor use 8000
   - Recommend standardizing all services to 8080 for consistency

---

## Recommended Fixes

### Fix 1: side-fumbling-collector Helm Chart

```yaml
# helm/charts/side-fumbling-collector/values.yaml
readinessProbe:
  httpGet:
    path: /ready  # Changed from /health
    port: 8000
  initialDelaySeconds: 10
  periodSeconds: 5
```

**Optional:** Standardize to port 8080
```yaml
service:
  port: 8080
  targetPort: 8080

livenessProbe:
  httpGet:
    path: /health
    port: 8080

readinessProbe:
  httpGet:
    path: /ready
    port: 8080

env:
  - name: PORT
    value: "8080"
```

And update `app/config.py`:
```python
port: int = 8080  # Changed from 8000
```

---

### Fix 2: unilateral-phase-detractor Configuration

**Add to config.py:**
```python
class Settings(BaseSettings):
    app_name: str = "Unilateral Phase Detractor"
    port: int = 8080  # Add this line
    secret_key: str = "your-secret-key-change-in-production"
    algorithm: str = "HS256"
    access_token_expire_minutes: int = 30
```

**Update Helm values:**
```yaml
# helm/charts/unilateral-phase-detractor/values.yaml
service:
  port: 8080  # Changed from 8000
  targetPort: 8080

livenessProbe:
  httpGet:
    path: /health
    port: 8080  # Changed from 8000

readinessProbe:
  httpGet:
    path: /ready  # Changed from /health
    port: 8080  # Changed from 8000

env:
  - name: PORT
    value: "8080"
  - name: SECRET_KEY
    valueFrom:
      secretKeyRef:
        name: unilateral-phase-detractor-secrets
        key: secret-key
```

**Update main.py to use port from config:**
```python
if __name__ == "__main__":
    uvicorn.run(
        "app.main:app",
        host="0.0.0.0",
        port=settings.port,  # Use port from settings
        log_level="info"
    )
```

---

## Deployment Order Recommendation

Based on dependencies, deploy in this order:

### Phase 1: Independent Services
1. ✅ differential-girdle (no dependencies)
2. ⚠️ side-fumbling-collector (fix readiness probe first)
3. ⚠️ unilateral-phase-detractor (fix port config first)

### Phase 2: Control Plane Foundation
4. ✅ cardinal-grammeter (no dependencies)
5. ✅ malaise-injection-engine (no dependencies)

### Phase 3: Control Plane Orchestration
6. ✅ demo-scenario-orchestrator (depends on cardinal-grammeter, malaise-injection-engine)
7. ✅ encabulation-control-console (no service dependencies, but benefits from full system)

---

## Testing Checklist

For each service after deployment:

- [ ] Pod starts successfully
- [ ] Liveness probe passes (returns 200 OK)
- [ ] Readiness probe passes (returns 200 OK)
- [ ] Service is accessible from other pods
- [ ] Metrics endpoint is scrapable by Prometheus
- [ ] Logs show no errors
- [ ] HPA scales correctly (if enabled)

---

## Next Steps

1. **Fix side-fumbling-collector:**
   - Update readiness probe path to `/ready`
   - Optionally standardize port to 8080

2. **Fix unilateral-phase-detractor:**
   - Add port configuration to config.py
   - Add PORT env var to Helm values
   - Update readiness probe path to `/ready`
   - Update service ports to 8080

3. **Deploy services in recommended order**

4. **Verify all health endpoints**

5. **Test inter-service communication**

6. **Verify Prometheus metrics collection**
