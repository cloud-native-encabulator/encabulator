import { register, Counter, Gauge } from 'prom-client'

export class MetricsService {
  private requestCounter: Counter
  private connectionGauge: Gauge

  constructor() {
    // Initialize Prometheus metrics
    this.requestCounter = new Counter({
      name: 'control_console_requests_total',
      help: 'Total number of requests to control console',
      labelNames: ['type'],
    })

    this.connectionGauge = new Gauge({
      name: 'control_console_connections',
      help: 'Number of active WebSocket connections',
    })

    register.registerMetric(this.requestCounter)
    register.registerMetric(this.connectionGauge)
  }

  incrementRequests(type: string): void {
    this.requestCounter.inc({ type })
  }

  setConnections(count: number): void {
    this.connectionGauge.set(count)
  }

  async getSystemMetrics(): Promise<Record<string, number>> {
    // Mock metrics for demonstration
    // In production, these would come from Prometheus queries
    return {
      totalRequests: Math.floor(Math.random() * 10000) + 5000,
      avgLatency: Math.floor(Math.random() * 100) + 20, // 20-120ms
      errorRate: Math.random() * 0.02, // 0-0.02 (0-2% when displayed)
      cpuUsage: Math.floor(Math.random() * 40) + 30, // 30-70%
      memoryUsage: Math.floor(Math.random() * 30) + 40, // 40-70%
    }
  }
}
