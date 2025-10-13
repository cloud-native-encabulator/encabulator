import axios, { AxiosInstance } from 'axios'

export interface TrafficConfig {
  target: string
  pattern: 'steady' | 'burst' | 'ramp' | 'sine'
  magnetoReluctance: number // 0-100: controls request frequency
  capacitiveDiractance: number // 0-100: controls request complexity
}

export interface TrafficStatus {
  active: boolean
  target?: string
  pattern?: string
  requestsSent: number
  successRate: number
  avgLatency: number
}

export class TrafficGenerator {
  private config?: TrafficConfig
  private active = false
  private intervalId?: NodeJS.Timeout
  private client: AxiosInstance
  private stats = {
    requestsSent: 0,
    requestsSucceeded: 0,
    totalLatency: 0,
  }

  constructor() {
    this.client = axios.create({
      timeout: 5000,
      validateStatus: () => true, // Accept all status codes
    })
  }

  async start(config: TrafficConfig): Promise<void> {
    if (this.active) {
      throw new Error('Traffic generation already active')
    }

    this.config = config
    this.active = true
    this.stats = { requestsSent: 0, requestsSucceeded: 0, totalLatency: 0 }

    this.scheduleRequests()
  }

  stop(): void {
    this.active = false
    if (this.intervalId) {
      clearInterval(this.intervalId)
      this.intervalId = undefined
    }
  }

  updateParameters(params: Partial<Pick<TrafficConfig, 'magnetoReluctance' | 'capacitiveDiractance'>>): void {
    if (this.config) {
      this.config = { ...this.config, ...params }
    }
  }

  getStatus(): TrafficStatus {
    const successRate = this.stats.requestsSent > 0
      ? (this.stats.requestsSucceeded / this.stats.requestsSent) * 100
      : 0

    const avgLatency = this.stats.requestsSucceeded > 0
      ? this.stats.totalLatency / this.stats.requestsSucceeded
      : 0

    return {
      active: this.active,
      target: this.config?.target,
      pattern: this.config?.pattern,
      requestsSent: this.stats.requestsSent,
      successRate,
      avgLatency,
    }
  }

  private scheduleRequests(): void {
    if (!this.config) return

    const interval = this.calculateInterval()
    
    this.intervalId = setInterval(() => {
      if (!this.active) return
      this.sendRequest()
    }, interval)
  }

  private calculateInterval(): number {
    if (!this.config) return 1000

    // Magneto reluctance controls frequency
    // Low reluctance = high frequency (short interval)
    // High reluctance = low frequency (long interval)
    const baseInterval = 1000
    const reluctanceFactor = (100 - this.config.magnetoReluctance) / 100
    return baseInterval * (0.1 + reluctanceFactor * 2)
  }

  private async sendRequest(): Promise<void> {
    if (!this.config) return

    const startTime = Date.now()
    this.stats.requestsSent++

    try {
      // Capacitive diractance controls request complexity
      const payload = this.generatePayload(this.config.capacitiveDiractance)

      const response = await this.client.post(this.config.target, payload)
      
      if (response.status >= 200 && response.status < 300) {
        this.stats.requestsSucceeded++
      }

      const latency = Date.now() - startTime
      this.stats.totalLatency += latency

    } catch (error) {
      // Request failed, but we continue
      console.error('Request failed:', error)
    }
  }

  private generatePayload(complexity: number): any {
    // Generate encabulation request payload
    // Higher complexity = more grammeters and additional metadata
    const grammeters = Math.floor(Math.random() * 100) + 1
    const prefabulation = Math.random() > 0.5
    const sinusoidalMode = ['STANDARD', 'INVERSE', 'LOGARITHMIC'][Math.floor(Math.random() * 3)]

    const payload: any = {
      grammeters,
      prefabulation,
      sinusoidalMode,
    }

    // Add metadata for higher complexity
    if (complexity > 60) {
      payload.metadata = {
        source: 'cardinal-grammeter',
        timestamp: new Date().toISOString(),
        requestId: `req-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
        correlationId: `corr-${Date.now()}`,
      }
    }

    return payload
  }
}
