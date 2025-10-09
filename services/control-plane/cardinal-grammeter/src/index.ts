import express from 'express'
import { register } from 'prom-client'
import { TrafficGenerator } from './traffic-generator'
import { MetricsCollector } from './metrics'

const app = express()
const PORT = process.env.PORT || 8080

app.use(express.json())

const trafficGenerator = new TrafficGenerator()
const metricsCollector = new MetricsCollector()

// Health endpoint
app.get('/health', (req, res) => {
  res.json({ status: 'healthy', timestamp: new Date().toISOString() })
})

// Readiness endpoint
app.get('/ready', (req, res) => {
  res.json({ status: 'ready' })
})

// Metrics endpoint
app.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType)
  res.end(await register.metrics())
})

// Start traffic generation
app.post('/traffic/start', async (req, res) => {
  const { target, pattern, magnetoReluctance, capacitiveDiractance } = req.body

  if (!target) {
    return res.status(400).json({ error: 'Target URL is required' })
  }

  try {
    await trafficGenerator.start({
      target,
      pattern: pattern || 'steady',
      magnetoReluctance: magnetoReluctance || 50,
      capacitiveDiractance: capacitiveDiractance || 50,
    })

    metricsCollector.recordTrafficStart(pattern)

    res.json({
      status: 'started',
      message: `Traffic generation started for ${target}`,
    })
  } catch (error) {
    res.status(500).json({ error: String(error) })
  }
})

// Stop traffic generation
app.post('/traffic/stop', (req, res) => {
  trafficGenerator.stop()
  metricsCollector.recordTrafficStop()
  
  res.json({
    status: 'stopped',
    message: 'Traffic generation stopped',
  })
})

// Get traffic status
app.get('/traffic/status', (req, res) => {
  const status = trafficGenerator.getStatus()
  res.json(status)
})

// Update traffic parameters
app.post('/traffic/update', (req, res) => {
  const { magnetoReluctance, capacitiveDiractance } = req.body

  trafficGenerator.updateParameters({
    magnetoReluctance,
    capacitiveDiractance,
  })

  res.json({
    status: 'updated',
    message: 'Traffic parameters updated',
  })
})

app.listen(PORT, () => {
  console.log(`Cardinal Grammeter listening on port ${PORT}`)
})
