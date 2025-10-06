import { MetricsService } from './metrics'
import { register, Counter, Gauge } from 'prom-client'

jest.mock('prom-client', () => {
  const mockCounter = {
    inc: jest.fn(),
  }
  const mockGauge = {
    set: jest.fn(),
  }

  return {
    register: {
      registerMetric: jest.fn(),
    },
    Counter: jest.fn(() => mockCounter),
    Gauge: jest.fn(() => mockGauge),
  }
})

describe('MetricsService', () => {
  let service: MetricsService
  let mockCounter: any
  let mockGauge: any

  beforeEach(() => {
    jest.clearAllMocks()
    service = new MetricsService()
    mockCounter = (Counter as jest.Mock).mock.results[0].value
    mockGauge = (Gauge as jest.Mock).mock.results[0].value
  })

  describe('constructor', () => {
    it('initializes request counter with correct config', () => {
      expect(Counter).toHaveBeenCalledWith({
        name: 'control_console_requests_total',
        help: 'Total number of requests to control console',
        labelNames: ['type'],
      })
    })

    it('initializes connection gauge with correct config', () => {
      expect(Gauge).toHaveBeenCalledWith({
        name: 'control_console_connections',
        help: 'Number of active WebSocket connections',
      })
    })

    it('registers metrics with Prometheus', () => {
      expect(register.registerMetric).toHaveBeenCalledTimes(2)
    })
  })

  describe('incrementRequests', () => {
    it('increments request counter with correct label', () => {
      service.incrementRequests('scale_service')

      expect(mockCounter.inc).toHaveBeenCalledWith({ type: 'scale_service' })
    })

    it('increments counter for different request types', () => {
      service.incrementRequests('inject_failure')
      service.incrementRequests('run_scenario')

      expect(mockCounter.inc).toHaveBeenCalledWith({ type: 'inject_failure' })
      expect(mockCounter.inc).toHaveBeenCalledWith({ type: 'run_scenario' })
      expect(mockCounter.inc).toHaveBeenCalledTimes(2)
    })
  })

  describe('setConnections', () => {
    it('sets connection gauge to correct value', () => {
      service.setConnections(5)

      expect(mockGauge.set).toHaveBeenCalledWith(5)
    })

    it('updates connection count', () => {
      service.setConnections(10)
      service.setConnections(8)

      expect(mockGauge.set).toHaveBeenCalledWith(10)
      expect(mockGauge.set).toHaveBeenCalledWith(8)
      expect(mockGauge.set).toHaveBeenCalledTimes(2)
    })

    it('handles zero connections', () => {
      service.setConnections(0)

      expect(mockGauge.set).toHaveBeenCalledWith(0)
    })
  })

  describe('getSystemMetrics', () => {
    it('returns metrics object with expected keys', async () => {
      const metrics = await service.getSystemMetrics()

      expect(metrics).toHaveProperty('request_rate')
      expect(metrics).toHaveProperty('avg_latency')
      expect(metrics).toHaveProperty('cpu_usage')
      expect(metrics).toHaveProperty('memory_usage')
      expect(metrics).toHaveProperty('error_rate')
    })

    it('returns numeric values for all metrics', async () => {
      const metrics = await service.getSystemMetrics()

      expect(typeof metrics.request_rate).toBe('number')
      expect(typeof metrics.avg_latency).toBe('number')
      expect(typeof metrics.cpu_usage).toBe('number')
      expect(typeof metrics.memory_usage).toBe('number')
      expect(typeof metrics.error_rate).toBe('number')
    })

    it('returns values within expected ranges', async () => {
      const metrics = await service.getSystemMetrics()

      expect(metrics.request_rate).toBeGreaterThanOrEqual(0)
      expect(metrics.request_rate).toBeLessThanOrEqual(100)
      
      expect(metrics.avg_latency).toBeGreaterThanOrEqual(0)
      expect(metrics.avg_latency).toBeLessThanOrEqual(200)
      
      expect(metrics.cpu_usage).toBeGreaterThanOrEqual(0)
      expect(metrics.cpu_usage).toBeLessThanOrEqual(80)
      
      expect(metrics.memory_usage).toBeGreaterThanOrEqual(0)
      expect(metrics.memory_usage).toBeLessThanOrEqual(70)
      
      expect(metrics.error_rate).toBeGreaterThanOrEqual(0)
      expect(metrics.error_rate).toBeLessThanOrEqual(5)
    })
  })
})
