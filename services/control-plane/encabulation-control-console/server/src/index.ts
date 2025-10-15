import express, { Request, Response, NextFunction } from 'express'
import { createServer } from 'http'
import { Server } from 'socket.io'
import cors from 'cors'
import path from 'path'
import { register, Counter, Histogram } from 'prom-client'
import rateLimit from 'express-rate-limit'
import { KubernetesService } from './services/kubernetes'
import { MetricsService } from './services/metrics'
import { TrafficSimulationService } from './services/traffic'

const app = express()
const httpServer = createServer(app)
const io = new Server(httpServer, {
  cors: {
    origin: process.env.CORS_ORIGIN || '*',
    methods: ['GET', 'POST'],
  },
})

const PORT = process.env.PORT || 8080
const k8sService = new KubernetesService()
const metricsService = new MetricsService()
const trafficService = new TrafficSimulationService()

// Prometheus metrics
const actionCounter = new Counter({
  name: 'control_console_actions_total',
  help: 'Total number of control actions executed',
  labelNames: ['action', 'status']
})

const actionDuration = new Histogram({
  name: 'control_console_action_duration_seconds',
  help: 'Duration of control actions',
  labelNames: ['action']
})

const authRequestCounter = new Counter({
  name: 'control_console_auth_requests_total',
  help: 'Total number of auth requests',
  labelNames: ['endpoint', 'status']
})

app.use(cors())
app.use(express.json())

// Rate limiting for control actions
const actionLimiter = rateLimit({
  windowMs: 60 * 1000, // 1 minute
  max: 10, // 10 requests per minute
  message: { error: 'Too many requests, please try again later' }
})

// Action history storage (in-memory)
interface ActionHistoryItem {
  id: string
  action: string
  user: string
  timestamp: string
  success: boolean
  message: string
}
const actionHistory: ActionHistoryItem[] = []

function addToActionHistory(action: string, user: string, success: boolean, message: string) {
  actionHistory.unshift({
    id: Date.now().toString(),
    action,
    user,
    timestamp: new Date().toISOString(),
    success,
    message
  })
  // Keep last 100 actions
  if (actionHistory.length > 100) {
    actionHistory.pop()
  }
  
  // Update Prometheus metrics
  actionCounter.inc({ action, status: success ? 'success' : 'failure' })
}

