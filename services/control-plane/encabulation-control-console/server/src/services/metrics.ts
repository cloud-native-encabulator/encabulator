import { register, Counter, Gauge } from 'prom-client'

const PROMETHEUS_URL = process.env.PROMETHEUS_URL || 'http://prometheus-stack-kube-prom-prometheus.encabulator-monitoring.svc.cluster.local:9090'

interface PrometheusQueryResult {
  status: string
  data: {
    resultType: string
    result: Array<{
      metric: Record<string, string>
      value: [number, string]
    }>
  }
}

export type MetricsMode = 'real' | 'simulated'

export class MetricsService {
  private requestCounter: Counter
  private connectionGauge: Gauge
  private metricsMode: MetricsMode = 'real'
  private prometheusAvailable: boolean = true

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

  /**
   * Get current metrics mode
   */
  public getMetricsMode(): MetricsMode {
    return this.metricsMode
  }

  /**
   * Set metrics mode (real or simulated)
   */
  public setMetricsMode(mode: MetricsMode): void {
    this.metricsMode = mode
    console.log(`Metrics mode set to: ${mode}`)
  }

  private async queryPrometheus(query: string): Promise<number> {
    try {
      const url = `${PROMETHEUS_URL}/api/v1/query?query=${encodeURIComponent(query)}`
      const response = await fetch(url)
      
      if (!response.ok) {
        console.error(`Prometheus query failed: ${response.statusText}`)
        return 0
      }

      const data = await response.json() as PrometheusQueryResult
      
      if (data.status === 'success' && data.data.result.length > 0) {
        return parseFloat(data.data.result[0].value[1])
      }
      
      return 0
    } catch (error) {
      // Only log error once per check cycle
      if (this.prometheusAvailable) {
        console.error('Prometheus unavailable, falling back to simulated metrics')
        this.prometheusAvailable = false
      }
      return 0
    }
  }

  /**
   * Generate simulated metrics for demo/development purposes
   */
  private getSimulatedMetrics(): Record<string, number> {
    const now = Date.now()
    const variation = Math.sin(now / 10000) * 0.3 + 1 // Sine wave variation ±30%
    
    return {
      totalRequests: Math.floor((Math.random() * 100 + 150) * variation), // 100-300 req/s with variation
      avgLatency: Math.floor(Math.random() * 80 + 20), // 20-100ms
      errorRate: Math.random() * 0.015, // 0-1.5%
      cpuUsage: Math.floor(Math.random() * 30 + 35), // 35-65%
      memoryUsage: Math.floor(Math.random() * 25 + 45), // 45-70%
    }
  }

  async getSystemMetrics(): Promise<Record<string, number>> {
    // If in simulated mode, return simulated metrics
    if (this.metricsMode === 'simulated') {
      return this.getSimulatedMetrics()
    }

    try {
      // Query real metrics from Prometheus
      // Use the actual metrics exposed by our services:
      // - encabulations_processed_total: from panametric-fam-processor
      // - encabulation_processing_duration_seconds: from panametric-fam-processor
      // - http_request_duration_seconds: from unilateral-phase-detractor
      const [totalRequests, avgLatency, errorRate, cpuUsage, memoryUsage] = await Promise.all([
        // Total encabulations processed per second (this is our main throughput metric)
        this.queryPrometheus('sum(rate(encabulations_processed_total{namespace=~"encabulator-.*"}[1m]))'),
        
        // Average encabulation processing latency in milliseconds
        this.queryPrometheus('(sum(rate(encabulation_processing_duration_seconds_sum{namespace=~"encabulator-.*"}[1m])) / sum(rate(encabulation_processing_duration_seconds_count{namespace=~"encabulator-.*"}[1m]))) * 1000'),
        
        // Error rate from encabulations (failed / total)
        this.queryPrometheus('sum(rate(encabulations_processed_total{namespace=~"encabulator-.*",status="error"}[1m])) / sum(rate(encabulations_processed_total{namespace=~"encabulator-.*"}[1m]))'),
        
        // Average CPU usage across encabulator pods (as percentage)
        this.queryPrometheus('avg(rate(container_cpu_usage_seconds_total{namespace=~"encabulator-.*",container!="",container!="POD"}[5m])) * 100'),
        
        // Average memory usage percentage across encabulator pods
        this.queryPrometheus('avg(container_memory_working_set_bytes{namespace=~"encabulator-.*",container!="",container!="POD"} / container_spec_memory_limit_bytes{namespace=~"encabulator-.*",container!="",container!="POD"} * 100)')
      ])

      // If Prometheus is available and returning data, mark it as available
      if (totalRequests > 0 || avgLatency > 0 || cpuUsage > 0) {
        if (!this.prometheusAvailable) {
          console.log('Prometheus metrics now available')
          this.prometheusAvailable = true
        }
      }

      // Return real metrics (even if zero - that's valid when there's no traffic)
      return {
        totalRequests: Math.round(totalRequests) || 0,
        avgLatency: Math.round(avgLatency) || 0,
        errorRate: errorRate || 0,
        cpuUsage: Math.round(cpuUsage * 10) / 10 || 0,
        memoryUsage: Math.round(memoryUsage * 10) / 10 || 0,
      }
    } catch (error) {
      console.error('Failed to fetch system metrics:', error)
      // Return zeros on error (not simulated - we want to show the system is idle/unavailable)
      return {
        totalRequests: 0,
        avgLatency: 0,
        errorRate: 0,
        cpuUsage: 0,
        memoryUsage: 0,
      }
    }
  }
}
