import express from 'express'
import { createServer } from 'http'
import { Server } from 'socket.io'
import cors from 'cors'
import path from 'path'
import { register } from 'prom-client'
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

// Control Actions API
app.post('/api/actions/synchronize-grammeters', async (req, res) => {
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
    res.json({ success: true, message: 'Cardinal Grammeters synchronized', data })
  } catch (error) {
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
