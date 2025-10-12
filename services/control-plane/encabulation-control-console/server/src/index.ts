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
