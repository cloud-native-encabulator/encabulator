# Control Plane Services

The Control Plane provides interactive system control, demonstration capabilities, chaos engineering, and advanced traffic simulation for the Cloud Native Encabulator system.

## Services

### 1. Encabulation Control Console
**Technology**: React + TypeScript (Frontend), Node.js + Express (Backend)  
**Port**: 8080  
**Purpose**: Interactive web interface for system control and monitoring

**Features**:
- Real-time system topology visualization with D3.js
- Service scaling controls
- Failure injection interface
- Demo scenario execution
- Live metrics dashboard
- WebSocket-based real-time updates

**Key Endpoints**:
- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /metrics` - Prometheus metrics
- `WS /ws` - WebSocket for real-time communication

### 2. Malaise Injection Engine
**Technology**: Go  
**Port**: 8080  
**Purpose**: Chaos engineering and failure injection

**Features**:
- Pod failure injection (terminates pods)
- Network latency injection (planned)
- Resource stress injection (planned)
- Kubernetes-native operations
- Safety mechanisms and validation

**Key Endpoints**:
- `POST /inject` - Inject failure
- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /metrics` - Prometheus metrics

**Example**:
```bash
curl -X POST http://malaise-injection-engine:8080/inject \
  -H "Content-Type: application/json" \
  -d '{
    "type": "pod_failure",
    "target": "panametric-fam-processor",
    "duration": 60
  }'
```

### 3. Cardinal Grammeter Engine
**Technology**: Node.js + TypeScript  
**Port**: 8080  
**Purpose**: Advanced traffic simulation and load generation

**Features**:
- Multiple traffic patterns (steady, burst, ramp, sine)
- Configurable parameters using Encabulator terminology
- Real-time traffic metrics
- Dynamic parameter adjustment

**Traffic Parameters**:
- **Magneto Reluctance** (0-100): Controls request frequency
- **Capacitive Diractance** (0-100): Controls request complexity

**Key Endpoints**:
- `POST /traffic/start` - Start traffic generation
- `POST /traffic/stop` - Stop traffic generation
- `GET /traffic/status` - Get traffic status
- `POST /traffic/update` - Update parameters
- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics

**Example**:
```bash
curl -X POST http://cardinal-grammeter:8080/traffic/start \
  -H "Content-Type: application/json" \
  -d '{
    "target": "http://dingle-arm-api-gateway:8080/api/encabulate",
    "pattern": "steady",
    "magnetoReluctance": 50,
    "capacitiveDiractance": 50
  }'
```

### 4. Demo Scenario Orchestrator
**Technology**: Python + FastAPI  
**Port**: 8080  
**Purpose**: Automated demonstration scenario management

**Features**:
- Pre-built demonstration scenarios
- Multi-step scenario execution
- Integration with traffic generation and chaos engineering
- Real-time status tracking

**Predefined Scenarios**:
1. **System Startup**: Demonstrates startup sequence and health checks
2. **Data Flow Demo**: Shows data flowing through the system
3. **Failure Recovery**: Demonstrates resilience and recovery
4. **Auto-scaling Demo**: Shows horizontal pod autoscaling

**Key Endpoints**:
- `GET /scenarios` - List available scenarios
- `GET /scenarios/{name}` - Get scenario details
- `POST /scenarios/{name}/execute` - Execute scenario
- `GET /scenarios/status` - Get current status
- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /metrics` - Prometheus metrics

**Example**:
```bash
# List scenarios
curl http://demo-scenario-orchestrator:8080/scenarios

# Execute scenario
curl -X POST http://demo-scenario-orchestrator:8080/scenarios/failure-recovery/execute

# Check status
curl http://demo-scenario-orchestrator:8080/scenarios/status
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Control Plane                             │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │   Encabulation Control Console (React + Node.js)     │  │
│  │   - Web UI for system control                         │  │
│  │   - Real-time visualization                           │  │
│  │   - WebSocket communication                           │  │
│  └──────────────────────────────────────────────────────┘  │
│                          │                                   │
│         ┌────────────────┼────────────────┐                │
│         │                │                 │                │
│         ▼                ▼                 ▼                │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐      │
│  │  Malaise    │  │  Unilateral │  │    Demo      │      │
│  │  Injection  │  │   Phase     │  │  Scenario    │      │
│  │  Engine     │  │  Detractor  │  │ Orchestrator │      │
│  │   (Go)      │  │  (Node.js)  │  │  (Python)    │      │
│  └─────────────┘  └─────────────┘  └──────────────┘      │
│         │                │                 │                │
└─────────┼────────────────┼─────────────────┼────────────────┘
          │                │                 │
          ▼                ▼                 ▼
    ┌──────────────────────────────────────────┐
    │  Processing, Support, and Data Planes    │
    └──────────────────────────────────────────┘
```

## Service Integration

### Control Console → Other Services
- Sends commands to Malaise Injection Engine for failure injection
- Sends commands to Cardinal Grammeter for traffic generation
- Triggers Demo Scenario Orchestrator for automated demos
- Queries Kubernetes API for system status

### Demo Scenario Orchestrator → Other Services
- Calls Cardinal Grammeter to generate traffic
- Calls Malaise Injection Engine to inject failures
- Monitors Kubernetes deployments for scaling events
- Coordinates multi-step demonstrations

## Deployment

All Control Plane services are deployed in the `encabulator-control` namespace with:
- Kubernetes Deployments
- Services for internal communication
- ServiceMonitors for Prometheus scraping
- Appropriate RBAC permissions

See individual service READMEs and Helm charts for detailed configuration.

## Development

Each service can be developed and tested independently:

```bash
# Encabulation Control Console
cd encabulation-control-console
npm install && npm run dev
cd server && npm install && npm run dev

# Malaise Injection Engine
cd malaise-injection-engine
go run main.go

# Cardinal Grammeter
cd cardinal-grammeter
npm install && npm run dev

# Demo Scenario Orchestrator
cd demo-scenario-orchestrator
pip install -r requirements.txt
python main.py
```

## Educational Value

The Control Plane demonstrates:
- Interactive system control and monitoring
- Chaos engineering practices
- Load testing and traffic simulation
- Automated demonstration workflows
- Real-time data visualization
- WebSocket communication
- Microservices orchestration

## License

Apache 2.0
