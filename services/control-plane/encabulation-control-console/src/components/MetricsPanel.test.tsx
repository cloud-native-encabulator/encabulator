import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import MetricsPanel from './MetricsPanel'

describe('MetricsPanel', () => {
  it('renders metrics panel with title', () => {
    render(<MetricsPanel metrics={{}} />)
    
    expect(screen.getByText('System Metrics')).toBeInTheDocument()
  })

  it('displays no metrics message when metrics object is empty', () => {
    render(<MetricsPanel metrics={{}} />)
    
    expect(screen.getByText('No metrics available')).toBeInTheDocument()
  })

  it('renders metrics with correct formatting for rates', () => {
    const metrics = {
      request_rate: 125.456,
      error_rate: 2.789,
    }
    
    render(<MetricsPanel metrics={metrics} />)
    
    expect(screen.getByText('125.46/s')).toBeInTheDocument()
    expect(screen.getByText('2.79/s')).toBeInTheDocument()
  })

  it('renders metrics with correct formatting for latency', () => {
    const metrics = {
      avg_latency: 45.678,
      p99_latency: 123.456,
    }
    
    render(<MetricsPanel metrics={metrics} />)
    
    expect(screen.getByText('46ms')).toBeInTheDocument()
    expect(screen.getByText('123ms')).toBeInTheDocument()
  })

  it('renders metrics with correct formatting for percentages', () => {
    const metrics = {
      cpu_usage: 67.8,
      memory_usage: 45.2,
    }
    
    render(<MetricsPanel metrics={metrics} />)
    
    expect(screen.getByText('67.8%')).toBeInTheDocument()
    expect(screen.getByText('45.2%')).toBeInTheDocument()
  })

  it('renders metric labels with proper formatting', () => {
    const metrics = {
      request_rate: 100,
      avg_latency: 50,
    }
    
    render(<MetricsPanel metrics={metrics} />)
    
    expect(screen.getByText('Request Rate')).toBeInTheDocument()
    expect(screen.getByText('Avg Latency')).toBeInTheDocument()
  })

  it('renders multiple metrics correctly', () => {
    const metrics = {
      request_rate: 100.5,
      error_rate: 1.2,
      avg_latency: 45.6,
      cpu_usage: 70.5,
      memory_usage: 60.3,
      active_connections: 250.75,
    }
    
    render(<MetricsPanel metrics={metrics} />)
    
    expect(screen.getByText('100.50/s')).toBeInTheDocument()
    expect(screen.getByText('1.20/s')).toBeInTheDocument()
    expect(screen.getByText('46ms')).toBeInTheDocument()
    expect(screen.getByText('70.5%')).toBeInTheDocument()
    expect(screen.getByText('60.3%')).toBeInTheDocument()
    expect(screen.getByText('250.75')).toBeInTheDocument()
  })
})
