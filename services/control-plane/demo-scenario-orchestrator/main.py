import os
import time
import logging
from typing import Dict, List, Optional
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from prometheus_client import Counter, Gauge, generate_latest, CONTENT_TYPE_LATEST
from starlette.responses import Response
import httpx
from kubernetes import client, config

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(title="Demo Scenario Orchestrator")

# Prometheus metrics
scenario_executions = Counter(
    'scenario_executions_total',
    'Total number of scenario executions',
    ['scenario', 'status']
)
active_scenarios = Gauge(
    'scenario_active_scenarios',
    'Number of currently active scenarios'
)

# Kubernetes client
try:
    config.load_incluster_config()
except config.ConfigException:
    config.load_kube_config()

k8s_apps_v1 = client.AppsV1Api()


class ScenarioStep(BaseModel):
    action: str
    target: str
    parameters: Dict = {}
    delay: int = 0  # seconds


class Scenario(BaseModel):
    name: str
    description: str
    steps: List[ScenarioStep]


class ScenarioStatus(BaseModel):
    name: str
    status: str
    current_step: int
    total_steps: int
    message: str


# Predefined scenarios
SCENARIOS: Dict[str, Scenario] = {
    "startup": Scenario(
        name="System Startup",
        description="Demonstrates the system startup sequence",
        steps=[
            ScenarioStep(action="log", target="console", parameters={"message": "Starting Cloud Native Encabulator system..."}),
            ScenarioStep(action="check_services", target="all", delay=2),
            ScenarioStep(action="log", target="console", parameters={"message": "All services healthy"}),
        ]
    ),
    "data-flow": Scenario(
        name="Data Flow Demo",
        description="Demonstrates data flowing through the system",
        steps=[
            ScenarioStep(action="log", target="console", parameters={"message": "Starting data flow demonstration..."}),
            ScenarioStep(action="generate_traffic", target="dingle-arm-api-gateway", parameters={"pattern": "steady", "duration": 30}),
            ScenarioStep(action="log", target="console", parameters={"message": "Data flow complete"}),
        ]
    ),
    "failure-recovery": Scenario(
        name="Failure Recovery",
        description="Demonstrates system recovery from failures",
        steps=[
            ScenarioStep(action="log", target="console", parameters={"message": "Starting failure recovery demonstration..."}),
            ScenarioStep(action="inject_failure", target="panametric-fam-processor", parameters={"type": "pod_failure"}),
            ScenarioStep(action="wait", target="recovery", parameters={"timeout": 30}, delay=5),
            ScenarioStep(action="check_services", target="all"),
            ScenarioStep(action="log", target="console", parameters={"message": "System recovered successfully"}),
        ]
    ),
    "scaling": Scenario(
        name="Auto-scaling Demo",
        description="Demonstrates horizontal pod autoscaling",
        steps=[
            ScenarioStep(action="log", target="console", parameters={"message": "Starting auto-scaling demonstration..."}),
            ScenarioStep(action="generate_traffic", target="dingle-arm-api-gateway", parameters={"pattern": "ramp", "duration": 60}),
            ScenarioStep(action="monitor_scaling", target="panametric-fam-processor", parameters={"duration": 60}),
            ScenarioStep(action="log", target="console", parameters={"message": "Auto-scaling demonstration complete"}),
        ]
    ),
}

# Active scenario tracking
active_scenario: Optional[ScenarioStatus] = None


@app.get("/health")
async def health():
    """Health check endpoint"""
    return {"status": "healthy", "timestamp": time.time()}


@app.get("/ready")
async def ready():
    """Readiness check endpoint"""
    try:
        k8s_apps_v1.list_namespaced_deployment(namespace="encabulator-system", limit=1)
        return {"status": "ready"}
    except Exception as e:
        raise HTTPException(status_code=503, detail=f"Not ready: {str(e)}")


