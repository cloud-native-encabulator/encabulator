<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import * as d3 from 'd3'
import { useSystemStore } from '@/stores/system'
import type { ServiceNode, ServiceLink } from '@/types'

const systemStore = useSystemStore()
const svgRef = ref<SVGSVGElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const selectedNode = ref<ServiceNode | null>(null)
const hoveredNode = ref<ServiceNode | null>(null)
const tooltipPosition = ref({ x: 0, y: 0 })
const tooltipVisible = ref(false)
const tooltipFlipped = ref(false)

// View toggles
const showTraffic = ref(false)
const showRoutes = ref(false)

// Pod tooltip state
const hoveredPod = ref<any>(null)
const podTooltipPosition = ref({ x: 0, y: 0 })
const podTooltipVisible = ref(false)
const podTooltipFlipped = ref(false)

// Connection tooltip state
const hoveredConnection = ref<{ source: string, target: string } | null>(null)
const connectionTooltipPosition = ref({ x: 0, y: 0 })
const connectionTooltipVisible = ref(false)
const connectionTooltipFlipped = ref(false)

// Get traffic metrics for hovered connection
const hoveredConnectionMetrics = computed(() => {
  if (!hoveredConnection.value) return null
  return systemStore.trafficMetrics.find(
    m => m.source === hoveredConnection.value!.source && m.target === hoveredConnection.value!.target
  )
})

// Particle animation state
interface Particle {
  id: number
  sourceId: string
  targetId: string
  progress: number
  speed: number
  color: string
}

const particles = ref<Particle[]>([])
let particleIdCounter = 0
let animationFrameId: number | null = null
let particleSpawnInterval: number | null = null

// Performance monitoring
let lastFrameTime = performance.now()
let frameCount = 0
let fps = 60
const fpsHistory: number[] = []
const MAX_FPS_HISTORY = 10
let particleSpawnRate = 1.0 // Multiplier for particle spawning (0.0 to 1.0)

// Zoom transform tracking for canvas particles
let currentTransform = { k: 1, x: 0, y: 0 }

// Service plane definitions
const servicePlanes = {
  data: ['hydrocoptic-datastore', 'sinusoidal-database', 'prefabulated-cache'],
  processing: ['panametric-fam-processor', 'logarithmic-casing-service', 'dingle-arm-api-gateway'],
  support: ['unilateral-phase-detractor', 'differential-girdle', 'side-fumbling-collector'],
  control: ['encabulation-control-console', 'malaise-injection-engine', 'demo-scenario-orchestrator', 'cardinal-grammeter']
}

const nodes = computed<ServiceNode[]>(() => {
  const result: ServiceNode[] = []
  
  Object.entries(servicePlanes).forEach(([plane, services]) => {
    services.forEach(serviceName => {
      const service = systemStore.services[serviceName]
      result.push({
        id: serviceName,
        name: serviceName,
        plane: plane as any,
        status: service?.status || 'unknown',
        replicas: service?.replicas || 0
      })
    })
  })
  
  return result
})

