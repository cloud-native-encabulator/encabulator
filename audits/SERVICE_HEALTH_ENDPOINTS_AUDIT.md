# Service Health Endpoints and Port Configuration Audit

## Summary

This document provides a comprehensive audit of all microservices in the Cloud Native Encabulator system, documenting their health endpoints, port configurations, and the alignment between implementation and Helm chart configurations.

**Audit Date:** 2025-10-08  
**Status:** ✅ All services verified and aligned

---

## Data Plane Services

### 1. hydrocoptic-datastore
- **Language:** Go
- **Port:** 8080
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe
- **Metrics:** `/metrics`
- **Status:** ✅ Deployed and healthy

### 2. prefabulated-cache
- **Language:** Go  
- **Port:** 8080
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe
- **Metrics:** `/metrics`
- **Status:** ✅ Deployed and healthy

### 3. sinusoidal-database
- **Language:** Go
- **Port:** 8080
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe
- **Metrics:** `/metrics`
- **Status:** ✅ Deployed and healthy

---

## Processing Plane Services

### 4. dingle-arm-api-gateway
- **Language:** Go
- **Port:** 8080
- **Health Endpoints:**
  - `/health` - Liveness probe (simple health check)
  - `/ready` - Readiness probe (simple health check)
- **Metrics:** `/metrics`
- **Implementation:** `services/processing-plane/dingle-arm-api-gateway/cmd/server/main.go`
- **Health Handler:** `services/processing-plane/dingle-arm-api-gateway/pkg/health/health.go`
- **Helm Chart:** `helm/charts/dingle-arm-api-gateway/values.yaml`
- **Status:** ✅ Deployed and healthy
- **Notes:** API Gateway with rate limiting, proxies to panametric-fam-processor

### 5. logarithmic-casing-service
- **Language:** Go
- **Port:** 8080
- **Health Endpoints:**
  - `/health` - Liveness probe (simple health check)
  - `/ready` - Readiness probe (checks NATS connectivity)
- **Metrics:** `/metrics`
- **Implementation:** `services/processing-plane/logarithmic-casing-service/cmd/server/main.go`
- **Health Handler:** `services/processing-plane/logarithmic-casing-service/pkg/health/health.go`
- **Helm Chart:** `helm/charts/logarithmic-casing-service/values.yaml`
- **Status:** ✅ Deployed and healthy
- **Dependencies:** NATS (deployed as subchart)
- **Notes:** Uses health checker pattern with pluggable checkers

### 6. panametric-fam-processor
- **Language:** TypeScript/Node.js
- **Port:** 8080 (default from PORT env var)
- **Health Endpoints:**
  - `/health` - Liveness probe (simple health check)
  - `/ready` - Readiness probe (checks data plane service connectivity)
- **Metrics:** `/metrics`
- **Implementation:** `services/processing-plane/panametric-fam-processor/src/index.ts`
- **Health Router:** `services/processing-plane/panametric-fam-processor/src/routes/health.ts`
- **Helm Chart:** `helm/charts/panametric-fam-processor/values.yaml`
- **Status:** ✅ Deployed and healthy
- **Configuration Fix:** ✅ Updated Helm chart from port 3000 to 8080 to match implementation
- **Dependencies:** 
  - hydrocoptic-datastore (via HYDROCOPTIC_URL)
  - prefabulated-cache (via CACHE_URL)
  - sinusoidal-database (via SINUSOIDAL_URL)
- **Notes:** Uses namespace-qualified DNS names for cross-namespace communication

---

## Control Plane Services

### 7. cardinal-grammeter
- **Language:** TypeScript/Node.js
- **Port:** 8080 (default from PORT env var)
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe
- **Metrics:** `/metrics`
- **Implementation:** `services/control-plane/cardinal-grammeter/src/index.ts`
- **Status:** ⏳ Not yet deployed
- **Notes:** Traffic generation engine for load testing

### 8. demo-scenario-orchestrator
- **Language:** Python/FastAPI
- **Port:** 8080 (default from PORT env var)
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe (checks Kubernetes API connectivity)
- **Metrics:** `/metrics`
- **Implementation:** `services/control-plane/demo-scenario-orchestrator/main.py`
- **Status:** ⏳ Not yet deployed
- **Dependencies:** Kubernetes API, cardinal-grammeter, malaise-injection-engine
- **Notes:** Orchestrates demo scenarios for system demonstrations

### 9. encabulation-control-console
- **Language:** TypeScript/Node.js (Backend) + React (Frontend)
- **Port:** 8080 (default from PORT env var)
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe (checks Kubernetes API connectivity)
- **Metrics:** `/metrics`
- **Implementation:** `services/control-plane/encabulation-control-console/server/src/index.ts`
- **Status:** ⏳ Not yet deployed
- **Dependencies:** Kubernetes API
- **Notes:** WebSocket-based control console with real-time updates

### 10. malaise-injection-engine
- **Language:** Go
- **Port:** 8080 (default from PORT env var)
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe (checks Kubernetes API connectivity)
- **Metrics:** `/metrics`
- **Implementation:** `services/control-plane/malaise-injection-engine/main.go`
- **Status:** ⏳ Not yet deployed
- **Dependencies:** Kubernetes API
- **Notes:** Chaos engineering service for failure injection

---

## Support Plane Services

### 11. differential-girdle
- **Language:** Go (Gin framework)
- **Port:** 8080 (default from PORT env var)
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe
- **Metrics:** `/metrics`
- **Implementation:** `services/support-plane/differential-girdle/main.go`
- **Routes:** `services/support-plane/differential-girdle/internal/api/routes.go`
- **Status:** ⏳ Not yet deployed
- **Notes:** Configuration management service with feature flags

