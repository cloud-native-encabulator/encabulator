# Cloud Native Encabulator - Testing Guide

Quick reference for testing the system after deployment.

## Prerequisites

```bash
# Ensure you're in the correct cluster context
kubectl config current-context

# Verify all services are running
kubectl get pods -A | grep encabulator
```

## Access the Control Console

```bash
# Port-forward to control console
kubectl port-forward -n encabulator-control svc/encabulation-control-console 8080:8080

# Open in browser
open http://localhost:8080
```

## Generate Traffic

### Option 1: Via UI
1. Open control console at http://localhost:8080
2. Toggle traffic mode to "Real" (⚡ button in top-right)
3. Use the Traffic Control Panel to start traffic
4. Watch particles flow through the system

### Option 2: Via API

```bash
# Port-forward to Cardinal Grammeter
kubectl port-forward -n encabulator-control svc/cardinal-grammeter 8081:8080

# Start low traffic (50 TPS)
curl -X POST http://localhost:8081/traffic/start \
  -H "Content-Type: application/json" \
  -d '{
    "target": "http://dingle-arm-api-gateway.encabulator-processing.svc.cluster.local:8080/api/v1/encabulations",
    "pattern": "steady",
    "fluorescent_score_motion": 50
  }'

# Start high traffic (1000 TPS)
curl -X POST http://localhost:8081/traffic/start \
  -H "Content-Type: application/json" \
  -d '{
    "target": "http://dingle-arm-api-gateway.encabulator-processing.svc.cluster.local:8080/api/v1/encabulations",
    "pattern": "steady",
    "fluorescent_score_motion": 1000
  }'

# Check status
curl http://localhost:8081/traffic/status

# Stop traffic
curl -X POST http://localhost:8081/traffic/stop
```

## Verify Data Flow

### Check PostgreSQL Data

```bash
# Port-forward to Hydrocoptic DataStore
kubectl port-forward -n encabulator-data svc/hydrocoptic-datastore 8082:8080

# View recent encabulations
curl http://localhost:8082/api/v1/encabulations?pageSize=10
```

### Check Redis Cache

```bash
# Port-forward to Prefabulated Cache
kubectl port-forward -n encabulator-data svc/prefabulated-cache 8083:8080

# Check cache stats
curl http://localhost:8083/health
```

### Check InfluxDB Metrics

```bash
# Port-forward to Sinusoidal Database
kubectl port-forward -n encabulator-data svc/sinusoidal-database 8084:8080

# Check metrics
curl http://localhost:8084/health
```

## Monitor System Metrics

### Via Prometheus

```bash
# Port-forward to Prometheus
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090

# Open Prometheus UI
open http://localhost:9090

# Example queries:
# - sum(rate(encabulations_processed_total[1m]))
# - histogram_quantile(0.99, rate(encabulation_processing_duration_seconds_bucket[1m]))
```

### Via Grafana

```bash
# Port-forward to Grafana
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80

# Open Grafana (admin/prom-operator)
open http://localhost:3000
```

## Test Auto-Scaling

### Watch HPA Scale Services

```bash
# Watch HPA status
watch kubectl get hpa -A

# Generate high load
curl -X POST http://localhost:8081/traffic/start \
  -H "Content-Type: application/json" \
  -d '{
    "target": "http://dingle-arm-api-gateway.encabulator-processing.svc.cluster.local:8080/api/v1/encabulations",
    "pattern": "steady",
    "fluorescent_score_motion": 2000
  }'

# Watch pods scale up
watch kubectl get pods -n encabulator-processing

# Stop traffic and watch scale down
curl -X POST http://localhost:8081/traffic/stop
```

## Expected Behavior

### Responsiveness
- **Start traffic**: Particles appear within 2-4 seconds
- **Stop traffic**: Particles stop within 45-50 seconds (Prometheus rate window)
- **Metrics updates**: Every 2 seconds

### Traffic Visualization
- **Low traffic (1-50 TPS)**: 1-3 particles per connection
- **Medium traffic (50-500 TPS)**: 3-8 particles per connection
- **High traffic (500+ TPS)**: 8-15 particles per connection

### Auto-Scaling
- **Idle system**: 1 replica per service (~1.5 cores total)
- **Under load**: Scales up to 10-20 replicas based on CPU
- **Scale down**: Takes 5-10 minutes after load stops (HPA cooldown)

### Data Flow
- Traffic flows: Cardinal → Dingle Arm → Panametric Fam → Data Services
- Data services: PostgreSQL (writes), Redis (cache), InfluxDB (metrics)
- Particle colors: Green (healthy), Yellow (elevated latency), Red (errors)

## Troubleshooting

### No Particles Showing
1. Check traffic mode is set to "Real" (⚡ button)
2. Verify traffic is actually running: `curl http://localhost:8081/traffic/status`
3. Check backend logs: `kubectl logs -n encabulator-control deployment/encabulation-control-console --tail=50`
4. Verify Prometheus is accessible: `kubectl get pods -n encabulator-monitoring | grep prometheus`

### Services Not Scaling
1. Check HPA is configured: `kubectl get hpa -A`
2. Verify metrics-server is running: `kubectl top nodes`
3. Check CPU usage: `kubectl top pods -n encabulator-processing`
4. Review HPA events: `kubectl describe hpa -n encabulator-processing`

### Port-Forward Died
- Port-forwards can timeout after inactivity
- Simply re-run the port-forward command
- Consider using `kubectl port-forward --address 0.0.0.0` for remote access

## Clean Up

```bash
# Stop all traffic
curl -X POST http://localhost:8081/traffic/stop

# Kill all port-forwards
pkill -f "port-forward"

# Optional: Scale down manually
kubectl scale deployment --all --replicas=1 -n encabulator-processing
kubectl scale deployment --all --replicas=1 -n encabulator-data
```

## Performance Benchmarks

### Observed Performance
- **Peak TPS**: 1400+ encabulations/second
- **Processing Latency**: 3-4ms average (90ms+ under high load)
- **Success Rate**: 99.99%+
- **HPA Scaling**: 1-5 replicas based on CPU (70-80% threshold)

### Resource Usage
- **Idle**: ~1.5 cores, ~2GB memory (8 replicas total)
- **Under Load**: ~4-6 cores, ~4GB memory (20-30 replicas)
- **Savings**: 70% reduction vs previous configuration
