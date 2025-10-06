import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import SystemTopology from './SystemTopology'

describe('SystemTopology', () => {
  it('renders system topology with title', () => {
    render(<SystemTopology services={{}} />)
    
    expect(screen.getByText('System Topology')).toBeInTheDocument()
  })

  it('renders SVG element', () => {
    const { container } = render(<SystemTopology services={{}} />)
    
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveAttribute('width', '100%')
    expect(svg).toHaveAttribute('height', '600')
  })

  it('renders with empty services object', () => {
    const { container } = render(<SystemTopology services={{}} />)
    
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
  })

  it('renders with service data', () => {
    const services = {
      'panametric-fam-processor': { status: 'healthy', replicas: 3 },
      'dingle-arm-api-gateway': { status: 'degraded', replicas: 2 },
      'sinusoidal-database': { status: 'healthy', replicas: 1 },
    }
    
    const { container } = render(<SystemTopology services={services} />)
    
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
  })

  it('updates visualization when services prop changes', () => {
    const initialServices = {
      'panametric-fam-processor': { status: 'healthy', replicas: 2 },
    }
    
    const { rerender, container } = render(<SystemTopology services={initialServices} />)
    
    const updatedServices = {
      'panametric-fam-processor': { status: 'degraded', replicas: 3 },
      'dingle-arm-api-gateway': { status: 'healthy', replicas: 1 },
    }
    
    rerender(<SystemTopology services={updatedServices} />)
    
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
  })
})
