import axios, { AxiosInstance } from 'axios'

export interface TrafficConfig {
  target: string
  pattern: 'steady' | 'burst' | 'ramp' | 'sine'
  fluorescentScoreMotion: number // 0-100: controls TPS (throughput)
  magnetoReluctance: number // 0-100: controls latency injection and jitter
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

  updateParameters(params: Partial<Pick<TrafficConfig, 'fluorescentScoreMotion' | 'magnetoReluctance' | 'capacitiveDiractance'>>): void {
    if (this.config) {
      this.config = { ...this.config, ...params }
      // Reschedule if TPS changed
      if (params.fluorescentScoreMotion !== undefined && this.active) {
        if (this.intervalId) {
          clearInterval(this.intervalId)
        }
        this.scheduleRequests()
      }
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
    const requestsPerInterval = this.calculateRequestsPerInterval()
    
    this.intervalId = setInterval(() => {
      if (!this.active) return
      
      // For high TPS, spawn multiple requests per interval
      for (let i = 0; i < requestsPerInterval; i++) {
        this.sendRequest()
      }
    }, interval)
  }

  private calculateInterval(): number {
    // Use 100ms intervals for batching
    return 100
  }

  private calculateRequestsPerInterval(): number {
    if (!this.config) return 1

    // Fluorescent Score Motion controls TPS (throughput)
    // Range: 0.5 TPS to 1000 TPS
    const minTPS = 0.5
    const maxTPS = 1000
    const fsm = this.config.fluorescentScoreMotion || 50
    
    // Exponential scaling for better control across the range
    const normalizedFSM = fsm / 100
    const targetTPS = minTPS + (maxTPS - minTPS) * Math.pow(normalizedFSM, 2)
    
    // Calculate requests per 100ms interval
    const requestsPerInterval = Math.max(1, Math.round((targetTPS * 100) / 1000))
    
    return requestsPerInterval
  }

  private async injectLatency(): Promise<void> {
    if (!this.config || this.config.magnetoReluctance === 0) return

    // Magneto Reluctance controls latency injection (0-500ms base + jitter)
    const baseLatency = (this.config.magnetoReluctance / 100) * 500
    
    // Add jitter (±30% of base latency)
    const jitter = baseLatency * 0.3 * (Math.random() * 2 - 1)
    const totalLatency = Math.max(0, baseLatency + jitter)
    
    if (totalLatency > 0) {
      await new Promise(resolve => setTimeout(resolve, totalLatency))
    }
  }

  private async sendRequest(): Promise<void> {
    if (!this.config) return

    // Inject latency BEFORE sending request (Magneto Reluctance)
    await this.injectLatency()

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