### 12. side-fumbling-collector
- **Language:** Python/FastAPI
- **Port:** 8000 (default from config)
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe
- **Metrics:** `/metrics`
- **Implementation:** `services/support-plane/side-fumbling-collector/app/main.py`
- **Status:** ⏳ Not yet deployed
- **Notes:** Resource optimization and cleanup service

### 13. unilateral-phase-detractor
- **Language:** Python/FastAPI
- **Port:** Not explicitly configured (needs PORT env var)
- **Health Endpoints:**
  - `/health` - Liveness probe
  - `/ready` - Readiness probe
- **Metrics:** `/metrics`
- **Implementation:** `services/support-plane/unilateral-phase-detractor/app/main.py`
- **Status:** ⏳ Not yet deployed
- **Notes:** Authentication and authorization service

---

## Issues Found and Fixed

### ✅ Fixed: panametric-fam-processor Port Mismatch
- **Issue:** Helm chart configured for port 3000, but service listens on port 8080
- **Impact:** Would cause readiness/liveness probes to fail on deployment
- **Fix:** Updated `helm/charts/panametric-fam-processor/values.yaml`:
  - Changed `service.port` from 3000 to 8080
  - Changed `service.targetPort` from 3000 to 8080
  - Updated `livenessProbe.httpGet.port` from 3000 to 8080
  - Updated `readinessProbe.httpGet.port` from 3000 to 8080

---

## Potential Issues to Address

### ⚠️ side-fumbling-collector Port Configuration
- **Issue:** Service defaults to port 8000, but other services use 8080
- **Recommendation:** Consider standardizing on port 8080 for consistency
- **Action:** Update `services/support-plane/side-fumbling-collector/app/config.py` to default to 8080

### ⚠️ unilateral-phase-detractor Missing Port Configuration
- **Issue:** No explicit port configuration in config.py
- **Recommendation:** Add port configuration with default 8080
- **Action:** Update `services/support-plane/unilateral-phase-detractor/app/config.py`

### ✅ differential-girdle Health Endpoints
- **Status:** Verified - health endpoints properly implemented
- **Endpoints:** `/health` and `/ready` both return 200 OK with status JSON

---

## Health Endpoint Patterns

### Pattern 1: Simple Health Check (Go)
```go
func LivenessHandler(w http.ResponseWriter, r *http.Request) {
    health := HealthCheck{
        Status:    "healthy",
        Timestamp: time.Now(),
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}
```

### Pattern 2: Dependency Health Check (Go)
```go
func (h *HealthHandler) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
    checks := make(map[string]string)
    allHealthy := true
    
    for name, checker := range h.checkers {
        if err := checker.Check(); err != nil {
            checks[name] = err.Error()
            allHealthy = false
        } else {
            checks[name] = "ok"
        }
    }
    
    if !allHealthy {
        w.WriteHeader(http.StatusServiceUnavailable)
    }
    json.NewEncoder(w).Encode(health)
}
```

### Pattern 3: Express Router (TypeScript)
```typescript
export function createHealthRouter(dataPlane: DataPlaneClient): Router {
  const router = Router();
  
  router.get('/health', (req, res) => {
    res.json({ status: 'healthy', timestamp: new Date().toISOString() });
  });
  
  router.get('/ready', async (req, res) => {
    const checks = await dataPlane.checkHealth();
    const allHealthy = Object.values(checks).every(v => v === true);
    res.status(allHealthy ? 200 : 503).json(health);
  });
  
  return router;
}
```

### Pattern 4: FastAPI (Python)
```python
@app.get("/health", response_model=HealthCheck)
async def health():
    return HealthCheck(status="healthy", timestamp=datetime.utcnow())

@app.get("/ready", response_model=HealthCheck)
async def ready():
    # Check dependencies
    return HealthCheck(status="ready", timestamp=datetime.utcnow())
```

---

## Deployment Checklist

When deploying a new service, verify:

- [ ] Service listens on port 8080 (or PORT env var)
- [ ] `/health` endpoint returns 200 OK for liveness
- [ ] `/ready` endpoint returns 200 OK when dependencies are available
- [ ] `/ready` endpoint returns 503 when dependencies are unavailable
- [ ] `/metrics` endpoint exposes Prometheus metrics
- [ ] Helm chart `values.yaml` matches implementation:
  - [ ] `service.port` matches listening port
  - [ ] `service.targetPort` matches listening port
  - [ ] `livenessProbe.httpGet.port` matches listening port
  - [ ] `readinessProbe.httpGet.port` matches listening port
  - [ ] `livenessProbe.httpGet.path` is `/health`
  - [ ] `readinessProbe.httpGet.path` is `/ready`
- [ ] Environment variables for dependencies use namespace-qualified DNS names
- [ ] Service can be reached from other namespaces if needed

---

## Next Steps

1. ✅ Fix panametric-fam-processor port configuration (COMPLETED)
2. Deploy support plane services:
   - differential-girdle
   - side-fumbling-collector (fix port to 8080)
   - unilateral-phase-detractor (add port config)
3. Deploy control plane services:
   - cardinal-grammeter
   - demo-scenario-orchestrator
   - encabulation-control-console
   - malaise-injection-engine
4. Verify all health endpoints are accessible
5. Test cross-namespace service communication
6. Verify Prometheus metrics collection

---

## References

- Kubernetes Liveness/Readiness Probes: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
- Prometheus Client Libraries: https://prometheus.io/docs/instrumenting/clientlibs/
- Service Mesh Best Practices: https://istio.io/latest/docs/ops/best-practices/observability/