// JWT middleware for protected routes
async function authenticateJWT(req: Request, res: Response, next: NextFunction) {
  const authHeader = req.headers.authorization
  
  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return res.status(401).json({ error: 'Missing or invalid authorization header' })
  }
  
  try {
    // Verify token with auth service
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users/me`, {
      headers: { 'Authorization': authHeader }
    })
    
    if (!response.ok) {
      return res.status(401).json({ error: 'Invalid or expired token' })
    }
    
    const user = await response.json() as { username: string; role: string }
    req.headers['x-user'] = user.username
    req.headers['x-user-role'] = user.role
    next()
  } catch (error) {
    console.error('JWT authentication error:', error)
    res.status(500).json({ error: 'Authentication service unavailable' })
  }
}

// Role-based authorization middleware
function requireRole(...roles: string[]) {
  return (req: Request, res: Response, next: NextFunction) => {
    const userRole = req.headers['x-user-role'] as string
    
    if (!userRole || !roles.includes(userRole)) {
      return res.status(403).json({ error: 'Insufficient permissions' })
    }
    
    next()
  }
}

// Proxy to unilateral-phase-detractor for auth
const AUTH_API_BASE = 'http://unilateral-phase-detractor.encabulator-support.svc.cluster.local:8080'

app.post('/api/auth/login', async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req.body)
    })
    const data = await response.json()
    authRequestCounter.inc({ endpoint: 'login', status: response.ok ? 'success' : 'failure' })
    res.status(response.status).json(data)
  } catch (error) {
    authRequestCounter.inc({ endpoint: 'login', status: 'error' })
    console.error('Login error:', error)
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.get('/api/auth/users/me', async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users/me`, {
      headers: { 'Authorization': req.headers.authorization || '' }
    })
    const data = await response.json()
    authRequestCounter.inc({ endpoint: 'users_me', status: response.ok ? 'success' : 'failure' })
    res.status(response.status).json(data)
  } catch (error) {
    authRequestCounter.inc({ endpoint: 'users_me', status: 'error' })
    console.error('Get user error:', error)
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.get('/api/auth/users', authenticateJWT, requireRole('admin'), async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users`, {
      headers: { 'Authorization': req.headers.authorization || '' }
    })
    const data = await response.json()
    authRequestCounter.inc({ endpoint: 'users_list', status: response.ok ? 'success' : 'failure' })
    res.status(response.status).json(data)
  } catch (error) {
    authRequestCounter.inc({ endpoint: 'users_list', status: 'error' })
    console.error('List users error:', error)
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.get('/api/auth/users/:username', async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users/${req.params.username}`, {
      headers: { 'Authorization': req.headers.authorization || '' }
    })
    const data = await response.json()
    res.status(response.status).json(data)
  } catch (error) {
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.post('/api/auth/users', authenticateJWT, requireRole('admin'), async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users`, {
      method: 'POST',
      headers: {
        'Authorization': req.headers.authorization || '',
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(req.body)
    })
    const data = await response.json()
    authRequestCounter.inc({ endpoint: 'users_create', status: response.ok ? 'success' : 'failure' })
    res.status(response.status).json(data)
  } catch (error) {
    authRequestCounter.inc({ endpoint: 'users_create', status: 'error' })
    console.error('Create user error:', error)
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.put('/api/auth/users/:username', authenticateJWT, requireRole('admin'), async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users/${req.params.username}`, {
      method: 'PUT',
      headers: {
        'Authorization': req.headers.authorization || '',
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(req.body)
    })
    const data = await response.json()
    authRequestCounter.inc({ endpoint: 'users_update', status: response.ok ? 'success' : 'failure' })
    res.status(response.status).json(data)
  } catch (error) {
    authRequestCounter.inc({ endpoint: 'users_update', status: 'error' })
    console.error('Update user error:', error)
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.delete('/api/auth/users/:username', authenticateJWT, requireRole('admin'), async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users/${req.params.username}`, {
      method: 'DELETE',
      headers: { 'Authorization': req.headers.authorization || '' }
    })
    authRequestCounter.inc({ endpoint: 'users_delete', status: response.ok ? 'success' : 'failure' })
    if (response.status === 204) {
      res.status(204).send()
    } else {
      const data = await response.json()
      res.status(response.status).json(data)
    }
  } catch (error) {
    authRequestCounter.inc({ endpoint: 'users_delete', status: 'error' })
    console.error('Delete user error:', error)
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.get('/api/auth/users/:username/activity', async (req, res) => {
  try {
    const limit = req.query.limit || '100'
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users/${req.params.username}/activity?limit=${limit}`, {
      headers: { 'Authorization': req.headers.authorization || '' }
    })
    const data = await response.json()
    res.status(response.status).json(data)
  } catch (error) {
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

// Serve static files from the React app
app.use(express.static(path.join(__dirname, '../public')))

// Health endpoint
app.get('/health', (req, res) => {
  res.json({ status: 'healthy', timestamp: new Date().toISOString() })
})

// Readiness endpoint
app.get('/ready', async (req, res) => {
  try {
    await k8sService.checkConnection()
    res.json({ status: 'ready' })
  } catch (error) {
    res.status(503).json({ status: 'not ready', error: String(error) })
  }
})

// Metrics endpoint
app.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType)
  res.end(await register.metrics())
})

// Traffic metrics endpoint
app.get('/api/traffic', (req, res) => {
  try {
    const trafficMetrics = trafficService.getTrafficMetrics()
    res.json({ trafficMetrics, timestamp: new Date().toISOString() })
  } catch (error) {
    res.status(500).json({ error: 'Failed to fetch traffic metrics', message: String(error) })
  }
})

// Traffic mode endpoints
app.get('/api/traffic/mode', (req, res) => {
  try {
    const mode = trafficService.getTrafficMode()
    res.json({ mode, timestamp: new Date().toISOString() })
  } catch (error) {
    res.status(500).json({ error: 'Failed to get traffic mode', message: String(error) })
  }
})

app.post('/api/traffic/mode', (req, res) => {
  try {
    const { mode } = req.body
    
    if (!mode || !['simulated', 'real'].includes(mode)) {
      return res.status(400).json({ error: 'Invalid mode. Must be "simulated" or "real"' })
    }
    
    trafficService.setTrafficMode(mode)
    // Sync metrics mode with traffic mode
    metricsService.setMetricsMode(mode)
    
    res.json({ mode, message: `Traffic mode set to ${mode}`, timestamp: new Date().toISOString() })
  } catch (error) {
    console.error('Set traffic mode error:', error)
    res.status(500).json({ error: 'Failed to set traffic mode', message: String(error) })
  }
})

