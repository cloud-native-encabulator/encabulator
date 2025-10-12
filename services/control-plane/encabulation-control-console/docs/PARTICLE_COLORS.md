# Particle Color Coding System

## Overview

Traffic flow particles in the System Topology visualization are color-coded based on real-time health metrics for each service-to-service connection. This provides instant visual feedback about the health of traffic flowing through the system.

## Color Scheme

### 🟢 Green (`#00ff88`) - Healthy Traffic
- **Latency**: < 100ms
- **Error Rate**: < 1%
- **Health Status**: `healthy`
- **Meaning**: Traffic is flowing normally with good performance

### 🟡 Yellow (`#ffaa00`) - Warning
- **Latency**: 100-500ms
- **Error Rate**: < 1%
- **Health Status**: `warning`
- **Meaning**: Elevated latency detected, but errors are within acceptable range

### 🔴 Red (`#ff0055`) - Error
- **Latency**: > 500ms OR
- **Error Rate**: > 1%
- **Health Status**: `error`
- **Meaning**: High latency or elevated error rate detected

### ⚪ Gray (`#666666`) - Unknown
- **Health Status**: `unknown` or metrics unavailable
- **Meaning**: No traffic metrics available for this connection

## Implementation

### Color Determination Logic

The `getParticleColor()` function in `SystemTopology.vue` determines particle color based on traffic metrics:

```typescript
function getParticleColor(sourceId: string, targetId: string): string {
  // Find traffic metrics for this connection
  const metrics = systemStore.trafficMetrics.find(
    m => m.source === sourceId && m.target === targetId
  )
  
  if (!metrics) {
    return '#666666' // Gray - no metrics
  }
  
  // Use health status from traffic metrics
  switch (metrics.healthStatus) {
    case 'healthy':
      return '#00ff88' // Green
    case 'warning':
      return '#ffaa00' // Yellow
    case 'error':
      return '#ff0055' // Red
    default:
      return '#666666' // Gray
  }
}
```

### Health Status Determination

Health status is determined by the `TrafficSimulationService` on the backend:

```typescript
private determineHealthStatus(latency: number, errorRate: number): 'healthy' | 'warning' | 'error' | 'unknown' {
  // High error rate or very high latency = error
  if (errorRate > 0.01 || latency > 500) {
    return 'error'
  }
  
  // Elevated latency = warning
  if (latency > 100) {
    return 'warning'
  }
  
  // Normal = healthy
  return 'healthy'
}
```

## Visual Examples

### Typical Healthy System
Most connections show **green particles** flowing smoothly between services, indicating normal operation.

### Degraded Performance
Some connections show **yellow particles**, indicating elevated latency but no errors. This might occur during:
- High load periods
- Database query slowdowns
- Network congestion

### Critical Issues
**Red particles** indicate serious problems:
- Service timeouts (> 500ms)
- High error rates (> 1%)
- Failed requests
- Service degradation

### Missing Metrics
**Gray particles** appear when:
- Service just started and metrics aren't available yet
- Metrics collection is disabled
- Connection is not actively monitored

## Real-Time Updates

Particle colors update automatically every 5 seconds as new traffic metrics are received via:
- WebSocket push updates
- REST API polling (fallback)

This ensures the visualization always reflects current system health.

## Usage Tips

### Identifying Problems Quickly
1. **Scan for red particles** - immediate attention needed
2. **Check yellow particle patterns** - potential performance issues
3. **Monitor gray particles** - verify metrics collection is working

### Understanding Traffic Patterns
- **Particle density** = traffic volume (more particles = more requests)
- **Particle color** = connection health
- **Particle speed** = varies slightly for visual effect (not meaningful)

### Troubleshooting

**All particles are gray:**
- Check if traffic metrics are being collected
- Verify WebSocket connection is active
- Check browser console for errors

**Colors don't match expected health:**
- Verify traffic metrics endpoint: `GET /api/traffic`
- Check if simulation is enabled (dev mode)
- Inspect `systemStore.trafficMetrics` in browser console

**Particles not animating:**
- Toggle "View Traffic" button
- Check browser performance (60fps required)
- Verify canvas element is rendering

## Future Enhancements

### Real Traffic Integration
When switching from simulated to real traffic metrics:
- Colors will reflect actual service mesh data
- Prometheus queries will provide latency/error rates
- Istio/Linkerd metrics will drive visualization

### Additional Color Coding
Potential future enhancements:
- **Blue particles**: Retried requests
- **Purple particles**: Circuit breaker open
- **Orange particles**: Rate limited requests
- **Gradient colors**: Interpolate between states

### Interactive Features
- Click particle to see request details
- Hover to show latency/error rate
- Filter by health status
- Replay historical traffic patterns
