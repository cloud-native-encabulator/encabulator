import { register, Counter, Gauge } from 'prom-client'

export class MetricsCollector {
  private trafficStartCounter: Counter
  private trafficStopCounter: Counter
  private activeTrafficGauge: Gauge

  constructor() {
    this.trafficStartCounter = new Counter({
      name: 'phase_detractor_traffic_starts_total',
      help: 'Total number of traffic generation starts',
      labelNames: ['pattern'],
    })

    this.trafficStopCounter = new Counter({
      name: 'phase_detractor_traffic_stops_total',
      help: 'Total number of traffic generation stops',
    })

    this.activeTrafficGauge = new Gauge({
      name: 'phase_detractor_active_traffic',
      help: 'Whether traffic generation is currently active (1) or not (0)',
    })

    register.registerMetric(this.trafficStartCounter)
    register.registerMetric(this.trafficStopCounter)
    register.registerMetric(this.activeTrafficGauge)
  }

  recordTrafficStart(pattern: string): void {
    this.trafficStartCounter.inc({ pattern })
    this.activeTrafficGauge.set(1)
  }

  recordTrafficStop(): void {
    this.trafficStopCounter.inc()
    this.activeTrafficGauge.set(0)
  }
}