const links = computed<ServiceLink[]>(() => {
  // Define actual service relationships based on architecture
  return [
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
})

const statusColors = {
  healthy: '#00ff88',
  degraded: '#ffaa00',
  unhealthy: '#ff0055',
  unknown: '#666666'
}

const planeColors = {
  data: '#00d9ff',      // cyan
  processing: '#ff00ff', // magenta
  support: '#ffaa00',    // yellow
  control: '#8b5cf6'     // purple (distinct from green status)
}

// Helper function to calculate pod positions in a grid
function calculatePodPositions(podCount: number): { x: number, y: number }[] {
  const positions: { x: number, y: number }[] = []
  const spacing = 10
  
  if (podCount === 1) {
    positions.push({ x: 0, y: 0 })
  } else if (podCount === 2) {
    positions.push({ x: -spacing / 2, y: 0 })
    positions.push({ x: spacing / 2, y: 0 })
  } else if (podCount === 3) {
    positions.push({ x: -spacing, y: 0 })
    positions.push({ x: 0, y: 0 })
    positions.push({ x: spacing, y: 0 })
  } else if (podCount === 4) {
    positions.push({ x: -spacing / 2, y: -spacing / 2 })
    positions.push({ x: spacing / 2, y: -spacing / 2 })
    positions.push({ x: -spacing / 2, y: spacing / 2 })
    positions.push({ x: spacing / 2, y: spacing / 2 })
  } else if (podCount === 5) {
    positions.push({ x: -spacing, y: -spacing / 2 })
    positions.push({ x: 0, y: -spacing / 2 })
    positions.push({ x: spacing, y: -spacing / 2 })
    positions.push({ x: -spacing / 2, y: spacing / 2 })
    positions.push({ x: spacing / 2, y: spacing / 2 })
  }
  
  return positions
}

// Helper function to determine pod color based on status
function getPodColor(pod: any, _plane: string): string {
  // Failed pods are red
  if (pod.status === 'Failed') {
    return '#ff0055'
  }
  // Not ready pods are gray
  if (!pod.ready) {
    return '#666666'
  }
  // Running and ready pods are green
  if (pod.status === 'Running' && pod.ready) {
    return '#00ff88'
  }
  // Default to gray for other states
  return '#666666'
}

// Particle animation functions
const nodePositions = new Map<string, { x: number, y: number }>()

/**
 * Determine particle color based on traffic health metrics
 */
function getParticleColor(sourceId: string, targetId: string): string {
  // Find traffic metrics for this connection
  const metrics = systemStore.trafficMetrics.find(
    m => m.source === sourceId && m.target === targetId
  )
  
  if (!metrics) {
    // No metrics available - gray
    return '#666666'
  }
  
  // Determine color based on health status
  switch (metrics.healthStatus) {
    case 'healthy':
      return '#00ff88' // Green
    case 'warning':
      return '#ffaa00' // Yellow
    case 'error':
      return '#ff0055' // Red
    default:
      return '#666666' // Gray (unknown)
  }
}

function spawnParticle(sourceId: string, targetId: string, color?: string) {
  // Cap at 200 particles for performance (increased to support higher TPS visualization)
  if (particles.value.length >= 200) return
  
  // Determine color based on traffic health if not explicitly provided
  const particleColor = color || getParticleColor(sourceId, targetId)
  
  particles.value.push({
    id: particleIdCounter++,
    sourceId,
    targetId,
    progress: 0,
    speed: 0.005 + Math.random() * 0.005, // Random speed between 0.005 and 0.01
    color: particleColor
  })
}

function getPointOnCurve(sourcePos: { x: number, y: number }, targetPos: { x: number, y: number }, t: number): { x: number, y: number } {
  // Calculate curved path (quadratic bezier)
  const dx = targetPos.x - sourcePos.x
  const dy = targetPos.y - sourcePos.y
  const dr = Math.sqrt(dx * dx + dy * dy) * 0.5
  
  // Control point for the curve
  const midX = (sourcePos.x + targetPos.x) / 2
  const midY = (sourcePos.y + targetPos.y) / 2
  const offsetX = -dy / Math.sqrt(dx * dx + dy * dy) * dr
  const offsetY = dx / Math.sqrt(dx * dx + dy * dy) * dr
  const controlX = midX + offsetX
  const controlY = midY + offsetY
  
  // Quadratic bezier formula
  const x = (1 - t) * (1 - t) * sourcePos.x + 2 * (1 - t) * t * controlX + t * t * targetPos.x
  const y = (1 - t) * (1 - t) * sourcePos.y + 2 * (1 - t) * t * controlY + t * t * targetPos.y
  
  return { x, y }
}

function animateParticles() {
  if (!canvasRef.value) return
  
  // Check if page is visible (Page Visibility API)
  if (document.hidden) {
    // Pause animation when tab is hidden
    animationFrameId = requestAnimationFrame(animateParticles)
    return
  }
  
  const canvas = canvasRef.value
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  
  // Monitor frame rate
  const currentTime = performance.now()
  const deltaTime = currentTime - lastFrameTime
  lastFrameTime = currentTime
  
  frameCount++
  if (frameCount % 10 === 0) {
    // Calculate FPS every 10 frames
    const currentFps = 1000 / deltaTime
    fpsHistory.push(currentFps)
    if (fpsHistory.length > MAX_FPS_HISTORY) {
      fpsHistory.shift()
    }
    fps = fpsHistory.reduce((a, b) => a + b, 0) / fpsHistory.length
    
    // Adaptive particle reduction if FPS drops below 45
    if (fps < 45 && particleSpawnRate > 0.3) {
      particleSpawnRate = Math.max(0.3, particleSpawnRate - 0.1)
      console.log(`FPS: ${fps.toFixed(1)} - Reducing particle spawn rate to ${(particleSpawnRate * 100).toFixed(0)}%`)
    } else if (fps > 55 && particleSpawnRate < 1.0) {
      particleSpawnRate = Math.min(1.0, particleSpawnRate + 0.05)
    }
  }
  
  // Clear canvas
  ctx.clearRect(0, 0, canvas.width, canvas.height)
  
  // Update and draw particles
  particles.value = particles.value.filter(particle => {
    // Update progress
    particle.progress += particle.speed
    
    // Remove if reached target
    if (particle.progress >= 1) return false
    
    // Get positions
    const sourcePos = nodePositions.get(particle.sourceId)
    const targetPos = nodePositions.get(particle.targetId)
    
    if (!sourcePos || !targetPos) return false
    
    // Calculate position on curve
    const pos = getPointOnCurve(sourcePos, targetPos, particle.progress)
    
    // Apply zoom transform to particle position
    const transformedX = pos.x * currentTransform.k + currentTransform.x
    const transformedY = pos.y * currentTransform.k + currentTransform.y
    
    // Draw particle
    ctx.beginPath()
    ctx.arc(transformedX, transformedY, 3 * currentTransform.k, 0, Math.PI * 2)
    ctx.fillStyle = particle.color
    ctx.fill()
    
    return true
  })
  
  // Continue animation if traffic is enabled OR there are still particles in flight
  if (showTraffic.value || particles.value.length > 0) {
    animationFrameId = requestAnimationFrame(animateParticles)
  } else {
    animationFrameId = null
  }
}

function startParticleAnimation() {
  if (animationFrameId !== null) return
  console.log('Starting particle animation')
  animateParticles()
  
  // Spawn particles periodically based on actual traffic metrics
  particleSpawnInterval = setInterval(() => {
    if (!showTraffic.value) {
      console.log('Traffic hidden, not spawning')
      return
    }
    if (document.hidden) return // Don't spawn when tab is hidden
    
    // Only spawn particles if we have traffic metrics with actual traffic
    const hasTraffic = systemStore.trafficMetrics && systemStore.trafficMetrics.length > 0
    console.log('Traffic metrics:', systemStore.trafficMetrics?.length || 0, 'metrics')
    if (!hasTraffic) {
      console.log('No traffic metrics available')
      return
    }
    
    // Spawn particles based on actual traffic metrics
    systemStore.trafficMetrics.forEach(metric => {
      console.log(`Metric: ${metric.source} -> ${metric.target}, RPS: ${metric.requestsPerSecond}`)
      // Only spawn if there's actual traffic (requestsPerSecond > 0)
      if (metric.requestsPerSecond <= 0) return
      
      // Scale particle count based on requests per second with better visual scaling
      // Use logarithmic scaling for better visual representation across wide TPS range
      // Low traffic (1-50 TPS): 1-3 particles
      // Medium traffic (50-500 TPS): 3-8 particles  
      // High traffic (500+ TPS): 8-15 particles
      let particleCount: number
      if (metric.requestsPerSecond < 50) {
        particleCount = Math.max(1, Math.ceil(metric.requestsPerSecond / 20))
      } else if (metric.requestsPerSecond < 500) {
        particleCount = 3 + Math.ceil((metric.requestsPerSecond - 50) / 90)
      } else {
        particleCount = 8 + Math.ceil(Math.log10(metric.requestsPerSecond - 400) * 2)
      }
      particleCount = Math.min(particleCount, 15) // Cap at 15 for performance
      
      const adjustedCount = Math.ceil(particleCount * particleSpawnRate)
      
      console.log(`Spawning ${adjustedCount} particles for ${metric.source} -> ${metric.target} (${metric.requestsPerSecond} TPS)`)
      for (let i = 0; i < adjustedCount; i++) {
        setTimeout(() => {
          spawnParticle(metric.source, metric.target)
        }, i * 100) // Stagger spawns
      }
    })
  }, 1000) // Spawn every second
}

function stopParticleAnimation() {
  // Stop spawning new particles
  if (particleSpawnInterval !== null) {
    clearInterval(particleSpawnInterval)
    particleSpawnInterval = null
  }
  // Don't clear particles or cancel animation - let them finish naturally
  // The animateParticles function will stop itself when particles.length === 0
}

function initializeVisualization() {
  if (!svgRef.value || !containerRef.value) return
  
  const container = containerRef.value
  const width = container.clientWidth
  const height = 600
  
  // Initialize canvas
  if (canvasRef.value) {
    canvasRef.value.width = width
    canvasRef.value.height = height
  }
  
  // Clear existing
  d3.select(svgRef.value).selectAll('*').remove()
  
  // Reset zoom transform
  currentTransform = { k: 1, x: 0, y: 0 }
  
  const svg = d3.select(svgRef.value)
    .attr('width', width)
    .attr('height', height)
    .attr('viewBox', [0, 0, width, height])
  
  const g = svg.append('g')
  
  // Zoom disabled - causes particle sync issues with canvas overlay
  // Can be re-enabled in future with proper canvas transform handling
  // const zoom = d3.zoom<SVGSVGElement, unknown>()
  //   .scaleExtent([0.5, 2])
  //   .on('zoom', (event) => {
  //     g.attr('transform', event.transform)
  //     currentTransform = { k: event.transform.k, x: event.transform.x, y: event.transform.y }
  //   })
  // svg.call(zoom as any)
  
  // Fixed hierarchical layout - 4 layers (planes)
  const layerHeight = height / 5
  const padding = 80
  
  // Clear and recalculate node positions
  nodePositions.clear()
  
  // Control Plane (top)
  const controlServices = nodes.value.filter(n => n.plane === 'control')
  const controlSpacing = (width - 2 * padding) / (controlServices.length + 1)
  controlServices.forEach((node, i) => {
    nodePositions.set(node.id, {
      x: padding + controlSpacing * (i + 1),
      y: layerHeight
    })
  })
  
  // Processing Plane
  const processingServices = nodes.value.filter(n => n.plane === 'processing')
  const processingSpacing = (width - 2 * padding) / (processingServices.length + 1)
  processingServices.forEach((node, i) => {
    nodePositions.set(node.id, {
      x: padding + processingSpacing * (i + 1),
      y: layerHeight * 2.5
    })
  })
  
  // Support Plane (left side, vertical)
  const supportServices = nodes.value.filter(n => n.plane === 'support')
  const supportSpacing = (height - 2 * padding) / (supportServices.length + 1)
  supportServices.forEach((node, i) => {
    nodePositions.set(node.id, {
      x: padding,
      y: padding + supportSpacing * (i + 1)
    })
  })
  
  // Data Plane (bottom)
  const dataServices = nodes.value.filter(n => n.plane === 'data')
  const dataSpacing = (width - 2 * padding) / (dataServices.length + 1)
  dataServices.forEach((node, i) => {
    nodePositions.set(node.id, {
      x: padding + dataSpacing * (i + 1),
      y: layerHeight * 4
    })
  })
  
  // Define arrow markers
  svg.append('defs').selectAll('marker')
    .data(['arrow'])
    .join('marker')
    .attr('id', 'arrow')
    .attr('viewBox', '0 -5 10 10')
    .attr('refX', 35)
    .attr('refY', 0)
    .attr('markerWidth', 6)
    .attr('markerHeight', 6)
    .attr('orient', 'auto')
    .append('path')
    .attr('d', 'M0,-5L10,0L0,5')
    .attr('fill', '#666')
  
  // Draw plane labels
  const planeLabels = [
    { name: 'Control Plane', y: layerHeight - 30, color: planeColors.control },
    { name: 'Processing Plane', y: layerHeight * 2.5 - 30, color: planeColors.processing },
    { name: 'Data Plane', y: layerHeight * 4 - 30, color: planeColors.data }
  ]
  
  g.selectAll('.plane-label')
    .data(planeLabels)
    .join('text')
    .attr('class', 'plane-label')
    .attr('x', width / 2)
    .attr('y', d => d.y)
    .attr('text-anchor', 'middle')
    .attr('font-size', '14px')
    .attr('font-weight', 'bold')
    .attr('fill', d => d.color)
    .attr('opacity', 0.6)
    .text(d => d.name)
  
  // Support plane label (vertical)
  g.append('text')
    .attr('class', 'plane-label')
    .attr('x', padding - 50)
    .attr('y', height / 2)
    .attr('text-anchor', 'middle')
    .attr('font-size', '14px')
    .attr('font-weight', 'bold')
    .attr('fill', planeColors.support)
    .attr('opacity', 0.6)
    .attr('transform', `rotate(-90, ${padding - 50}, ${height / 2})`)
    .text('Support Plane')
  
  // Draw links with curved paths
  const linksGroup = g.append('g')
    .attr('class', 'connection-links')
    .style('opacity', showRoutes.value ? 1 : 0)
    .style('pointer-events', showRoutes.value ? 'auto' : 'none')
  
  linksGroup.selectAll('path')
    .data(links.value)
    .join('path')
    .attr('d', (d: any) => {
      const sourcePos = nodePositions.get(d.source)
      const targetPos = nodePositions.get(d.target)
      if (!sourcePos || !targetPos) return ''
      
      // Create curved path using quadratic bezier (same as particle animation)
      const dx = targetPos.x - sourcePos.x
      const dy = targetPos.y - sourcePos.y
      const dr = Math.sqrt(dx * dx + dy * dy) * 0.5
      
      // Control point for the curve (same calculation as getPointOnCurve)
      const midX = (sourcePos.x + targetPos.x) / 2
      const midY = (sourcePos.y + targetPos.y) / 2
      const offsetX = -dy / Math.sqrt(dx * dx + dy * dy) * dr
      const offsetY = dx / Math.sqrt(dx * dx + dy * dy) * dr
      const controlX = midX + offsetX
      const controlY = midY + offsetY
      
      return `M${sourcePos.x},${sourcePos.y}Q${controlX},${controlY} ${targetPos.x},${targetPos.y}`
    })
    .attr('stroke', '#666')
    .attr('stroke-opacity', 0.4)
    .attr('stroke-width', (d: any) => Math.sqrt(d.value) * 1.5)
    .attr('fill', 'none')
    .attr('marker-end', 'url(#arrow)')
    .style('cursor', 'pointer')
    .on('mouseenter', (event: any, d: any) => {
      const sourceId = typeof d.source === 'string' ? d.source : d.source.id
      const targetId = typeof d.target === 'string' ? d.target : d.target.id
      
      hoveredConnection.value = { source: sourceId, target: targetId }
      
      // Get midpoint of the connection for tooltip position
      const sourcePos = nodePositions.get(sourceId)
      const targetPos = nodePositions.get(targetId)
      
      if (sourcePos && targetPos) {
        const midX = (sourcePos.x + targetPos.x) / 2
        const midY = (sourcePos.y + targetPos.y) / 2
        
        // Smart positioning: flip tooltip below if near top
        const flipThreshold = 120
        
        if (midY < flipThreshold) {
          connectionTooltipPosition.value = { x: midX, y: midY + 30 }
          connectionTooltipFlipped.value = true
        } else {
          connectionTooltipPosition.value = { x: midX, y: midY - 30 }
          connectionTooltipFlipped.value = false
        }
        
        connectionTooltipVisible.value = true
      }
      
      // Highlight the hovered connection
      d3.select(event.currentTarget)
        .transition()
        .duration(200)
        .attr('stroke-opacity', 0.8)
        .attr('stroke-width', Math.sqrt(d.value) * 2.5)
    })
    .on('mouseleave', (event: any, d: any) => {
      hoveredConnection.value = null
      connectionTooltipVisible.value = false
      
      // Reset connection appearance
      d3.select(event.currentTarget)
        .transition()
        .duration(200)
        .attr('stroke-opacity', 0.4)
        .attr('stroke-width', Math.sqrt(d.value) * 1.5)
    })
  
  // Get connected node IDs for a given node
  const getConnectedNodes = (nodeId: string): Set<string> => {
    const connected = new Set<string>()
    links.value.forEach(link => {
      const sourceId = typeof link.source === 'string' ? link.source : link.source.id
      const targetId = typeof link.target === 'string' ? link.target : link.target.id
      if (sourceId === nodeId) connected.add(targetId)
      if (targetId === nodeId) connected.add(sourceId)
    })
    return connected
  }
  
  // Draw nodes
  const node = g.append('g')
    .selectAll('g')
    .data(nodes.value)
    .join('g')
    .attr('class', 'service-node')
    .attr('transform', (d: ServiceNode) => {
      const pos = nodePositions.get(d.id)
      return pos ? `translate(${pos.x},${pos.y})` : ''
    })
    .on('click', (event, d) => {
      event.stopPropagation()
      selectedNode.value = d
    })
    .on('mouseenter', (_event, d) => {
      hoveredNode.value = d
      const pos = nodePositions.get(d.id)
      if (pos) {
        // Smart positioning: flip tooltip below if near top
        const flipThreshold = 120
        
        if (pos.y < flipThreshold) {
          // Near top: show below
          tooltipPosition.value = { x: pos.x, y: pos.y + 50 }
          tooltipFlipped.value = true
        } else {
          // Normal: show above
          tooltipPosition.value = { x: pos.x, y: pos.y - 50 }
          tooltipFlipped.value = false
        }
        
        tooltipVisible.value = true
      }
      
      // Highlight connected nodes and links
      const connectedIds = getConnectedNodes(d.id)
      
      // Dim all nodes except hovered and connected
      g.selectAll('.service-node')
        .transition()
        .duration(200)
        .style('opacity', (n: any) => {
          return n.id === d.id || connectedIds.has(n.id) ? 1 : 0.2
        })
      
      // Highlight connected links
      g.selectAll('path')
        .transition()
        .duration(200)
        .style('opacity', (l: any) => {
          return l.source === d.id || l.target === d.id ? 0.8 : 0.1
        })
        .attr('stroke-width', (l: any) => {
          return l.source === d.id || l.target === d.id ? Math.sqrt(l.value) * 2.5 : Math.sqrt(l.value) * 1.5
        })
    })
    .on('mouseleave', () => {
      hoveredNode.value = null
      tooltipVisible.value = false
      
      // Reset all opacities
      g.selectAll('.service-node')
        .transition()
        .duration(200)
        .style('opacity', 1)
      
      g.selectAll('path')
        .transition()
        .duration(200)
        .style('opacity', 0.4)
        .attr('stroke-width', (l: any) => Math.sqrt(l.value) * 1.5)
    })
    .style('cursor', 'pointer')
  
  // Node shapes (rounded rectangles for services)
  node.append('rect')
    .attr('width', (d: ServiceNode) => Math.max(50, 40 + (d.replicas * 8)))
    .attr('height', (d: ServiceNode) => Math.max(50, 40 + (d.replicas * 8)))
    .attr('x', (d: ServiceNode) => -Math.max(25, 20 + (d.replicas * 4)))
    .attr('y', (d: ServiceNode) => -Math.max(25, 20 + (d.replicas * 4)))
    .attr('rx', 8)
    .attr('ry', 8)
    .attr('fill', (d: ServiceNode) => planeColors[d.plane])
    .attr('stroke', (d: ServiceNode) => statusColors[d.status])
    .attr('stroke-width', 3)
    .style('transition', 'all 0.3s ease')
  
  // Pod indicators
  node.each(function(d: ServiceNode) {
    const service = systemStore.services[d.id]
    const pods = service?.pods || []
    
    if (pods.length === 0) return
    
    const podGroup = d3.select(this).append('g').attr('class', 'pod-indicators')
    
    if (pods.length <= 5) {
      // Draw individual pod circles in a grid
      const positions = calculatePodPositions(pods.length)
      
      podGroup.selectAll('circle.pod')
        .data(pods)
        .join('circle')
        .attr('class', 'pod')
        .attr('r', 6)
        .attr('cx', (_: any, i: number) => positions[i].x)
        .attr('cy', (_: any, i: number) => positions[i].y)
        .attr('fill', (pod: any) => getPodColor(pod, d.plane))
        .attr('stroke', '#ffffff')
        .attr('stroke-width', 1)
        .style('cursor', 'pointer')
        .on('mouseenter', (event: any, pod: any) => {
          event.stopPropagation()
          hoveredPod.value = pod
          
          const nodePos = nodePositions.get(d.id)
          if (nodePos) {
            const podIndex = pods.indexOf(pod)
            const podPos = positions[podIndex]
            
            // Calculate absolute position
            const absX = nodePos.x + podPos.x
            const absY = nodePos.y + podPos.y
            
            // Smart positioning: flip tooltip below if near top
            const flipThreshold = 120
            
            if (absY < flipThreshold) {
              podTooltipPosition.value = { x: absX, y: absY + 20 }
              podTooltipFlipped.value = true
            } else {
              podTooltipPosition.value = { x: absX, y: absY - 20 }
              podTooltipFlipped.value = false
            }
            
            podTooltipVisible.value = true
          }
        })
        .on('mouseleave', () => {
          hoveredPod.value = null
          podTooltipVisible.value = false
        })
    } else {
      // Grouped representation with count badge
      podGroup.append('text')
        .attr('class', 'pod-count-badge')
        .text(`${pods.length} pods`)
        .attr('text-anchor', 'middle')
        .attr('dy', -15)
        .attr('font-size', '8px')
        .attr('fill', '#ffffff')
        .attr('opacity', 0.8)
        .style('pointer-events', 'none')
    }
  })
  
  // Node labels
  node.append('text')
    .text((d: ServiceNode) => {
      const parts = d.name.split('-')
      return parts.slice(0, 2).join(' ')
    })
    .attr('text-anchor', 'middle')
    .attr('dy', 40)
    .attr('font-size', '10px')
    .attr('fill', '#ffffff')
    .style('pointer-events', 'none')
  
  // Click outside to deselect
  svg.on('click', () => {
    selectedNode.value = null
  })
}

// Handle page visibility changes
function handleVisibilityChange() {
  if (document.hidden) {
    console.log('Tab hidden - animations paused')
  } else {
    console.log('Tab visible - animations resumed')
    // Reset frame timing when tab becomes visible again
    lastFrameTime = performance.now()
  }
}

onMounted(() => {
  initializeVisualization()
  
  // Only start particle animation if showTraffic is true
  if (showTraffic.value) {
    startParticleAnimation()
  }
  
  // Reinitialize on window resize
  window.addEventListener('resize', initializeVisualization)
  
  // Listen for visibility changes (Page Visibility API)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  // Force stop everything on unmount
  if (particleSpawnInterval !== null) {
    clearInterval(particleSpawnInterval)
    particleSpawnInterval = null
  }
  if (animationFrameId !== null) {
    cancelAnimationFrame(animationFrameId)
    animationFrameId = null
  }
  particles.value = []
  window.removeEventListener('resize', initializeVisualization)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})

watch(() => systemStore.services, () => {
  initializeVisualization()
}, { deep: true })

watch(showRoutes, (newValue) => {
  if (svgRef.value) {
    d3.select(svgRef.value)
      .select('.connection-links')
      .transition()
      .duration(300)
      .style('opacity', newValue ? 1 : 0)
      .style('pointer-events', newValue ? 'auto' : 'none')
  }
})

watch(showTraffic, (newValue) => {
  if (newValue) {
    startParticleAnimation()
  } else {
    stopParticleAnimation()
  }
})

// Watch for traffic metrics changes to control particle animation
watch(() => systemStore.trafficMetrics, (metrics) => {
  // Check if there's actual traffic data with non-zero requests
  const hasTraffic = metrics && metrics.length > 0 && metrics.some(m => m.requestsPerSecond > 0)
  
  if (showTraffic.value) {
    if (hasTraffic) {
      // Start animation if we have traffic and it's not running
      if (animationFrameId === null) {
        startParticleAnimation()
      }
      // Restart spawn interval if it's not running (this handles the case where
      // animation is running but spawn interval was stopped due to no metrics)
      if (particleSpawnInterval === null) {
        console.log('Restarting particle spawn interval due to new traffic metrics')
        particleSpawnInterval = setInterval(() => {
          if (!showTraffic.value) {
            console.log('Traffic hidden, not spawning')
            return
          }
          if (document.hidden) return
          
          const hasTraffic = systemStore.trafficMetrics && systemStore.trafficMetrics.length > 0
          console.log('Traffic metrics:', systemStore.trafficMetrics?.length || 0, 'metrics')
          if (!hasTraffic) {
            console.log('No traffic metrics available')
            return
          }
          
          systemStore.trafficMetrics.forEach(metric => {
            console.log(`Metric: ${metric.source} -> ${metric.target}, RPS: ${metric.requestsPerSecond}`)
            if (metric.requestsPerSecond <= 0) return
            
            const particleCount = Math.min(Math.ceil(metric.requestsPerSecond / 10), 3)
            const adjustedCount = Math.ceil(particleCount * particleSpawnRate)
            
            console.log(`Spawning ${adjustedCount} particles for ${metric.source} -> ${metric.target}`)
            for (let i = 0; i < adjustedCount; i++) {
              setTimeout(() => {
                spawnParticle(metric.source, metric.target)
              }, i * 100)
            }
          })
        }, 1000)
      }
    } else if (!hasTraffic && particleSpawnInterval !== null) {
      // Stop spawning new particles if no traffic (but let existing ones finish)
      console.log('No traffic, stopping spawn interval')
      clearInterval(particleSpawnInterval)
      particleSpawnInterval = null
    }
  }
}, { deep: true })

// Simulate traffic for testing
const simulateTraffic = async () => {
  const newMode = systemStore.trafficMode === 'simulated' ? 'real' : 'simulated'
  const success = await systemStore.setTrafficMode(newMode)
  if (success) {
    console.log(`Traffic mode switched to: ${newMode}`)
    // Auto-enable traffic view when simulating
    if (newMode === 'simulated') {
      showTraffic.value = true
    }
  }
}

// Initialize traffic mode on mount
onMounted(() => {
  systemStore.initializeTrafficMode()
})
</script>

<template>
  <div class="system-topology">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-bold">System Topology</h2>
      <div class="flex items-center gap-6">
        <div class="flex gap-2">
          <button
            @click="showRoutes = !showRoutes"
            class="px-3 py-1.5 rounded-lg text-sm font-medium transition-colors"
            :class="showRoutes 
              ? 'bg-primary text-background hover:bg-primary/90' 
              : 'bg-surface border border-text-muted/30 text-text-secondary hover:bg-surface-hover'"
          >
            {{ showRoutes ? 'Hide Routes' : 'View Routes' }}
          </button>
          <button
            @click="showTraffic = !showTraffic"
            class="px-3 py-1.5 rounded-lg text-sm font-medium transition-colors"
            :class="showTraffic 
              ? 'bg-primary text-background hover:bg-primary/90' 
              : 'bg-surface border border-text-muted/30 text-text-secondary hover:bg-surface-hover'"
          >
            {{ showTraffic ? 'Hide Traffic' : 'View Traffic' }}
          </button>
          <button
            @click="simulateTraffic"
            :disabled="systemStore.trafficModeLoading"
            class="px-3 py-1.5 rounded-lg text-sm font-medium transition-colors flex items-center gap-2"
            :class="systemStore.trafficMode === 'simulated'
              ? 'bg-purple-600 text-white hover:bg-purple-700' 
              : 'bg-surface border border-text-muted/30 text-text-secondary hover:bg-surface-hover'"
            :title="systemStore.trafficMode === 'simulated' ? 'Stop simulation' : 'Simulate traffic for testing'"
          >
            <span>🎭</span>
            <span>{{ systemStore.trafficMode === 'simulated' ? 'Stop Simulation' : 'Simulate Traffic' }}</span>
          </button>
        </div>
        <div class="flex gap-4 text-sm">
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full" :style="{ backgroundColor: planeColors.data }"></div>
            <span>Data</span>
          </div>
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full" :style="{ backgroundColor: planeColors.processing }"></div>
            <span>Processing</span>
          </div>
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full" :style="{ backgroundColor: planeColors.support }"></div>
            <span>Support</span>
          </div>
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full" :style="{ backgroundColor: planeColors.control }"></div>
            <span>Control</span>
          </div>
        </div>
      </div>
    </div>
    
    <div ref="containerRef" class="relative bg-background rounded-lg border border-text-muted/20 overflow-visible">
      <svg ref="svgRef"></svg>
      <canvas ref="canvasRef" class="absolute top-0 left-0 pointer-events-none"></canvas>
      
      <!-- Hover tooltip -->
      <div 
        v-if="tooltipVisible && hoveredNode" 
        class="absolute pointer-events-none bg-surface/95 backdrop-blur-sm border border-primary/50 rounded-lg p-3 shadow-lg transition-opacity duration-200"
        :style="{ 
          left: `${tooltipPosition.x}px`, 
          top: `${tooltipPosition.y}px`,
          transform: tooltipFlipped ? 'translate(-50%, 0%)' : 'translate(-50%, -100%)',
          zIndex: 50
        }"
      >
        <div class="text-sm space-y-1.5 min-w-48">
          <div class="font-bold text-primary border-b border-primary/30 pb-1 mb-2">
            {{ hoveredNode.name }}
          </div>
          <div class="flex justify-between items-center">
            <span class="text-text-secondary text-xs">Status:</span>
            <span 
              class="capitalize font-medium text-xs px-2 py-0.5 rounded"
              :style="{ 
                color: statusColors[hoveredNode.status],
                backgroundColor: statusColors[hoveredNode.status] + '20'
              }"
            >
              {{ hoveredNode.status }}
            </span>
          </div>
          <div class="flex justify-between">
            <span class="text-text-secondary text-xs">Plane:</span>
            <span class="capitalize text-xs" :style="{ color: planeColors[hoveredNode.plane] }">
              {{ hoveredNode.plane }}
            </span>
          </div>
          <div class="flex justify-between">
            <span class="text-text-secondary text-xs">Replicas:</span>
            <span class="font-mono text-xs">{{ hoveredNode.replicas }}</span>
          </div>
          <div v-if="systemStore.services[hoveredNode.id]" class="pt-1 border-t border-text-muted/20 mt-2">
            <div class="flex justify-between">
              <span class="text-text-secondary text-xs">CPU:</span>
              <span class="font-mono text-xs">{{ systemStore.services[hoveredNode.id].cpu || 'N/A' }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-text-secondary text-xs">Memory:</span>
              <span class="font-mono text-xs">{{ systemStore.services[hoveredNode.id].memory || 'N/A' }}</span>
            </div>
          </div>
        </div>
      </div>
      
      <!-- Pod tooltip -->
      <div 
        v-if="podTooltipVisible && hoveredPod" 
        class="absolute pointer-events-none bg-surface/95 backdrop-blur-sm border border-primary/50 rounded-lg p-3 shadow-lg transition-opacity duration-200"
        :style="{ 
          left: `${podTooltipPosition.x}px`, 
          top: `${podTooltipPosition.y}px`,
          transform: podTooltipFlipped ? 'translate(-50%, 0%)' : 'translate(-50%, -100%)',
          zIndex: 60
        }"
      >
        <div class="text-sm space-y-1.5 min-w-48">
          <div class="font-bold text-primary border-b border-primary/30 pb-1 mb-2 text-xs">
            {{ hoveredPod.name }}
          </div>
          <div class="flex justify-between items-center">
            <span class="text-text-secondary text-xs">Status:</span>
            <span 
              class="capitalize font-medium text-xs px-2 py-0.5 rounded"
              :style="{ 
                color: hoveredPod.ready ? '#00ff88' : '#666666',
                backgroundColor: hoveredPod.ready ? '#00ff8820' : '#66666620'
              }"
            >
              {{ hoveredPod.status }}
            </span>
          </div>
          <div class="flex justify-between">
            <span class="text-text-secondary text-xs">CPU:</span>
            <span class="font-mono text-xs">{{ hoveredPod.cpu }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-text-secondary text-xs">Memory:</span>
            <span class="font-mono text-xs">{{ hoveredPod.memory }}</span>
          </div>
          <div v-if="hoveredPod.p50Latency || hoveredPod.p99Latency" class="pt-1 border-t border-text-muted/20 mt-2">
            <div v-if="hoveredPod.p50Latency" class="flex justify-between">
              <span class="text-text-secondary text-xs">P50 Latency:</span>
              <span class="font-mono text-xs">{{ hoveredPod.p50Latency }}ms</span>
            </div>
            <div v-if="hoveredPod.p99Latency" class="flex justify-between">
              <span class="text-text-secondary text-xs">P99 Latency:</span>
              <span class="font-mono text-xs">{{ hoveredPod.p99Latency }}ms</span>
            </div>
          </div>
          <div v-if="hoveredPod.errorRate !== undefined" class="flex justify-between">
            <span class="text-text-secondary text-xs">Error Rate:</span>
            <span class="font-mono text-xs">{{ (hoveredPod.errorRate * 100).toFixed(2) }}%</span>
          </div>
        </div>
      </div>
      
      <!-- Connection tooltip -->
      <div 
        v-if="connectionTooltipVisible && hoveredConnection" 
        class="absolute pointer-events-none bg-surface/95 backdrop-blur-sm border border-primary/50 rounded-lg p-3 shadow-lg transition-opacity duration-200"
        :style="{ 
          left: `${connectionTooltipPosition.x}px`, 
          top: `${connectionTooltipPosition.y}px`,
          transform: connectionTooltipFlipped ? 'translate(-50%, 0%)' : 'translate(-50%, -100%)',
          zIndex: 55
        }"
      >
        <div class="text-sm space-y-1.5 min-w-56">
          <div class="font-bold text-primary border-b border-primary/30 pb-1 mb-2 text-xs">
            {{ hoveredConnection.source }} → {{ hoveredConnection.target }}
          </div>
          <template v-if="hoveredConnectionMetrics">
            <div class="flex justify-between">
              <span class="text-text-secondary text-xs">Traffic Volume:</span>
              <span class="font-mono text-xs">
                {{ hoveredConnectionMetrics.requestsPerSecond.toFixed(1) }} req/s
              </span>
            </div>
            <div class="pt-1 border-t border-text-muted/20 mt-2">
              <div class="flex justify-between">
                <span class="text-text-secondary text-xs">Avg Latency:</span>
                <span class="font-mono text-xs">{{ hoveredConnectionMetrics.avgLatency }}ms</span>
              </div>
              <div class="flex justify-between">
                <span class="text-text-secondary text-xs">P50 Latency:</span>
                <span class="font-mono text-xs">{{ hoveredConnectionMetrics.p50Latency }}ms</span>
              </div>
              <div class="flex justify-between">
                <span class="text-text-secondary text-xs">P99 Latency:</span>
                <span class="font-mono text-xs">{{ hoveredConnectionMetrics.p99Latency }}ms</span>
              </div>
            </div>
            <div class="flex justify-between pt-1 border-t border-text-muted/20 mt-2">
              <span class="text-text-secondary text-xs">Error Rate:</span>
              <span class="font-mono text-xs" :class="{
                'text-success': hoveredConnectionMetrics.errorRate < 0.01,
                'text-danger': hoveredConnectionMetrics.errorRate >= 0.01
              }">
                {{ (hoveredConnectionMetrics.errorRate * 100).toFixed(2) }}%
              </span>
            </div>
            <div class="flex justify-between items-center">
              <span class="text-text-secondary text-xs">Health:</span>
              <span 
                class="capitalize font-medium text-xs px-2 py-0.5 rounded"
                :style="{ 
                  color: hoveredConnectionMetrics.healthStatus === 'healthy' ? '#00ff88' :
                         hoveredConnectionMetrics.healthStatus === 'warning' ? '#ffaa00' :
                         hoveredConnectionMetrics.healthStatus === 'error' ? '#ff0055' : '#666666',
                  backgroundColor: hoveredConnectionMetrics.healthStatus === 'healthy' ? '#00ff8820' :
                                   hoveredConnectionMetrics.healthStatus === 'warning' ? '#ffaa0020' :
                                   hoveredConnectionMetrics.healthStatus === 'error' ? '#ff005520' : '#66666620'
                }"
              >
                {{ hoveredConnectionMetrics.healthStatus }}
              </span>
            </div>
          </template>
          <div v-else class="text-text-muted text-xs italic">
            No traffic metrics available
          </div>
        </div>
      </div>
      
      <!-- Node details panel (click) -->
      <div 
        v-if="selectedNode" 
        class="absolute top-4 right-4 bg-surface border border-primary rounded-lg p-4 min-w-64 shadow-xl"
      >
        <div class="flex items-center justify-between mb-2">
          <h3 class="font-bold text-primary">{{ selectedNode.name }}</h3>
          <button @click="selectedNode = null" class="text-text-muted hover:text-text-primary">✕</button>
        </div>
        <div class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-text-secondary">Plane:</span>
            <span class="capitalize">{{ selectedNode.plane }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-text-secondary">Status:</span>
            <span 
              class="capitalize font-medium"
              :style="{ color: statusColors[selectedNode.status] }"
            >
              {{ selectedNode.status }}
            </span>
          </div>
          <div class="flex justify-between">
            <span class="text-text-secondary">Replicas:</span>
            <span>{{ selectedNode.replicas }}</span>
          </div>
        </div>
      </div>
    </div>
    
    <div class="mt-2 text-xs text-text-muted text-center">
      Hover for quick info • Click for details
    </div>
  </div>
</template>

<style scoped>
.system-topology {
  width: 100%;
}
</style>