// Metrics mode endpoints
app.get('/api/metrics/mode', (req, res) => {
  try {
    const mode = metricsService.getMetricsMode()
    res.json({ mode, timestamp: new Date().toISOString() })
  } catch (error) {
    res.status(500).json({ error: 'Failed to get metrics mode', message: String(error) })
  }
})

app.post('/api/metrics/mode', (req, res) => {
  try {
    const { mode } = req.body
    
    if (!mode || !['simulated', 'real'].includes(mode)) {
      return res.status(400).json({ error: 'Invalid mode. Must be "simulated" or "real"' })
    }
    
    metricsService.setMetricsMode(mode)
    res.json({ mode, message: `Metrics mode set to ${mode}`, timestamp: new Date().toISOString() })
  } catch (error) {
    console.error('Set metrics mode error:', error)
    res.status(500).json({ error: 'Failed to set metrics mode', message: String(error) })
  }
})

// Action history endpoint
app.get('/api/actions/history', (req, res) => {
  const limit = parseInt(req.query.limit as string) || 50
  res.json(actionHistory.slice(0, limit))
})

// Grafana proxy endpoint
app.use('/api/grafana', async (req, res) => {
  try {
    const grafanaPath = req.path
    const grafanaUrl = `http://grafana.encabulator-monitoring.svc.cluster.local:3000${grafanaPath}`
    const response = await fetch(grafanaUrl)
    const data = await response.text()
    res.send(data)
  } catch (error) {
    console.error('Grafana proxy error:', error)
    res.status(500).json({ error: 'Grafana service unavailable' })
  }
})

// Encabulations data endpoint
app.get('/api/encabulations', async (req, res) => {
  try {
    const page = req.query.page || '1'
    const pageSize = req.query.pageSize || '10'
    const response = await fetch(`http://hydrocoptic-datastore.encabulator-data.svc.cluster.local:8080/api/v1/encabulations?page=${page}&pageSize=${pageSize}`)
    const data = await response.json()
    res.json(data)
  } catch (error) {
    console.error('Failed to fetch encabulations:', error)
    res.status(500).json({ error: 'Failed to fetch encabulations', encabulations: [], total: 0 })
  }
})

// Cardinal Grammeter proxy endpoints
app.get('/api/cardinal-grammeter/status', async (req, res) => {
  try {
    const response = await fetch('http://cardinal-grammeter.encabulator-control.svc.cluster.local:8080/traffic/status')
    const data = await response.json()
    res.json(data)
  } catch (error) {
    console.error('Failed to get grammeter status:', error)
    res.status(500).json({ error: 'Failed to get traffic status' })
  }
})

app.post('/api/cardinal-grammeter/update', async (req, res) => {
  try {
    const response = await fetch('http://cardinal-grammeter.encabulator-control.svc.cluster.local:8080/traffic/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req.body)
    })
    const data = await response.json()
    res.json(data)
  } catch (error) {
    console.error('Failed to update grammeter:', error)
    res.status(500).json({ error: 'Failed to update traffic parameters' })
  }
})

// Control Actions API (with rate limiting)
app.post('/api/actions/synchronize-grammeters', actionLimiter, async (req, res) => {
  const user = req.headers['x-user'] as string || 'unknown'
  const end = actionDuration.startTimer({ action: 'synchronize-grammeters' })
  
  try {
    // Get parameters from request body or use defaults
    const {
      fluorescentScoreMotion = 50,
      magnetoReluctance = 0,
      capacitiveDiractance = 50,
      pattern = 'steady'
    } = req.body
    
    const response = await fetch('http://cardinal-grammeter.encabulator-control.svc.cluster.local:8080/traffic/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        target: 'http://dingle-arm-api-gateway.encabulator-processing.svc.cluster.local:8080/api/v1/encabulations',
        pattern,
        fluorescentScoreMotion,
        magnetoReluctance,
        capacitiveDiractance
      })
    })
    const data = await response.json()
    
    console.log(`Traffic started: ${fluorescentScoreMotion}% FSM, pattern: ${pattern}`)
    addToActionHistory('synchronize-grammeters', user, true, 'Cardinal Grammeters synchronized')
    end()
    
    // Emit WebSocket event for real-time updates
    io.emit('message', {
      type: 'action_completed',
      payload: {
        action: 'synchronize-grammeters',
        user,
        success: true,
        message: 'Cardinal Grammeters synchronized'
      }
    })
    
    res.json({ success: true, message: 'Cardinal Grammeters synchronized', data })
  } catch (error) {
    addToActionHistory('synchronize-grammeters', user, false, String(error))
    end()
    console.error('Synchronize grammeters error:', error)
    res.status(500).json({ error: 'Failed to synchronize grammeters', message: String(error) })
  }
})

