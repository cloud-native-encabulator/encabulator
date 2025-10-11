export interface PodInfo {
  name: string
  status: string
  ready: boolean
  cpu: string
  memory: string
  p50Latency?: number
  p99Latency?: number
  errorRate?: number
}

export interface Service {
  status: 'healthy' | 'degraded' | 'unhealthy' | 'unknown'
  replicas: number
  desiredReplicas?: number
  cpu?: string
  memory?: string
  requestRate?: number
  pods?: PodInfo[]
}

export interface SystemMetrics {
  totalRequests: number
  avgLatency: number
  errorRate: number
  cpuUsage: number
  memoryUsage: number
}

export interface User {
  id: string
  username: string
  email: string
  role: 'admin' | 'operator' | 'viewer' | 'guest'
  createdAt?: Date
  lastLogin?: Date
}

export interface ActionHistoryItem {
  id: string
  type: string
  description: string
  timestamp: Date
  status: 'pending' | 'success' | 'error'
  user?: string
  details?: Record<string, any>
}

export type ServicePlane = 'data' | 'processing' | 'support' | 'control'

export interface ServiceNode {
  id: string
  name: string
  plane: ServicePlane
  status: Service['status']
  replicas: number
  x?: number
  y?: number
  vx?: number
  vy?: number
}

export interface ServiceLink {
  source: string | ServiceNode
  target: string | ServiceNode
  value: number
}
