<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import * as d3 from 'd3'
import { useSystemStore } from '@/stores/system'
import type { ServiceNode, ServiceLink } from '@/types'

const systemStore = useSystemStore()
const svgRef = ref<SVGSVGElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const selectedNode = ref<ServiceNode | null>(null)

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
  // Define service relationships
  return [
    // Processing → Data
    { source: 'panametric-fam-processor', target: 'hydrocoptic-datastore', value: 1 },
    { source: 'panametric-fam-processor', target: 'prefabulated-cache', value: 1 },
    { source: 'logarithmic-casing-service', target: 'sinusoidal-database', value: 1 },
    
    // API Gateway → Processing
    { source: 'dingle-arm-api-gateway', target: 'panametric-fam-processor', value: 2 },
    { source: 'dingle-arm-api-gateway', target: 'logarithmic-casing-service', value: 1 },
    
    // Support → Processing
    { source: 'unilateral-phase-detractor', target: 'dingle-arm-api-gateway', value: 1 },
    { source: 'differential-girdle', target: 'panametric-fam-processor', value: 1 },
    { source: 'side-fumbling-collector', target: 'hydrocoptic-datastore', value: 1 },
    
    // Control → Everything
    { source: 'encabulation-control-console', target: 'dingle-arm-api-gateway', value: 1 },
    { source: 'malaise-injection-engine', target: 'panametric-fam-processor', value: 1 },
    { source: 'cardinal-grammeter', target: 'dingle-arm-api-gateway', value: 2 },
    { source: 'demo-scenario-orchestrator', target: 'encabulation-control-console', value: 1 }
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
  
  // Add zoom behavior
  const g = svg.append('g')
  
  const zoom = d3.zoom<SVGSVGElement, unknown>()
    .scaleExtent([0.5, 3])
    .on('zoom', (event) => {
      g.attr('transform', event.transform)
    })
  
  svg.call(zoom as any)
  
  // Create force simulation
  const simulation = d3.forceSimulation(nodes.value as any)
    .force('link', d3.forceLink(links.value)
      .id((d: any) => d.id)
      .distance(150))
    .force('charge', d3.forceManyBody().strength(-300))
    .force('center', d3.forceCenter(width / 2, height / 2))
    .force('collision', d3.forceCollide().radius(50))
  
  // Draw links
  const link = g.append('g')
    .selectAll('line')
    .data(links.value)
    .join('line')
    .attr('stroke', '#666')
    .attr('stroke-opacity', 0.6)
    .attr('stroke-width', (d: any) => Math.sqrt(d.value) * 2)
  
  // Draw nodes
  const node = g.append('g')
    .selectAll('g')
    .data(nodes.value)
    .join('g')
    .call(d3.drag<any, ServiceNode>()
      .on('start', dragstarted)
      .on('drag', dragged)
      .on('end', dragended) as any)
    .on('click', (event, d) => {
      event.stopPropagation()
      selectedNode.value = d
    })
  
  // Node circles
  node.append('circle')
    .attr('r', 25)
    .attr('fill', (d: ServiceNode) => planeColors[d.plane])
    .attr('stroke', (d: ServiceNode) => statusColors[d.status])
    .attr('stroke-width', 3)
    .style('cursor', 'pointer')
  
  // Node labels
  node.append('text')
    .text((d: ServiceNode) => d.name.split('-').slice(0, 2).join(' '))
    .attr('text-anchor', 'middle')
    .attr('dy', 40)
    .attr('font-size', '10px')
    .attr('fill', '#ffffff')
    .style('pointer-events', 'none')
  
  // Replica count
  node.append('text')
    .text((d: ServiceNode) => d.replicas)
    .attr('text-anchor', 'middle')
    .attr('dy', 5)
    .attr('font-size', '14px')
    .attr('font-weight', 'bold')
    .attr('fill', '#0a0a0a')
    .style('pointer-events', 'none')
  
  // Update positions on tick
  simulation.on('tick', () => {
    link
      .attr('x1', (d: any) => d.source.x)
      .attr('y1', (d: any) => d.source.y)
      .attr('x2', (d: any) => d.target.x)
      .attr('y2', (d: any) => d.target.y)
    
    node.attr('transform', (d: any) => `translate(${d.x},${d.y})`)
  })
  
  function dragstarted(event: any) {
    if (!event.active) simulation.alphaTarget(0.3).restart()
    event.subject.fx = event.subject.x
    event.subject.fy = event.subject.y
  }
  
  function dragged(event: any) {
    event.subject.fx = event.x
    event.subject.fy = event.y
  }
  
  function dragended(event: any) {
    if (!event.active) simulation.alphaTarget(0)
    event.subject.fx = null
    event.subject.fy = null
  }
  
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
    
    <div ref="containerRef" class="relative bg-background rounded-lg border border-text-muted/20 overflow-hidden">
      <svg ref="svgRef"></svg>
      
      <!-- Node details panel -->
      <div 
        v-if="selectedNode" 
        class="absolute top-4 right-4 bg-surface border border-primary rounded-lg p-4 min-w-64"
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
      Drag nodes to reposition • Scroll to zoom • Click node for details
    </div>
  </div>
</template>

<style scoped>
.system-topology {
  width: 100%;
}
</style>
