import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Service, SystemMetrics, TrafficMetrics } from '@/types'

export const useSystemStore = defineStore('system', () => {
  const services = ref<Record<string, Service>>({})
  const metrics = ref<SystemMetrics>({
    totalRequests: 0,
    avgLatency: 0,
    errorRate: 0,
    cpuUsage: 0,
    memoryUsage: 0
  })
  const trafficMetrics = ref<TrafficMetrics[]>([])
  const connected = ref(false)
  const lastUpdate = ref<Date | null>(null)

  const healthyServices = computed(() => 
    Object.entries(services.value)
      .filter(([_, service]) => service.status === 'healthy')
      .length
  )

  const totalServices = computed(() => Object.keys(services.value).length)

  const systemHealth = computed(() => {
    if (totalServices.value === 0) return 'unknown'
    const healthPercentage = (healthyServices.value / totalServices.value) * 100
    if (healthPercentage === 100) return 'healthy'
    if (healthPercentage >= 80) return 'degraded'
    return 'unhealthy'
  })

  function updateServices(data: Record<string, Service>) {
    services.value = data
    lastUpdate.value = new Date()
  }

  function updateMetrics(data: SystemMetrics) {
    metrics.value = data
  }

  function updateTrafficMetrics(data: TrafficMetrics[]) {
    trafficMetrics.value = data
  }

  function updateServiceStatus(serviceName: string, status: Service['status']) {
    if (services.value[serviceName]) {
      services.value[serviceName].status = status
    }
  }

  function setConnected(status: boolean) {
    connected.value = status
  }

  return {
    services,
    metrics,
    trafficMetrics,
    connected,
    lastUpdate,
    healthyServices,
    totalServices,
    systemHealth,
    updateServices,
    updateMetrics,
    updateTrafficMetrics,
    updateServiceStatus,
    setConnected
  }
})