app.post('/api/actions/induce-fumbling', actionLimiter, async (req, res) => {
  const user = req.headers['x-user'] as string || 'unknown'
  const end = actionDuration.startTimer({ action: 'induce-fumbling' })
  
  try {
    const response = await fetch('http://malaise-injection-engine.encabulator-control.svc.cluster.local:8080/inject', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        type: 'latency',
        target: 'panametric-fam-processor',
        duration: 300
      })
    })
    const data = await response.json()
    addToActionHistory('induce-fumbling', user, true, 'Side fumbling induced')
    end()
    res.json({ success: true, message: 'Side fumbling induced', data })
  } catch (error) {
    addToActionHistory('induce-fumbling', user, false, String(error))
    end()
    console.error('Induce fumbling error:', error)
    res.status(500).json({ error: 'Failed to induce fumbling', message: String(error) })
  }
})

app.post('/api/actions/flex-dingle-arm', actionLimiter, async (req, res) => {
  const user = req.headers['x-user'] as string || 'unknown'
  const { direction } = req.body
  const end = actionDuration.startTimer({ action: 'flex-dingle-arm' })
  
  if (!direction || !['up', 'down'].includes(direction)) {
    end()
    return res.status(400).json({ error: 'Direction must be "up" or "down"' })
  }
  
  try {
    await k8sService.scaleService('dingle-arm-api-gateway', direction)
    addToActionHistory('flex-dingle-arm', user, true, `Dingle Arm flexed ${direction}`)
    end()
    res.json({ success: true, message: `Dingle Arm flexed ${direction}` })
  } catch (error) {
    addToActionHistory('flex-dingle-arm', user, false, String(error))
    end()
    console.error('Flex dingle arm error:', error)
    res.status(500).json({ error: 'Failed to flex dingle arm', message: String(error) })
  }
})

app.post('/api/actions/recalibrate-fam', actionLimiter, async (req, res) => {
  const user = req.headers['x-user'] as string || 'unknown'
  const end = actionDuration.startTimer({ action: 'recalibrate-fam' })
  
  try {
    await k8sService.restartService('panametric-fam-processor')
    addToActionHistory('recalibrate-fam', user, true, 'Panametric Fam recalibrated')
    end()
    res.json({ success: true, message: 'Panametric Fam recalibrated' })
  } catch (error) {
    addToActionHistory('recalibrate-fam', user, false, String(error))
    end()
    console.error('Recalibrate fam error:', error)
    res.status(500).json({ error: 'Failed to recalibrate fam', message: String(error) })
  }
})

app.post('/api/actions/engage-dampeners', actionLimiter, async (req, res) => {
  const user = req.headers['x-user'] as string || 'unknown'
  const end = actionDuration.startTimer({ action: 'engage-dampeners' })
  
  try {
    const response = await fetch('http://cardinal-grammeter.encabulator-control.svc.cluster.local:8080/traffic/stop', {
      method: 'POST'
    })
    const data = await response.json()
    addToActionHistory('engage-dampeners', user, true, 'Malaise dampeners engaged')
    end()
    res.json({ success: true, message: 'Malaise dampeners engaged', data })
  } catch (error) {
    addToActionHistory('engage-dampeners', user, false, String(error))
    end()
    console.error('Engage dampeners error:', error)
    res.status(500).json({ error: 'Failed to engage dampeners', message: String(error) })
  }
})

