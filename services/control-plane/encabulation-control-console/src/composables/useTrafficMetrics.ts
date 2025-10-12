import { ref, onMounted, onUnmounted } from 'vue'
import { useSystemStore } from '@/stores/system'
import type { TrafficMetrics } from '@/types'

/**
 * Composable for managing traffic metrics polling and access
 * 
 * This composable handles:
 * - Automatic polling of traffic metrics every 5 seconds
 * - Error handling and recovery
 * - Lifecycle management (auto-start on mount, auto-stop on unmount)
 * - Helper methods for querying metrics
 * 
 * Usage example:
 * ```typescript
 * const { isPolling, error, getMetricsForConnection } = useTrafficMetrics()
 * 
 * // Get metrics for a specific connection
 * const metrics = getMetricsForConnection('source-service', 'target-service')
 * if (metrics) {
 *   console.log(`RPS: ${metrics.requestsPerSecond}, Latency: ${metrics.avgLatency}ms`)
 * }
 * ```
 */
export function useTrafficMetrics() {
  const systemStore = useSystemStore()
  const isPolling = ref(false)
  const error = ref<string | null>(null)
  const lastFetchTime = ref<Date | null>(null)
  let pollingInterval: number | null = null

  /**
   * Fetch traffic metrics from the API
   */
  async function fetchTrafficMetrics(): Promise<void> {
    try {
      const response = await fetch('/api/traffic')
      
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`)
      }
      
      const data = await response.json()
      
      if (data.trafficMetrics && Array.isArray(data.trafficMetrics)) {
        systemStore.updateTrafficMetrics(data.trafficMetrics)
        lastFetchTime.value = new Date()
        error.value = null
      } else {
        throw new Error('Invalid traffic metrics data format')
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Unknown error'
      error.value = `Failed to fetch traffic metrics: ${errorMessage}`
      console.error('Traffic metrics fetch error:', err)
      
      // On error, clear traffic metrics to avoid showing stale data
      systemStore.updateTrafficMetrics([])
    }
  }

  /**
   * Start polling for traffic metrics every 5 seconds
   */
  function startPolling(): void {
    if (isPolling.value) {
      return
    }

    isPolling.value = true
    
    // Fetch immediately
    fetchTrafficMetrics()
    
    // Then poll every 5 seconds
    pollingInterval = window.setInterval(() => {
      fetchTrafficMetrics()
    }, 5000)
  }

  /**
   * Stop polling for traffic metrics
   */
  function stopPolling(): void {
    if (pollingInterval !== null) {
      clearInterval(pollingInterval)
      pollingInterval = null
    }
    isPolling.value = false
  }

  /**
   * Get traffic metrics for a specific connection
   */
  function getMetricsForConnection(source: string, target: string): TrafficMetrics | undefined {
    return systemStore.trafficMetrics.find(
      m => m.source === source && m.target === target
    )
  }

  /**
   * Get all traffic metrics for a specific service (as source or target)
   */
  function getMetricsForService(serviceName: string): TrafficMetrics[] {
    return systemStore.trafficMetrics.filter(
      m => m.source === serviceName || m.target === serviceName
    )
  }

  // Auto-start polling when component mounts
  onMounted(() => {
    startPolling()
  })

  // Auto-stop polling when component unmounts
  onUnmounted(() => {
    stopPolling()
  })

  return {
    isPolling,
    error,
    lastFetchTime,
    fetchTrafficMetrics,
    startPolling,
    stopPolling,
    getMetricsForConnection,
    getMetricsForService
  }
}
