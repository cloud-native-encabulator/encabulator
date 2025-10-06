import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ControlPanel from './ControlPanel'

describe('ControlPanel', () => {
  it('renders control panel with title', () => {
    const mockOnAction = vi.fn()
    render(<ControlPanel onAction={mockOnAction} />)
    
    expect(screen.getByText('Control Panel')).toBeInTheDocument()
    expect(screen.getByText('Service Control')).toBeInTheDocument()
    expect(screen.getByText('Demo Scenarios')).toBeInTheDocument()
  })

  it('disables action buttons when no service is selected', () => {
    const mockOnAction = vi.fn()
    render(<ControlPanel onAction={mockOnAction} />)
    
    const scaleUpButton = screen.getByText('Scale Up')
    const scaleDownButton = screen.getByText('Scale Down')
    const injectFailureButton = screen.getByText('Inject Failure')
    
    expect(scaleUpButton).toBeDisabled()
    expect(scaleDownButton).toBeDisabled()
    expect(injectFailureButton).toBeDisabled()
  })

  it('enables action buttons when service is selected', () => {
    const mockOnAction = vi.fn()
    render(<ControlPanel onAction={mockOnAction} />)
    
    const serviceSelect = screen.getAllByRole('combobox')[0]
    fireEvent.change(serviceSelect, { target: { value: 'panametric-fam-processor' } })
    
    const scaleUpButton = screen.getByText('Scale Up')
    const scaleDownButton = screen.getByText('Scale Down')
    const injectFailureButton = screen.getByText('Inject Failure')
    
    expect(scaleUpButton).not.toBeDisabled()
    expect(scaleDownButton).not.toBeDisabled()
    expect(injectFailureButton).not.toBeDisabled()
  })

  it('calls onAction with correct payload when scaling up', () => {
    const mockOnAction = vi.fn()
    render(<ControlPanel onAction={mockOnAction} />)
    
    const serviceSelect = screen.getAllByRole('combobox')[0]
    fireEvent.change(serviceSelect, { target: { value: 'panametric-fam-processor' } })
    
    const scaleUpButton = screen.getByText('Scale Up')
    fireEvent.click(scaleUpButton)
    
    expect(mockOnAction).toHaveBeenCalledWith({
      type: 'scale_service',
      payload: { service: 'panametric-fam-processor', direction: 'up' },
    })
  })

  it('calls onAction with correct payload when scaling down', () => {
    const mockOnAction = vi.fn()
    render(<ControlPanel onAction={mockOnAction} />)
    
    const serviceSelect = screen.getAllByRole('combobox')[0]
    fireEvent.change(serviceSelect, { target: { value: 'dingle-arm-api-gateway' } })
    
    const scaleDownButton = screen.getByText('Scale Down')
    fireEvent.click(scaleDownButton)
    
    expect(mockOnAction).toHaveBeenCalledWith({
      type: 'scale_service',
      payload: { service: 'dingle-arm-api-gateway', direction: 'down' },
    })
  })

  it('calls onAction when injecting failure', () => {
    const mockOnAction = vi.fn()
    render(<ControlPanel onAction={mockOnAction} />)
    
    const serviceSelect = screen.getAllByRole('combobox')[0]
    fireEvent.change(serviceSelect, { target: { value: 'logarithmic-casing-service' } })
    
    const injectFailureButton = screen.getByText('Inject Failure')
    fireEvent.click(injectFailureButton)
    
    expect(mockOnAction).toHaveBeenCalledWith({
      type: 'inject_failure',
      payload: { service: 'logarithmic-casing-service' },
    })
  })

  it('disables scenario button when no scenario is selected', () => {
    const mockOnAction = vi.fn()
    render(<ControlPanel onAction={mockOnAction} />)
    
    const runScenarioButton = screen.getByText('Run Scenario')
    expect(runScenarioButton).toBeDisabled()
  })

  it('calls onAction when running a scenario', () => {
    const mockOnAction = vi.fn()
    render(<ControlPanel onAction={mockOnAction} />)
    
    const scenarioSelect = screen.getAllByRole('combobox')[1]
    fireEvent.change(scenarioSelect, { target: { value: 'startup' } })
    
    const runScenarioButton = screen.getByText('Run Scenario')
    fireEvent.click(runScenarioButton)
    
    expect(mockOnAction).toHaveBeenCalledWith({
      type: 'run_scenario',
      payload: { scenario: 'startup' },
    })
  })
})
