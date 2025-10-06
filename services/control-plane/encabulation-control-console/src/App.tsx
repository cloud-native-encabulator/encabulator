import { useState } from 'react'
import SystemTopology from './components/SystemTopology'
import ControlPanel from './components/ControlPanel'
import MetricsPanel from './components/MetricsPanel'
import { useWebSocket } from './hooks/useWebSocket'
import './App.css'

interface SystemStatus {
  services: Record<string, { status: string; replicas: number }>
  metrics: Record<string, number>
}

function App() {
  const [systemStatus, setSystemStatus] = useState<SystemStatus>({
    services: {},
    metrics: {},
  })
  
  const { connected, sendMessage } = useWebSocket({
    url: (import.meta as any).env?.VITE_WS_URL || 'ws://localhost:8080/ws',
    onMessage: (data) => {
      if (data.type === 'system_status') {
        setSystemStatus(data.payload)
      }
    },
  })

  return (
    <div className="app">
      <header className="app-header">
        <h1>🔧 Encabulation Control Console</h1>
        <div className="connection-status">
          <span className={`status-indicator ${connected ? 'connected' : 'disconnected'}`} />
          {connected ? 'Connected' : 'Disconnected'}
        </div>
      </header>
      
      <div className="app-content">
        <div className="main-panel">
          <SystemTopology services={systemStatus.services} />
        </div>
        
        <div className="side-panel">
          <ControlPanel onAction={sendMessage} />
          <MetricsPanel metrics={systemStatus.metrics} />
        </div>
      </div>
    </div>
  )
}

export default App
