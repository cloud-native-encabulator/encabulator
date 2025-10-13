import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import axios from 'axios'
import type { Service, SystemMetrics, TrafficMetrics } from '@/types'

export type TrafficMode = 'simulated' | 'real'

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
  const trafficMode = ref<TrafficMode>('simulated')
  const trafficModeLoading = ref(false)

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

  async function fetchTrafficMode() {
    try {
      const response = await axios.get('/api/traffic/mode')
      trafficMode.value = response.data.mode
      return response.data.mode
    } catch (error) {
      console.error('Failed to fetch traffic mode:', error)
      return trafficMode.value
    }
  }

  async function setTrafficMode(mode: TrafficMode) {
    trafficModeLoading.value = true
    try {
      const response = await axios.post('/api/traffic/mode', { mode })
      trafficMode.value = response.data.mode
      
      // Store preference in localStorage
      localStorage.setItem('traffic_mode', mode)
      
      return true
    } catch (error) {
      console.error('Failed to set traffic mode:', error)
      return false
    } finally {
      trafficModeLoading.value = false
    }
  }

  // Initialize traffic mode from localStorage
  function initializeTrafficMode() {
    const stored = localStorage.getItem('traffic_mode')
    if (stored === 'simulated' || stored === 'real') {
      trafficMode.value = stored
      // Sync with backend
      setTrafficMode(stored)
    } else {
      // Fetch from backend
      fetchTrafficMode()
    }
  }

  return {
    services,
    metrics,
    trafficMetrics,
    connected,
    lastUpdate,
    trafficMode,
    trafficModeLoading,
    healthyServices,
    totalServices,
    systemHealth,
    updateServices,
    updateMetrics,
    updateTrafficMetrics,
    updateServiceStatus,
    setConnected,
    fetchTrafficMode,
    setTrafficMode,
    initializeTrafficMode
  }
})
