# Metrics Accuracy Fix - TODO

**Issue**: System shows simulated traffic metrics even in "real" mode. Metrics don't reflect actual load.

## Problems:
1. `TrafficSimulationService.getTrafficMetrics()` always generates fake metrics
2. Health check probes create "noise" in metrics
3. Traffic mode toggle doesn't affect metrics display
4. No integration with real Prometheus metrics

## Solution:

### 1. Respect Traffic Mode
```typescript
// In traffic.ts
public getTrafficMetrics(): TrafficMetrics[] {
  if (this.trafficMode === 'real') {
    return this.getRealMetrics()  // Fetch from Prometheus or return empty
  }
  return this.getSimulatedMetrics()
}
```

### 2. Filter Health Checks
- Exclude `/health`, `/ready`, `/metrics` endpoints from traffic metrics
- Only count actual business traffic (e.g., `/api/v1/encabulations`)

### 3. Real Metrics Options:

**Option A: Return Empty (Simplest)**
- When in real mode, return `[]` for traffic metrics
- Shows idle system when no load
- Particles only appear when Cardinal Grammeter is active

**Option B: Query Prometheus (Better)**
- Fetch real request rates from Prometheus
- Query: `rate(http_requests_total[1m])`
- Show actual latency, error rates
- Requires Prometheus query endpoint

**Option C: Service Mesh Metrics (Best)**
- Use Istio/Linkerd metrics if available
- Real service-to-service traffic
- Automatic without instrumentation

## Recommended Approach:

**Phase 1 (Quick Fix):**
- Check `trafficMode` in `getTrafficMetrics()`
- Return `[]` when mode is 'real' and no active traffic
- Shows idle system correctly

**Phase 2 (Real Metrics):**
- Add Prometheus query endpoint
- Fetch real metrics when available
- Fall back to empty if no data

## Files to Modify:
- `services/control-plane/encabulation-control-console/server/src/services/traffic.ts`
- `services/control-plane/encabulation-control-console/server/src/index.ts`
- Potentially add Prometheus client

## Expected Behavior:
- **Simulated Mode**: Always show animated traffic (demo mode)
- **Real Mode + No Load**: Empty/idle system
- **Real Mode + Active Load**: Show actual traffic from Cardinal Grammeter

## Testing:
1. Switch to real mode
2. Verify no particles when idle
3. Start Cardinal Grammeter
4. Verify particles appear for actual traffic
5. Check metrics match actual TPS
