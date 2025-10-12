# Traffic Metrics System

## Overview

The traffic metrics system provides real-time visibility into service-to-service communication patterns, latency, and error rates within the Cloud Native Encabulator microservices architecture.

## Features

### Simulated Traffic Metrics

For development and demonstration purposes, the system generates realistic simulated traffic metrics based on the service topology. This allows for:

- **Rapid development**: Build and test UI components without requiring real traffic
- **Consistent demos**: Predictable traffic patterns for presentations
- **Edge case testing**: Simulate various traffic conditions (high load, errors, latency spikes)

### Metrics Collected

For each service-to-service connection, the following metrics are tracked:

- **Requests Per Second (RPS)**: Volume of traffic between services
- **Average Latency**: Mean response time
- **P50 Latency**: 50th percentile response time
- **P99 Latency**: 99th percentile response time
- **Error Rate**: Percentage of failed requests
- **Health Status**: Overall connection health (`healthy`, `warning`, `error`, `unknown`)

### Health Status Determination

Traffic health is automatically determined based on latency and error rates:

- **Healthy** (Green): Latency < 100ms, Error rate < 1%
- **Warning** (Yellow): Latency 100-500ms, Error rate < 1%
- **Error** (Red): Latency > 500ms OR Error rate > 1%
- **Unknown** (Gray): Metrics unavailable

## Architecture

### Backend Components

#### TrafficSimulationService

Located at: `server/src/services/traffic.ts`

Generates realistic traffic metrics based on:
- Service topology (predefined connections)
- Service type (data stores have higher latency than caches)
- Time-based variations (sine wave patterns)
- Random fluctuations for realism

#### API Endpoints

**REST API**: `GET /api/traffic`
```json
{
  "trafficMetrics": [
    {
      "source": "dingle-arm-api-gateway",
      "target": "panametric-fam-processor",
      "requestsPerSecond": 32.5,
      "avgLatency": 45,
      "p50Latency": 40,
      "p99Latency": 112,
      "errorRate": 0.0012,
      "healthStatus": "healthy"
    }
  ],
  "timestamp": "2025-01-10T12:34:56.789Z"
}
```

**WebSocket**: Real-time updates every 5 seconds via `system_status` messages

### Frontend Components

#### System Store

The Pinia store (`src/stores/system.ts`) maintains traffic metrics state:

```typescript
const systemStore = useSystemStore()
const metrics = systemStore.trafficMetrics // Array of TrafficMetrics
```

#### useTrafficMetrics Composable

Located at: `src/composables/useTrafficMetrics.ts`

Provides convenient access to traffic metrics with automatic polling:

```typescript
import { useTrafficMetrics } from '@/composables/useTrafficMetrics'

const { 
  isPolling,           // Boolean: is polling active?
  error,               // String | null: last error message
  lastFetchTime,       // Date | null: last successful fetch
  getMetricsForConnection,  // Get metrics for specific connection
  getMetricsForService      // Get all metrics for a service
} = useTrafficMetrics()

// Get metrics for a specific connection
const metrics = getMetricsForConnection('source-service', 'target-service')
if (metrics) {
  console.log(`RPS: ${metrics.requestsPerSecond}`)
  console.log(`Latency: ${metrics.avgLatency}ms`)
  console.log(`Health: ${metrics.healthStatus}`)
}
```

## Usage Examples

### In a Vue Component

```vue
<script setup lang="ts">
import { useTrafficMetrics } from '@/composables/useTrafficMetrics'
import { useSystemStore } from '@/stores/system'

const systemStore = useSystemStore()
const { getMetricsForConnection } = useTrafficMetrics()

// Access all traffic metrics
const allMetrics = computed(() => systemStore.trafficMetrics)

// Get metrics for a specific connection
const gatewayToProcessor = computed(() => 
  getMetricsForConnection('dingle-arm-api-gateway', 'panametric-fam-processor')
)
</script>

<template>
  <div v-if="gatewayToProcessor">
    <p>RPS: {{ gatewayToProcessor.requestsPerSecond }}</p>
    <p>Latency: {{ gatewayToProcessor.avgLatency }}ms</p>
    <p>Status: {{ gatewayToProcessor.healthStatus }}</p>
  </div>
</template>
```

### Accessing via WebSocket

Traffic metrics are automatically pushed via WebSocket every 5 seconds. The `useWebSocket` composable handles this automatically and updates the store.

## Error Handling

The system includes comprehensive error handling:

1. **Network Failures**: Retries with exponential backoff
2. **Invalid Data**: Validation and fallback to empty state
3. **Stale Data**: Clears metrics on error to avoid showing outdated information
4. **Logging**: All errors are logged to console for debugging

## Future Enhancements

### Real Traffic Integration

When ready to switch from simulated to real traffic:

1. **Service Mesh Integration**: Query Istio/Linkerd for actual traffic metrics
2. **Prometheus Integration**: Pull metrics from Prometheus queries
3. **Toggle Mode**: Add UI control to switch between simulated and real metrics
4. **Hybrid Mode**: Use real metrics when available, fall back to simulation

### Example Real Traffic Implementation

```typescript
// In TrafficSimulationService
async getRealTrafficMetrics(): Promise<TrafficMetrics[]> {
  // Query Prometheus for actual traffic metrics
  const query = `rate(istio_requests_total[5m])`
  const response = await fetch(`/api/prometheus/query?query=${query}`)
  // Parse and transform to TrafficMetrics format
  return transformPrometheusData(response)
}
```

## Configuration

### Polling Interval

Default: 5 seconds (configurable in `useTrafficMetrics.ts`)

```typescript
// Change polling interval
pollingInterval = window.setInterval(() => {
  fetchTrafficMetrics()
}, 3000) // 3 seconds instead of 5
```

### Simulation Parameters

Adjust simulation behavior in `TrafficSimulationService`:

```typescript
// Modify base RPS calculation
const baseRps = link.value * 10 // Increase multiplier for more traffic

// Adjust variation amplitude
const variation = Math.sin(now / 10000) * 0.5 + 1 // ±50% instead of ±30%

// Change error rate
const baseErrorRate = 0.005 // 0.5% instead of 0.1%
```

## Testing

### Manual Testing

1. Start the control console: `npm run dev`
2. Open browser console
3. Check for traffic metrics in store: `$pinia.state.value.system.trafficMetrics`
4. Verify polling: Look for `/api/traffic` requests every 5 seconds

### Automated Testing

```typescript
import { describe, it, expect } from 'vitest'
import { TrafficSimulationService } from './traffic'

describe('TrafficSimulationService', () => {
  it('generates metrics for all connections', () => {
    const service = new TrafficSimulationService()
    const metrics = service.getTrafficMetrics()
    
    expect(metrics.length).toBeGreaterThan(0)
    expect(metrics[0]).toHaveProperty('source')
    expect(metrics[0]).toHaveProperty('requestsPerSecond')
  })
})
```

## Troubleshooting

### No Traffic Metrics Displayed

1. Check browser console for errors
2. Verify `/api/traffic` endpoint is accessible
3. Check WebSocket connection status
4. Verify store is being updated: `systemStore.trafficMetrics`

### Stale or Incorrect Data

1. Check `lastFetchTime` to verify polling is active
2. Look for error messages in `error` ref
3. Verify backend service is running
4. Check network tab for failed requests

### Performance Issues

1. Reduce polling frequency
2. Limit number of connections in topology
3. Check browser performance profiler
4. Verify particle animation is not running simultaneously
