<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import * as d3 from 'd3'
import { useSystemStore } from '@/stores/system'
import type { ServiceNode, ServiceLink } from '@/types'

const systemStore = useSystemStore()
const svgRef = ref<SVGSVGElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const selectedNode = ref<ServiceNode | null>(null)
const hoveredNode = ref<ServiceNode | null>(null)
const tooltipPosition = ref({ x: 0, y: 0 })
const tooltipVisible = ref(false)
const tooltipFlipped = ref(false)

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
  data: '#00d9ff',
  processing: '#ff00ff',
  support: '#ffaa00',
  control: '#00ff88'
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
function getPodColor(pod: any, plane: string): string {
  const colors = {
    data: '#00d9ff',
    processing: '#ff00ff',
    support: '#ffaa00',
    control: '#00ff88'
  }
  
  // Failed pods are red
  if (pod.status === 'Failed') {
    return '#ff0055'
  }
  // Not ready pods are gray
  if (!pod.ready) {
    return '#666666'
  }
  // Running and ready pods use plane color
  if (pod.status === 'Running' && pod.ready) {
    return colors[plane as keyof typeof colors] || '#666666'
  }
  // Default to gray for other states
  return '#666666'
}

function initializeVisualization() {
  if (!svgRef.value || !containerRef.value) return
  
  const container = containerRef.value
  const width = container.clientWidth
  const height = 600
  
  // Clear existing
  d3.select(svgRef.value).selectAll('*').remove()
  
  const svg = d3.select(svgRef.value)
    .attr('width', width)
    .attr('height', height)
    .attr('viewBox', [0, 0, width, height])
  
  const g = svg.append('g')
  
  // Add zoom behavior
  const zoom = d3.zoom<SVGSVGElement, unknown>()
    .scaleExtent([0.5, 2])
    .on('zoom', (event) => {
      g.attr('transform', event.transform)
    })
  
  svg.call(zoom as any)
  
  // Fixed hierarchical layout - 4 layers (planes)
  const layerHeight = height / 5
  const padding = 80
  
  // Calculate fixed positions for each node
  const nodePositions = new Map<string, { x: number, y: number }>()
  
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
  g.append('g')
    .selectAll('path')
    .data(links.value)
    .join('path')
    .attr('d', (d: any) => {
      const sourcePos = nodePositions.get(d.source)
      const targetPos = nodePositions.get(d.target)
      if (!sourcePos || !targetPos) return ''
      
      // Create curved path
      const dx = targetPos.x - sourcePos.x
      const dy = targetPos.y - sourcePos.y
      const dr = Math.sqrt(dx * dx + dy * dy) * 0.5
      
      return `M${sourcePos.x},${sourcePos.y}A${dr},${dr} 0 0,1 ${targetPos.x},${targetPos.y}`
    })
    .attr('stroke', '#666')
    .attr('stroke-opacity', 0.4)
    .attr('stroke-width', (d: any) => Math.sqrt(d.value) * 1.5)
    .attr('fill', 'none')
    .attr('marker-end', 'url(#arrow)')
  
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
  
  // Node circles
  node.append('circle')
    .attr('r', (d: ServiceNode) => Math.max(20, 15 + (d.replicas * 5)))
    .attr('fill', (d: ServiceNode) => planeColors[d.plane])
    .attr('stroke', (d: ServiceNode) => statusColors[d.status])
    .attr('stroke-width', 3)
    .style('transition', 'all 0.3s ease')
  
  // Pod indicators
  node.each(function(d: ServiceNode) {
    const service = systemStore.services[d.id]
    const pods = service?.pods || []
    
    if (pods.length === 0) return
    
    // Debug: log first pod to see what data we're getting
    if (pods.length > 0) {
      console.log(`Service ${d.id} pod data:`, pods[0], `plane: ${d.plane}`)
    }
    
    const podGroup = d3.select(this).append('g').attr('class', 'pod-indicators')
    
    if (pods.length <= 5) {
      // Draw individual pod circles in a grid
      const positions = calculatePodPositions(pods.length)
      
      podGroup.selectAll('circle.pod')
        .data(pods)
        .join('circle')
        .attr('class', 'pod')
        .attr('r', 4)
        .attr('cx', (_: any, i: number) => positions[i].x)
        .attr('cy', (_: any, i: number) => positions[i].y)
        .attr('fill', (pod: any) => getPodColor(pod, d.plane))
        .attr('stroke', '#0a0a0a')
        .attr('stroke-width', 0.5)
        .style('cursor', 'pointer')
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

onMounted(() => {
  initializeVisualization()
  
  // Reinitialize on window resize
  window.addEventListener('resize', initializeVisualization)
})

watch(() => systemStore.services, () => {
  initializeVisualization()
}, { deep: true })
</script>

<template>
  <div class="system-topology">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-bold">System Topology</h2>
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
    
    <div ref="containerRef" class="relative bg-background rounded-lg border border-text-muted/20 overflow-visible">
      <svg ref="svgRef"></svg>
      
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
      Hover for quick info • Click for details • Scroll to zoom
    </div>
  </div>
</template>

<style scoped>
.system-topology {
  width: 100%;
}
</style>