app.post('/api/actions/purge-marzelvanes', actionLimiter, async (req, res) => {
  const user = req.headers['x-user'] as string || 'unknown'
  const end = actionDuration.startTimer({ action: 'purge-marzelvanes' })
  
  try {
    const response = await fetch('http://prefabulated-cache.encabulator-data.svc.cluster.local:8080/cache/clear', {
      method: 'POST'
    })
    const data = await response.json()
    addToActionHistory('purge-marzelvanes', user, true, 'Hydrocoptic marzelvanes purged')
    end()
    res.json({ success: true, message: 'Hydrocoptic marzelvanes purged', data })
  } catch (error) {
    addToActionHistory('purge-marzelvanes', user, false, String(error))
    end()
    console.error('Purge marzelvanes error:', error)
    res.status(500).json({ error: 'Failed to purge marzelvanes', message: String(error) })
  }
})

// WebSocket connection handling
io.on('connection', (socket) => {
  console.log('Client connected:', socket.id)

  // Send initial system status
  k8sService.getSystemStatus().then((status) => {
    const trafficMetrics = trafficService.getTrafficMetrics()
    socket.emit('message', {
      type: 'system_status',
      payload: {
        ...status,
        trafficMetrics,
      },
    })
  })

  // Handle client messages
  socket.on('message', async (data: any) => {
    console.log('Received message:', data)
    const user = (data.user as string) || 'unknown'

    try {
      switch (data.type) {
        case 'scale_service':
          await k8sService.scaleService(
            data.payload.service,
            data.payload.direction
          )
          addToActionHistory('scale_service', user, true, `Scaled ${data.payload.service} ${data.payload.direction}`)
          break
        case 'inject_failure':
          await k8sService.injectFailure(data.payload.service)
          addToActionHistory('inject_failure', user, true, `Injected failure into ${data.payload.service}`)
          break
        case 'run_scenario':
          await k8sService.runScenario(data.payload.scenario)
          addToActionHistory('run_scenario', user, true, `Ran scenario: ${data.payload.scenario}`)
          break
        case 'synchronize_grammeters':
          const gramResponse = await fetch('http://cardinal-grammeter.encabulator-control.svc.cluster.local:8080/traffic/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              target: 'http://dingle-arm-api-gateway.encabulator-processing.svc.cluster.local:8080/api/v1/encabulations',
              pattern: data.payload?.pattern || 'steady',
              fluorescentScoreMotion: data.payload?.fluorescentScoreMotion || 50,
              magnetoReluctance: data.payload?.magnetoReluctance || 0,
              capacitiveDiractance: data.payload?.capacitiveDiractance || 50
            })
          })
          await gramResponse.json()
          console.log(`Traffic started via WebSocket: ${data.payload?.fluorescentScoreMotion || 50}% FSM`)
          addToActionHistory('synchronize_grammeters', user, true, 'Cardinal Grammeters synchronized')
          break
        case 'induce_fumbling':
          const fumblingResponse = await fetch('http://malaise-injection-engine.encabulator-control.svc.cluster.local:8080/inject', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              type: 'latency',
              target: 'panametric-fam-processor',
              duration: 300
            })
          })
          await fumblingResponse.json()
          addToActionHistory('induce_fumbling', user, true, 'Side fumbling induced')
          break
        case 'flex_dingle_arm':
          await k8sService.scaleService('dingle-arm-api-gateway', data.payload.direction)
          addToActionHistory('flex_dingle_arm', user, true, `Dingle Arm flexed ${data.payload.direction}`)
          break
        case 'recalibrate_fam':
          await k8sService.restartService('panametric-fam-processor')
          addToActionHistory('recalibrate_fam', user, true, 'Panametric Fam recalibrated')
          break
        case 'engage_dampeners':
          const dampenerResponse = await fetch('http://cardinal-grammeter.encabulator-control.svc.cluster.local:8080/traffic/stop', {
            method: 'POST'
          })
          await dampenerResponse.json()
          addToActionHistory('engage_dampeners', user, true, 'Malaise dampeners engaged')
          break
        case 'purge_marzelvanes':
          const purgeResponse = await fetch('http://prefabulated-cache.encabulator-data.svc.cluster.local:8080/cache/clear', {
            method: 'POST'
          })
          await purgeResponse.json()
          addToActionHistory('purge_marzelvanes', user, true, 'Hydrocoptic marzelvanes purged')
          break
      }

      // Send updated status
      const status = await k8sService.getSystemStatus()
      io.emit('message', {
        type: 'system_status',
        payload: status,
      })
      
      // Broadcast action completion
      io.emit('message', {
        type: 'action_completed',
        payload: {
          action: data.type,
          user,
          success: true,
          timestamp: new Date().toISOString()
        }
      })
    } catch (error) {
      console.error('WebSocket action error:', error)
      addToActionHistory(data.type, user, false, String(error))
      socket.emit('message', {
        type: 'error',
        payload: { message: String(error) },
      })
    }
  })

  socket.on('disconnect', () => {
    console.log('Client disconnected:', socket.id)
  })
})

