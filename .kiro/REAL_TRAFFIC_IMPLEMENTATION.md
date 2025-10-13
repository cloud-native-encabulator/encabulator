# Real Traffic Implementation Plan

**Goal**: Enable both simulated (demo) and real traffic modes in the Cloud Native Encabulator

**Status**: 🚧 In Progress  
**Started**: 2025-10-13  
**Token Budget**: ~40k tokens used, 121k remaining

---

## Overview

Currently, the system shows simulated traffic metrics. We want to:
1. Keep simulation mode for demos/presentations
2. Add real traffic mode where services actually communicate and process data
3. Toggle between modes via UI

---

## Phase 1: Traffic Mode Toggle ✅

**Estimated Time**: 30 minutes  
**Status**: COMPLETE  
**Deployed**: Revision 60

### Backend Changes
- [x] Add `/api/traffic/mode` endpoint (GET/POST)
  - GET: Returns current mode (simulated/real)
  - POST: Sets mode (simulated/real)
- [x] Modify TrafficSimulationService to check mode
- [x] Add TrafficMode type ('simulated' | 'real')
- [x] Store mode preference (in-memory)

### Frontend Changes
- [x] Add toggle button in SystemTopology component
  - Location: Top-right corner with view toggles
  - States: "Simulated" (🎭 purple) / "Real" (⚡ green)
  - Tooltip: Shows current mode description
- [x] Store preference in localStorage
- [x] Show indicator of current mode (button color + icon)
- [x] Add loading state during mode switch

### Files Modified
- ✅ `services/control-plane/encabulation-control-console/server/src/index.ts`
- ✅ `services/control-plane/encabulation-control-console/server/src/services/traffic.ts`
- ✅ `services/control-plane/encabulation-control-console/src/components/SystemTopology.vue`
- ✅ `services/control-plane/encabulation-control-console/src/stores/system.ts`
- ✅ Fixed test files for compatibility

---

## Phase 2: Real Service Implementation 🎯

**Estimated Time**: 2-3 hours  
**Status**: Not Started

### 2.1 Panametric Fam Processor (Core Processing)
**Status**: Not Started

**What it does**:
- Accepts "encabulation requests" via REST API
- Processes data (simulated business logic)
- Stores results in PostgreSQL
- Caches in Redis
- Writes metrics to InfluxDB
- Publishes events to message queue

**Implementation**:
- [ ] Create `/api/v1/encabulations` POST endpoint
  - Accept: `{ data: string, priority: number }`
  - Return: `{ id: string, status: string, result: object }`
- [ ] Add PostgreSQL client and queries
  - Store encabulation records
  - Query history
- [ ] Add Redis client for caching
  - Cache recent results (TTL: 5 minutes)
  - Cache hit/miss tracking
- [ ] Add InfluxDB client for metrics
  - Write processing time, throughput
  - Track success/failure rates
- [ ] Add message queue publishing
  - Publish completion events
  - Include processing metadata

**Files**:
- `services/processing-plane/panametric-fam-processor/src/routes/encabulation.ts`
- `services/processing-plane/panametric-fam-processor/src/services/processor.ts`
- `services/processing-plane/panametric-fam-processor/src/services/dataPlaneClient.ts`

### 2.2 Dingle Arm API Gateway
**Status**: Not Started

**What it does**:
- Routes external requests to internal services
- Validates authentication via Unilateral Phase Detractor
- Rate limiting
- Request/response logging

**Implementation**:
- [ ] Create `/api/v1/encabulate` endpoint (public-facing)
- [ ] Add auth middleware (call unilateral-phase-detractor)
- [ ] Add rate limiting (100 req/min per user)
- [ ] Proxy to panametric-fam-processor
- [ ] Add request/response logging

**Files**:
- `services/processing-plane/dingle-arm-api-gateway/src/index.ts`
- `services/processing-plane/dingle-arm-api-gateway/src/middleware/auth.ts`

