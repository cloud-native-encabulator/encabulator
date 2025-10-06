import './MetricsPanel.css'

interface MetricsPanelProps {
  metrics: Record<string, number>
}

export default function MetricsPanel({ metrics }: MetricsPanelProps) {
  const formatMetric = (value: number, key: string): string => {
    if (key.includes('rate')) {
      return `${value.toFixed(2)}/s`
    }
    if (key.includes('latency')) {
      return `${value.toFixed(0)}ms`
    }
    if (key.includes('cpu') || key.includes('memory')) {
      return `${value.toFixed(1)}%`
    }
    return value.toFixed(2)
  }

  const metricEntries = Object.entries(metrics)

  return (
    <div className="metrics-panel">
      <h3>System Metrics</h3>
      
      {metricEntries.length === 0 ? (
        <div className="no-metrics">No metrics available</div>
      ) : (
        <div className="metrics-grid">
          {metricEntries.map(([key, value]) => (
            <div key={key} className="metric-card">
              <div className="metric-label">
                {key.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase())}
              </div>
              <div className="metric-value">
                {formatMetric(value, key)}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
