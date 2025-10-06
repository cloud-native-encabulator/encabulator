import { useState } from 'react'
import './ControlPanel.css'

interface ControlPanelProps {
  onAction: (action: any) => void
}

export default function ControlPanel({ onAction }: ControlPanelProps) {
  const [selectedService, setSelectedService] = useState('')
  const [selectedScenario, setSelectedScenario] = useState('')

  const handleScaleService = (direction: 'up' | 'down') => {
    if (!selectedService) return
    onAction({
      type: 'scale_service',
      payload: { service: selectedService, direction },
    })
  }

  const handleInjectFailure = () => {
    if (!selectedService) return
    onAction({
      type: 'inject_failure',
      payload: { service: selectedService },
    })
  }

  const handleRunScenario = () => {
    if (!selectedScenario) return
    onAction({
      type: 'run_scenario',
      payload: { scenario: selectedScenario },
    })
  }

  return (
    <div className="control-panel">
      <h3>Control Panel</h3>
      
      <div className="control-section">
        <h4>Service Control</h4>
        <select 
          value={selectedService} 
          onChange={(e) => setSelectedService(e.target.value)}
          className="control-select"
        >
          <option value="">Select Service</option>
          <option value="panametric-fam-processor">Panametric Fam Processor</option>
          <option value="dingle-arm-api-gateway">Dingle Arm Gateway</option>
          <option value="logarithmic-casing-service">Logarithmic Casing</option>
        </select>
        
        <div className="button-group">
          <button onClick={() => handleScaleService('up')} disabled={!selectedService}>
            Scale Up
          </button>
          <button onClick={() => handleScaleService('down')} disabled={!selectedService}>
            Scale Down
          </button>
        </div>
        
        <button 
          onClick={handleInjectFailure} 
          disabled={!selectedService}
          className="danger-button"
        >
          Inject Failure
        </button>
      </div>

      <div className="control-section">
        <h4>Demo Scenarios</h4>
        <select 
          value={selectedScenario} 
          onChange={(e) => setSelectedScenario(e.target.value)}
          className="control-select"
        >
          <option value="">Select Scenario</option>
          <option value="startup">System Startup</option>
          <option value="data-flow">Data Flow Demo</option>
          <option value="failure-recovery">Failure Recovery</option>
          <option value="scaling">Auto-scaling Demo</option>
        </select>
        
        <button onClick={handleRunScenario} disabled={!selectedScenario}>
          Run Scenario
        </button>
      </div>
    </div>
  )
}
