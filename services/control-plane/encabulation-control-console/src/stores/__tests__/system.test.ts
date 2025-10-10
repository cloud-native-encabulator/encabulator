import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useSystemStore } from '../system'
import type { Service, SystemMetrics } from '@/types'

describe('System Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('initializes with default values', () => {
    const store = useSystemStore()
    
    expect(store.services).toEqual({})
    expect(store.connected).toBe(false)
    expect(store.totalServices).toBe(0)
    expect(store.healthyServices).toBe(0)
    expect(store.systemHealth).toBe('unknown')
  })

  it('updates services correctly', () => {
    const store = useSystemStore()
    const mockServices: Record<string, Service> = {
      'service-1': { status: 'healthy', replicas: 3 },
      'service-2': { status: 'degraded', replicas: 2 }
    }
    
    store.updateServices(mockServices)
    
    expect(store.services).toEqual(mockServices)
    expect(store.totalServices).toBe(2)
    expect(store.healthyServices).toBe(1)
    expect(store.lastUpdate).toBeInstanceOf(Date)
  })

  it('calculates system health correctly', () => {
    const store = useSystemStore()
    
    // All healthy
    store.updateServices({
      's1': { status: 'healthy', replicas: 1 },
      's2': { status: 'healthy', replicas: 1 }
    })
    expect(store.systemHealth).toBe('healthy')
    
    // 80% healthy (degraded)
    store.updateServices({
      's1': { status: 'healthy', replicas: 1 },
      's2': { status: 'healthy', replicas: 1 },
      's3': { status: 'healthy', replicas: 1 },
      's4': { status: 'healthy', replicas: 1 },
      's5': { status: 'unhealthy', replicas: 0 }
    })
    expect(store.systemHealth).toBe('degraded')
    
    // Less than 80% healthy (unhealthy)
    store.updateServices({
      's1': { status: 'healthy', replicas: 1 },
      's2': { status: 'unhealthy', replicas: 0 },
      's3': { status: 'unhealthy', replicas: 0 }
    })
    expect(store.systemHealth).toBe('unhealthy')
  })

  it('updates metrics correctly', () => {
    const store = useSystemStore()
    const mockMetrics: SystemMetrics = {
      totalRequests: 1000,
      avgLatency: 50,
      errorRate: 0.01,
      cpuUsage: 45.5,
      memoryUsage: 60.2
    }
    
    store.updateMetrics(mockMetrics)
    
    expect(store.metrics).toEqual(mockMetrics)
  })

  it('updates individual service status', () => {
    const store = useSystemStore()
    store.updateServices({
      'test-service': { status: 'healthy', replicas: 3 }
    })
    
    store.updateServiceStatus('test-service', 'degraded')
    
    expect(store.services['test-service'].status).toBe('degraded')
  })

  it('sets connection status', () => {
    const store = useSystemStore()
    
    store.setConnected(true)
    expect(store.connected).toBe(true)
    
    store.setConnected(false)
    expect(store.connected).toBe(false)
  })
})
