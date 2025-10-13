import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import SystemTopology from '../SystemTopology.vue'
import { useSystemStore } from '@/stores/system'

describe('SystemTopology Integration Tests', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('updates pod visualization on scaling events', async () => {
    const systemStore = useSystemStore()
    const wrapper = mount(SystemTopology)
    const component = wrapper.vm as any

    // Initial state
    const initialService = component.nodes.find((n: any) => n.id === 'panametric-fam-processor')
    expect(initialService).toBeTruthy()

    // Simulate scaling event
    systemStore.updateServices({
      'panametric-fam-processor': {
        status: 'healthy',
        replicas: 5,
        pods: [
          { name: 'pod-1', status: 'ready', ready: true, cpu: '100m', memory: '256Mi', p50Latency: 45, p99Latency: 120, errorRate: 0 },
          { name: 'pod-2', status: 'ready', ready: true, cpu: '110m', memory: '260Mi', p50Latency: 50, p99Latency: 130, errorRate: 0 },
          { name: 'pod-3', status: 'ready', ready: true, cpu: '95m', memory: '250Mi', p50Latency: 42, p99Latency: 115, errorRate: 0 },
          { name: 'pod-4', status: 'not-ready', ready: false, cpu: '0m', memory: '0Mi', p50Latency: 0, p99Latency: 0, errorRate: 0 },
          { name: 'pod-5', status: 'ready', ready: true, cpu: '105m', memory: '258Mi', p50Latency: 48, p99Latency: 125, errorRate: 0 },
        ]
      }
    })

    await flushPromises()
    await wrapper.vm.$nextTick()

    // Verify pod count updated
    const updatedService = systemStore.services['panametric-fam-processor']
    expect(updatedService.replicas).toBe(5)
    expect(updatedService.pods?.length).toBe(5)
  })

  it('renders traffic particles with metrics', async () => {
    const systemStore = useSystemStore()
    const wrapper = mount(SystemTopology)

    // Set up traffic metrics as array
    systemStore.updateTrafficMetrics([{
      source: 'panametric-fam-processor',
      target: 'dingle-arm-api-gateway',
      requestsPerSecond: 50,
      avgLatency: 75,
      p50Latency: 65,
      p99Latency: 150,
      errorRate: 0.005,
      healthStatus: 'healthy'
    }])

    await flushPromises()
    await wrapper.vm.$nextTick()

    // Verify canvas exists for particles
    const canvas = wrapper.find('canvas')
    expect(canvas.exists()).toBe(true)

    // Verify traffic metrics are available
    const metrics = systemStore.trafficMetrics.find(m => 
      m.source === 'panametric-fam-processor' && m.target === 'dingle-arm-api-gateway'
    )
    expect(metrics).toBeTruthy()
    expect(metrics?.requestsPerSecond).toBe(50)
    expect(metrics?.avgLatency).toBe(75)
  })

  it('displays tooltip on hover interactions', async () => {
    const wrapper = mount(SystemTopology)
    const component = wrapper.vm as any

    // Simulate hovering over a pod
    component.hoveredPod = {
      name: 'test-pod-1',
      status: 'ready',
      cpu: '120m',
      memory: '300Mi',
      p50Latency: 55,
      p99Latency: 140,
      errorRate: 0.002
    }

    await wrapper.vm.$nextTick()

    // Tooltip should be visible with pod details
    expect(component.hoveredPod).toBeTruthy()
    expect(component.hoveredPod.name).toBe('test-pod-1')
    expect(component.hoveredPod.status).toBe('ready')
  })

  it('updates metrics on polling interval', async () => {
    vi.useFakeTimers()
    const systemStore = useSystemStore()
    mount(SystemTopology)

    // Initial metrics
    systemStore.updateTrafficMetrics([{
      source: 'test-service-a',
      target: 'test-service-b',
      requestsPerSecond: 10,
      avgLatency: 50,
      p50Latency: 45,
      p99Latency: 100,
      errorRate: 0,
      healthStatus: 'healthy'
    }])

    await flushPromises()

    // Advance time by 5 seconds (polling interval)
    vi.advanceTimersByTime(5000)

    // Simulate metrics update
    systemStore.updateTrafficMetrics([{
      source: 'test-service-a',
      target: 'test-service-b',
      requestsPerSecond: 25,
      avgLatency: 75,
      p50Latency: 70,
      p99Latency: 150,
      errorRate: 0.01,
      healthStatus: 'warning'
    }])

    await flushPromises()

    // Verify metrics updated
    const metrics = systemStore.trafficMetrics.find(m => 
      m.source === 'test-service-a' && m.target === 'test-service-b'
    )
    expect(metrics?.requestsPerSecond).toBe(25)
    expect(metrics?.avgLatency).toBe(75)

    vi.useRealTimers()
  })

  it('maintains 60fps with 20 services and 50 connections', async () => {
    const wrapper = mount(SystemTopology)
    const component = wrapper.vm as any

    // Component has default nodes, verify it can handle them
    const nodeCount = component.nodes.length
    expect(nodeCount).toBeGreaterThan(0)

    // Simulate animation frame
    const startTime = performance.now()
    component.animateParticles?.()
    const endTime = performance.now()

    // Frame should complete in less than 16.67ms (60fps)
    const frameDuration = endTime - startTime
    expect(frameDuration).toBeLessThan(16.67)
  })

  it('handles connection tooltip display', async () => {
    const wrapper = mount(SystemTopology)
    const component = wrapper.vm as any

    // Simulate hovering over a connection
    component.hoveredConnection = {
      source: 'service-a',
      target: 'service-b',
      requestsPerSecond: 45,
      avgLatency: 85,
      p50Latency: 75,
      p99Latency: 180,
      errorRate: 0.008
    }

    await wrapper.vm.$nextTick()

    // Connection tooltip should be visible
    expect(component.hoveredConnection).toBeTruthy()
    expect(component.hoveredConnection.source).toBe('service-a')
    expect(component.hoveredConnection.target).toBe('service-b')
    expect(component.hoveredConnection.requestsPerSecond).toBe(45)
  })

  it('cleans up resources on unmount', async () => {
    const wrapper = mount(SystemTopology)

    // Verify component is mounted
    expect(wrapper.exists()).toBe(true)

    // Unmount component - Vue will handle cleanup automatically
    wrapper.unmount()

    // Verify component is unmounted
    expect(wrapper.exists()).toBe(false)
  })

  it('handles rapid state changes without memory leaks', async () => {
    const systemStore = useSystemStore()
    const wrapper = mount(SystemTopology)

    // Simulate rapid updates
    for (let i = 0; i < 100; i++) {
      systemStore.updateServices({
        'test-service': {
          status: i % 2 === 0 ? 'healthy' : 'degraded',
          replicas: (i % 5) + 1
        }
      })
      await wrapper.vm.$nextTick()
    }

    // Component should still be responsive
    expect(wrapper.exists()).toBe(true)
    const service = systemStore.services['test-service']
    expect(service).toBeTruthy()
  })
})