### 2.3 Cardinal Grammeter (Traffic Generator)
**Status**: Not Started

**What it does**:
- Generates real HTTP requests to API Gateway
- Configurable patterns (steady, burst, spike)
- Tracks success/failure rates

**Implementation**:
- [ ] Modify `/traffic/start` to make real HTTP calls
- [ ] Add configurable request rate
- [ ] Add request payload generation
- [ ] Track and report metrics
- [ ] Add `/traffic/stats` endpoint

**Files**:
- `services/control-plane/cardinal-grammeter/src/index.ts`
- `services/control-plane/cardinal-grammeter/src/generator.ts`

### 2.4 Data Store Services
**Status**: Already Deployed ✅

These are already running and accessible:
- ✅ Hydrocoptic DataStore (PostgreSQL wrapper)
- ✅ Prefabulated Cache (Redis wrapper)
- ✅ Sinusoidal Database (InfluxDB wrapper)

Just need to use them from Panametric Fam Processor!

---

## Phase 3: Real Metrics Collection 📊

**Estimated Time**: 1 hour  
**Status**: Not Started

### 3.1 OpenTelemetry Instrumentation
- [ ] Add OpenTelemetry SDK to services
- [ ] Auto-instrument Express apps
- [ ] Collect traces, metrics, logs
- [ ] Export to in-memory collector

### 3.2 Metrics Aggregation Service
- [ ] Create new service or enhance existing
- [ ] Aggregate metrics from all services
- [ ] Provide `/api/metrics/traffic` endpoint
- [ ] Return real traffic data in same format as simulation

### 3.3 Backend Integration
- [ ] Modify TrafficSimulationService
  - Check mode (simulated/real)
  - If real: fetch from metrics service
  - If simulated: use current logic
- [ ] Add fallback to simulation if real metrics unavailable

**Files**:
- `services/control-plane/encabulation-control-console/server/src/services/traffic.ts`
- `services/control-plane/encabulation-control-console/server/src/services/realMetrics.ts` (new)

---

## Testing Plan

### Manual Testing
- [ ] Toggle between modes in UI
- [ ] Verify simulated mode still works
- [ ] Start Cardinal Grammeter in real mode
- [ ] Verify real traffic flows through system
- [ ] Check data in PostgreSQL
- [ ] Check cache hits in Redis
- [ ] Check metrics in InfluxDB
- [ ] Verify UI shows real metrics

### Automated Testing
- [ ] Unit tests for new endpoints
- [ ] Integration tests for service communication
- [ ] Load test with Cardinal Grammeter

---

## Deployment Strategy

### Development
1. Deploy enhanced services to dev cluster
2. Test with simulated mode (default)
3. Switch to real mode and test
4. Verify no breaking changes

### Production
- Keep simulated mode as default
- Document how to enable real mode
- Add environment variable: `TRAFFIC_MODE=real|simulated`

---

## Success Criteria

- [ ] UI toggle works smoothly
- [ ] Simulated mode unchanged (backward compatible)
- [ ] Real mode shows actual service communication
- [ ] Data persists in databases
- [ ] Metrics reflect real traffic
- [ ] No performance degradation
- [ ] Documentation updated

---

## Notes

- **Current State**: Services exist but don't communicate
- **Target State**: Full distributed system with real data flow
- **Fallback**: Always default to simulation if real mode fails
- **Demo-Friendly**: Simulation mode perfect for presentations

---

## Progress Log

### 2025-10-13 - Phase 1 Complete ✅
- ✅ Created implementation plan
- ✅ Identified current state (mixed real/simulated)
- ✅ Defined three phases
- ✅ **Phase 1 Complete**: Traffic mode toggle working
  - Backend API endpoints functional
  - Frontend toggle button added (purple 🎭 for simulated, green ⚡ for real)
  - localStorage persistence working
  - Deployed to cluster (revision 60)
- 🎯 **Next**: Phase 2 - Make services actually work
