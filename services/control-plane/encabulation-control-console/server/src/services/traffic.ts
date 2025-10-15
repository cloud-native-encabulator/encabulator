interface TrafficMetrics {
  source: string
  target: string
  requestsPerSecond: number
  avgLatency: number
  p50Latency: number
  p99Latency: number
  errorRate: number
  healthStatus: 'healthy' | 'warning' | 'error' | 'unknown'
}

interface ServiceLink {
  source: string
  target: string
  value: number
}

export type TrafficMode = 'simulated' | 'real'

export class TrafficSimulationService {
  private serviceLinks: ServiceLink[] = []
  private baselineMetrics: Map<string, TrafficMetrics> = new Map()
  private simulationEnabled: boolean = false
  private trafficMode: TrafficMode = 'real'

  constructor() {
    this.initializeServiceTopology()
  }

  /**
   * Get current traffic mode
   */
  public getTrafficMode(): TrafficMode {
    return this.trafficMode
  }

  /**
   * Set traffic mode
   */
  public setTrafficMode(mode: TrafficMode): void {
    this.trafficMode = mode
    // Enable/disable simulation based on mode
    this.simulationEnabled = (mode === 'simulated')
    console.log(`Traffic mode set to: ${mode}, simulation enabled: ${this.simulationEnabled}`)
  }

  private initializeServiceTopology(): void {
    // Define the service topology based on the architecture
    this.serviceLinks = [
      // Traffic Generation: Cardinal Grammeter → API Gateway
      { source: 'cardinal-grammeter', target: 'dingle-arm-api-gateway', value: 3 },
      
      // API Gateway → Auth & Processing
      { source: 'dingle-arm-api-gateway', target: 'unilateral-phase-detractor', value: 2 },
      { source: 'dingle-arm-api-gateway', target: 'panametric-fam-processor', value: 3 },
      
      // Processing → Data Stores
      { source: 'panametric-fam-processor', target: 'hydrocoptic-datastore', value: 2 },
      { source: 'panametric-fam-processor', target: 'prefabulated-cache', value: 2 },
      { source: 'panametric-fam-processor', target: 'sinusoidal-database', value: 2 },
      
      // Processing → Message Broker
      { source: 'panametric-fam-processor', target: 'logarithmic-casing-service', value: 2 },
      { source: 'logarithmic-casing-service', target: 'panametric-fam-processor', value: 1 },
      
      // Configuration → Services
      { source: 'differential-girdle', target: 'panametric-fam-processor', value: 1 },
      { source: 'differential-girdle', target: 'dingle-arm-api-gateway', value: 1 },
      { source: 'differential-girdle', target: 'logarithmic-casing-service', value: 1 },
      
      // Resource Management → Data Stores
      { source: 'side-fumbling-collector', target: 'hydrocoptic-datastore', value: 1 },
      { source: 'side-fumbling-collector', target: 'sinusoidal-database', value: 1 },
      { source: 'side-fumbling-collector', target: 'prefabulated-cache', value: 1 },
      
      // Chaos Engineering → Services
      { source: 'malaise-injection-engine', target: 'panametric-fam-processor', value: 1 },
      { source: 'malaise-injection-engine', target: 'hydrocoptic-datastore', value: 1 },
      { source: 'malaise-injection-engine', target: 'prefabulated-cache', value: 1 },
      
      // Control Console → Monitoring
      { source: 'encabulation-control-console', target: 'dingle-arm-api-gateway', value: 1 },
      { source: 'encabulation-control-console', target: 'panametric-fam-processor', value: 1 },
      
      // Demo Orchestrator → Control
      { source: 'demo-scenario-orchestrator', target: 'encabulation-control-console', value: 1 },
      { source: 'demo-scenario-orchestrator', target: 'cardinal-grammeter', value: 1 },
      { source: 'demo-scenario-orchestrator', target: 'malaise-injection-engine', value: 1 }
    ]
  }

  /**
   * Generate simulated traffic metrics for all service connections
   */
  public getTrafficMetrics(): TrafficMetrics[] {
    if (!this.simulationEnabled) {
      return []
    }

    const metrics: TrafficMetrics[] = []
    const now = Date.now()

    for (const link of this.serviceLinks) {
      const linkKey = `${link.source}->${link.target}`
      
      // Generate realistic metrics based on link value (traffic volume)
      const baseRps = link.value * 10 // Base requests per second
      const variation = Math.sin(now / 10000) * 0.3 + 1 // Sine wave variation ±30%
      const requestsPerSecond = Math.max(0, baseRps * variation + this.randomVariation(-2, 2))

      // Latency varies by service type
      const baseLatency = this.getBaseLatency(link.target)
      const latencyVariation = this.randomVariation(0.8, 1.2)
      const avgLatency = baseLatency * latencyVariation
      const p50Latency = avgLatency * 0.9
      const p99Latency = avgLatency * 2.5

      // Error rate - mostly healthy with occasional spikes
      const baseErrorRate = 0.001 // 0.1% base error rate
      const errorSpike = Math.random() < 0.05 ? this.randomVariation(0, 0.02) : 0
      const errorRate = Math.min(0.05, baseErrorRate + errorSpike)

      // Determine health status based on latency and error rate
      const healthStatus = this.determineHealthStatus(avgLatency, errorRate)

      metrics.push({
        source: link.source,
        target: link.target,
        requestsPerSecond: Math.round(requestsPerSecond * 10) / 10,
        avgLatency: Math.round(avgLatency),
        p50Latency: Math.round(p50Latency),
        p99Latency: Math.round(p99Latency),
        errorRate: Math.round(errorRate * 10000) / 10000,
        healthStatus
      })
    }

    return metrics
  }

  /**
   * Get baseline latency for a service based on its type
   */
  private getBaseLatency(serviceName: string): number {
    // Data stores have higher latency
    if (serviceName.includes('datastore') || serviceName.includes('database')) {
      return 50 + Math.random() * 30 // 50-80ms
    }
    
    // Cache is fast
    if (serviceName.includes('cache')) {
      return 5 + Math.random() * 10 // 5-15ms
    }
    
    // Processing services
    if (serviceName.includes('processor') || serviceName.includes('gateway')) {
      return 20 + Math.random() * 20 // 20-40ms
    }
    
    // Support services
    if (serviceName.includes('detractor') || serviceName.includes('girdle') || serviceName.includes('collector')) {
      return 15 + Math.random() * 15 // 15-30ms
    }
    
    // Control plane services
    if (serviceName.includes('console') || serviceName.includes('engine') || serviceName.includes('orchestrator')) {
      return 10 + Math.random() * 10 // 10-20ms
    }
    
    // Default
    return 25 + Math.random() * 25 // 25-50ms
  }

  /**
   * Determine health status based on latency and error rate
   */
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

  /**
   * Generate random variation within a range
   */
  private randomVariation(min: number, max: number): number {
    return min + Math.random() * (max - min)
  }

  /**
   * Enable or disable traffic simulation
   */
  public setSimulationEnabled(enabled: boolean): void {
    this.simulationEnabled = enabled
  }

  /**
   * Check if simulation is enabled
   */
  public isSimulationEnabled(): boolean {
    return this.simulationEnabled
  }
}
