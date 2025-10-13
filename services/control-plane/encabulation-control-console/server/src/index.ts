import express from 'express'
import { createServer } from 'http'
import { Server } from 'socket.io'
import cors from 'cors'
import path from 'path'
import { register } from 'prom-client'
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
    res.status(response.status).json(data)
  } catch (error) {
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.get('/api/auth/users/me', async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users/me`, {
      headers: { 'Authorization': req.headers.authorization || '' }
    })
    const data = await response.json()
    res.status(response.status).json(data)
  } catch (error) {
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.get('/api/auth/users', async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users`, {
      headers: { 'Authorization': req.headers.authorization || '' }
    })
    const data = await response.json()
    res.status(response.status).json(data)
  } catch (error) {
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

app.post('/api/auth/users', async (req, res) => {
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
    res.status(response.status).json(data)
  } catch (error) {
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.put('/api/auth/users/:username', async (req, res) => {
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
    res.status(response.status).json(data)
  } catch (error) {
    res.status(500).json({ error: 'Auth service unavailable' })
  }
})

app.delete('/api/auth/users/:username', async (req, res) => {
  try {
    const response = await fetch(`${AUTH_API_BASE}/api/v1/users/${req.params.username}`, {
      method: 'DELETE',
      headers: { 'Authorization': req.headers.authorization || '' }
    })
    if (response.status === 204) {
      res.status(204).send()
    } else {
      const data = await response.json()
      res.status(response.status).json(data)
    }
  } catch (error) {
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

// Action history endpoint
app.get('/api/actions/history', (req, res) => {
  const limit = parseInt(req.query.limit as string) || 50
  res.json(actionHistory.slice(0, limit))
})

// Grafana proxy endpoint
app.get('/api/grafana/*', async (req, res) => {
  try {
    const grafanaPath = req.path.replace('/api/grafana/', '')
    const grafanaUrl = `http://grafana.encabulator-monitoring.svc.cluster.local:3000/${grafanaPath}`
    const response = await fetch(grafanaUrl)
    const data = await response.text()
    res.send(data)
  } catch (error) {
    res.status(500).json({ error: 'Grafana service unavailable' })
  }
})

// Control Actions API (with rate limiting)
app.post('/api/actions/synchronize-grammeters', actionLimiter, async (req, res) => {
  const user = req.headers['x-user'] as string || 'unknown'
  try {
    const response = await fetch('http://cardinal-grammeter.encabulator-control.svc.cluster.local:8080/traffic/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        target: 'http://dingle-arm-api-gateway.encabulator-processing.svc.cluster.local:8080',
        pattern: 'steady',
        magnetoReluctance: 50,
        capacitiveDiractance: 50
      })
    })
    const data = await response.json()
    addToActionHistory('synchronize-grammeters', user, true, 'Cardinal Grammeters synchronized')
    res.json({ success: true, message: 'Cardinal Grammeters synchronized', data })
  } catch (error) {
    addToActionHistory('synchronize-grammeters', user, false, String(error))
    res.status(500).json({ error: 'Failed to synchronize grammeters', message: String(error) })
  }
})

app.post('/api/actions/induce-fumbling', async (req, res) => {
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
    res.json({ success: true, message: 'Side fumbling induced', data })
  } catch (error) {
    res.status(500).json({ error: 'Failed to induce fumbling', message: String(error) })
  }
})

app.post('/api/actions/flex-dingle-arm', async (req, res) => {
  const { direction } = req.body
  if (!direction || !['up', 'down'].includes(direction)) {
    return res.status(400).json({ error: 'Direction must be "up" or "down"' })
  }
  
  try {
    await k8sService.scaleService('dingle-arm-api-gateway', direction)
    res.json({ success: true, message: `Dingle Arm flexed ${direction}` })
  } catch (error) {
    res.status(500).json({ error: 'Failed to flex dingle arm', message: String(error) })
  }
})

app.post('/api/actions/recalibrate-fam', async (req, res) => {
  try {
    await k8sService.restartService('panametric-fam-processor')
    res.json({ success: true, message: 'Panametric Fam recalibrated' })
  } catch (error) {
    res.status(500).json({ error: 'Failed to recalibrate fam', message: String(error) })
  }
})

app.post('/api/actions/engage-dampeners', async (req, res) => {
  try {
    const response = await fetch('http://cardinal-grammeter.encabulator-control.svc.cluster.local:8080/traffic/stop', {
      method: 'POST'
    })
    const data = await response.json()
    res.json({ success: true, message: 'Malaise dampeners engaged', data })
  } catch (error) {
    res.status(500).json({ error: 'Failed to engage dampeners', message: String(error) })
  }
})

app.post('/api/actions/purge-marzelvanes', async (req, res) => {
  try {
    const response = await fetch('http://prefabulated-cache.encabulator-data.svc.cluster.local:8080/cache/clear', {
      method: 'POST'
    })
    const data = await response.json()
    res.json({ success: true, message: 'Hydrocoptic marzelvanes purged', data })
  } catch (error) {
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
  socket.on('message', async (data) => {
    console.log('Received message:', data)

    try {
      switch (data.type) {
        case 'scale_service':
          await k8sService.scaleService(
            data.payload.service,
            data.payload.direction
          )
          break
        case 'inject_failure':
          await k8sService.injectFailure(data.payload.service)
          break
        case 'run_scenario':
          await k8sService.runScenario(data.payload.scenario)
          break
      }

      // Send updated status
      const status = await k8sService.getSystemStatus()
      socket.emit('message', {
        type: 'system_status',
        payload: status,
      })
    } catch (error) {
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
    const trafficMetrics = trafficService.getTrafficMetrics()
    
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
}, 5000)

httpServer.listen(PORT, () => {
  console.log(`Control Console server listening on port ${PORT}`)
})
