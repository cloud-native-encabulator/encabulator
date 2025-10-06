import { useEffect, useRef } from 'react'
import * as d3 from 'd3'
import './SystemTopology.css'

interface Service {
  status: string
  replicas: number
}

interface SystemTopologyProps {
  services: Record<string, Service>
}

export default function SystemTopology({ services }: SystemTopologyProps) {
  const svgRef = useRef<SVGSVGElement>(null)

  useEffect(() => {
    if (!svgRef.current) return

    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()

    // Define service planes
    const planes = [
      { name: 'Data Plane', services: ['hydrocoptic-datastore', 'sinusoidal-database', 'prefabulated-cache'], y: 100 },
      { name: 'Processing Plane', services: ['panametric-fam-processor', 'logarithmic-casing-service', 'dingle-arm-api-gateway'], y: 250 },
      { name: 'Support Plane', services: ['cardinal-grammeter', 'differential-girdle', 'side-fumbling-collector'], y: 400 },
      { name: 'Control Plane', services: ['encabulation-control-console', 'malaise-injection-engine', 'demo-scenario-orchestrator', 'unilateral-phase-detractor'], y: 550 },
    ]

    // Draw planes
    planes.forEach((plane) => {
      const g = svg.append('g')
      
      // Plane label
      g.append('text')
        .attr('x', 20)
        .attr('y', plane.y - 10)
        .attr('class', 'plane-label')
        .text(plane.name)

      // Draw services
      plane.services.forEach((serviceName, i) => {
        const x = 150 + i * 200
        const service = services[serviceName] || { status: 'unknown', replicas: 0 }
        const statusColor = service.status === 'healthy' ? '#4caf50' : service.status === 'degraded' ? '#ff9800' : '#666'

        // Service box
        g.append('rect')
          .attr('x', x)
          .attr('y', plane.y)
          .attr('width', 150)
          .attr('height', 60)
          .attr('rx', 5)
          .attr('fill', '#2a2a2a')
          .attr('stroke', statusColor)
          .attr('stroke-width', 2)

        // Service name
        g.append('text')
          .attr('x', x + 75)
          .attr('y', plane.y + 25)
          .attr('class', 'service-name')
          .attr('text-anchor', 'middle')
          .text(serviceName.split('-').slice(0, 2).join(' '))

        // Replica count
        g.append('text')
          .attr('x', x + 75)
          .attr('y', plane.y + 45)
          .attr('class', 'service-replicas')
          .attr('text-anchor', 'middle')
          .text(`${service.replicas} replicas`)
      })
    })
  }, [services])

  return (
    <div className="system-topology">
      <h2>System Topology</h2>
      <svg ref={svgRef} width="100%" height="600"></svg>
    </div>
  )
}