// Periodic system status updates
setInterval(async () => {
  try {
    const status = await k8sService.getSystemStatus()
    const metrics = await metricsService.getSystemMetrics()
    
    // Send traffic metrics based on mode
    const trafficMode = trafficService.getTrafficMode()
    let trafficMetrics: any[] = []
    
    if (trafficMode === 'simulated') {
      // Use simulated traffic metrics
      trafficMetrics = trafficService.getTrafficMetrics()
      console.log(`Simulated mode: ${trafficMetrics.length} traffic metrics`)
    } else {
      // Generate traffic metrics from real system metrics for visualization
      // This allows particles to show when there's real traffic
      if (metrics.totalRequests > 0) {
        trafficMetrics = [
          {
            source: 'cardinal-grammeter',
            target: 'dingle-arm-api-gateway',
            requestsPerSecond: metrics.totalRequests,
            avgLatency: metrics.avgLatency,
            p50Latency: metrics.avgLatency * 0.9,
            p99Latency: metrics.avgLatency * 2.5,
            errorRate: metrics.errorRate,
            healthStatus: metrics.errorRate > 0.01 ? 'error' : metrics.avgLatency > 100 ? 'warning' : 'healthy'
          },
          {
            source: 'dingle-arm-api-gateway',
            target: 'panametric-fam-processor',
            requestsPerSecond: metrics.totalRequests,
            avgLatency: metrics.avgLatency,
            p50Latency: metrics.avgLatency * 0.9,
            p99Latency: metrics.avgLatency * 2.5,
            errorRate: metrics.errorRate,
            healthStatus: metrics.errorRate > 0.01 ? 'error' : metrics.avgLatency > 100 ? 'warning' : 'healthy'
          },
          // Panametric Fam writes to PostgreSQL (hydrocoptic-datastore)
          {
            source: 'panametric-fam-processor',
            target: 'hydrocoptic-datastore',
            requestsPerSecond: metrics.totalRequests,
            avgLatency: metrics.avgLatency * 0.5, // DB writes are typically faster
            p50Latency: metrics.avgLatency * 0.4,
            p99Latency: metrics.avgLatency * 1.2,
            errorRate: metrics.errorRate,
            healthStatus: metrics.errorRate > 0.01 ? 'error' : metrics.avgLatency > 100 ? 'warning' : 'healthy'
          },
          // Panametric Fam checks cache (prefabulated-cache)
          {
            source: 'panametric-fam-processor',
            target: 'prefabulated-cache',
            requestsPerSecond: metrics.totalRequests * 1.5, // Cache checks happen more frequently
            avgLatency: metrics.avgLatency * 0.1, // Cache is very fast
            p50Latency: metrics.avgLatency * 0.05,
            p99Latency: metrics.avgLatency * 0.3,
            errorRate: metrics.errorRate * 0.5, // Cache errors are less common
            healthStatus: 'healthy'
          },
          // Panametric Fam writes metrics (sinusoidal-database)
          {
            source: 'panametric-fam-processor',
            target: 'sinusoidal-database',
            requestsPerSecond: metrics.totalRequests * 0.8, // Not every request writes metrics
            avgLatency: metrics.avgLatency * 0.3, // Time-series writes are fast
            p50Latency: metrics.avgLatency * 0.2,
            p99Latency: metrics.avgLatency * 0.8,
            errorRate: metrics.errorRate * 0.3,
            healthStatus: 'healthy'
          }
        ]
        console.log(`Real mode: Generated ${trafficMetrics.length} traffic metrics from TPS=${metrics.totalRequests}`)
      } else {
        console.log(`Real mode: No traffic (TPS=0)`)
      }
    }
    
    io.emit('message', {
      type: 'system_status',
      payload: {
        ...status,
        metrics,
        trafficMetrics,
      },
    })
  } catch (error) {
    console.error('Error fetching system status:', error)
  }
}, 2000) // Poll every 2 seconds for more responsive UI

httpServer.listen(PORT, () => {
  console.log(`Control Console server listening on port ${PORT}`)
})