@app.get("/metrics")
async def metrics():
    """Prometheus metrics endpoint"""
    return Response(content=generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.get("/scenarios")
async def list_scenarios():
    """List available scenarios"""
    return {
        "scenarios": [
            {
                "name": scenario.name,
                "description": scenario.description,
                "steps": len(scenario.steps)
            }
            for scenario in SCENARIOS.values()
        ]
    }


@app.get("/scenarios/status")
async def get_scenario_status():
    """Get current scenario status"""
    if not active_scenario:
        return {"status": "idle", "message": "No scenario running"}
    
    return active_scenario


@app.get("/scenarios/{scenario_name}")
async def get_scenario(scenario_name: str):
    """Get scenario details"""
    if scenario_name not in SCENARIOS:
        raise HTTPException(status_code=404, detail="Scenario not found")
    
    return SCENARIOS[scenario_name]


@app.post("/scenarios/{scenario_name}/execute")
async def execute_scenario(scenario_name: str):
    """Execute a scenario"""
    global active_scenario
    
    if active_scenario and active_scenario.status == "running":
        raise HTTPException(status_code=409, detail="Another scenario is already running")
    
    if scenario_name not in SCENARIOS:
        raise HTTPException(status_code=404, detail="Scenario not found")
    
    scenario = SCENARIOS[scenario_name]
    active_scenario = ScenarioStatus(
        name=scenario.name,
        status="running",
        current_step=0,
        total_steps=len(scenario.steps),
        message="Starting scenario..."
    )
    
    active_scenarios.inc()
    
    try:
        for i, step in enumerate(scenario.steps):
            active_scenario.current_step = i + 1
            active_scenario.message = f"Executing step {i + 1}: {step.action}"
            
            logger.info(f"Executing step {i + 1}/{len(scenario.steps)}: {step.action} on {step.target}")
            
            await execute_step(step)
            
            if step.delay > 0:
                time.sleep(step.delay)
        
        active_scenario.status = "completed"
        active_scenario.message = "Scenario completed successfully"
        scenario_executions.labels(scenario=scenario_name, status="success").inc()
        
    except Exception as e:
        logger.error(f"Scenario execution failed: {str(e)}")
        active_scenario.status = "failed"
        active_scenario.message = f"Scenario failed: {str(e)}"
        scenario_executions.labels(scenario=scenario_name, status="failure").inc()
        raise HTTPException(status_code=500, detail=str(e))
    
    finally:
        active_scenarios.dec()
    
    return active_scenario


async def execute_step(step: ScenarioStep):
    """Execute a single scenario step"""
    if step.action == "log":
        logger.info(step.parameters.get("message", ""))
    
    elif step.action == "check_services":
        # Check service health
        namespace = os.getenv("NAMESPACE", "encabulator-system")
        deployments = k8s_apps_v1.list_namespaced_deployment(namespace=namespace)
        
        for deployment in deployments.items:
            replicas = deployment.status.replicas or 0
            ready_replicas = deployment.status.ready_replicas or 0
            
            if ready_replicas < replicas:
                raise Exception(f"Service {deployment.metadata.name} is not healthy")
    
    elif step.action == "generate_traffic":
        # Call Cardinal Grammeter Engine
        traffic_engine_url = os.getenv("TRAFFIC_ENGINE_URL", "http://cardinal-grammeter:8080")
        
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{traffic_engine_url}/traffic/start",
                json={
                    "target": f"http://{step.target}:8080/api/encabulate",
                    "pattern": step.parameters.get("pattern", "steady"),
                    "magnetoReluctance": 50,
                    "capacitiveDiractance": 50,
                },
                timeout=10.0
            )
            response.raise_for_status()
        
        # Wait for duration
        duration = step.parameters.get("duration", 10)
        time.sleep(duration)
        
        # Stop traffic
        async with httpx.AsyncClient() as client:
            await client.post(f"{traffic_engine_url}/traffic/stop", timeout=10.0)
    
    elif step.action == "inject_failure":
        # Call Malaise Injection Engine
        malaise_engine_url = os.getenv("MALAISE_ENGINE_URL", "http://malaise-injection-engine:8080")
        
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{malaise_engine_url}/inject",
                json={
                    "type": step.parameters.get("type", "pod_failure"),
                    "target": step.target,
                    "duration": step.parameters.get("duration", 60),
                },
                timeout=10.0
            )
            response.raise_for_status()
    
    elif step.action == "wait":
        timeout = step.parameters.get("timeout", 10)
        time.sleep(timeout)
    
    elif step.action == "monitor_scaling":
        # Monitor deployment scaling
        namespace = os.getenv("NAMESPACE", "encabulator-system")
        duration = step.parameters.get("duration", 30)
        start_time = time.time()
        
        while time.time() - start_time < duration:
            deployment = k8s_apps_v1.read_namespaced_deployment(
                name=step.target,
                namespace=namespace
            )
            replicas = deployment.status.replicas or 0
            logger.info(f"Current replicas for {step.target}: {replicas}")
            time.sleep(5)


if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("PORT", "8080"))
    uvicorn.run(app, host="0.0.0.0", port=port)
