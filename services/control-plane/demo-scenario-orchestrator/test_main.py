import pytest
from fastapi.testclient import TestClient
from main import app, SCENARIOS

client = TestClient(app)


def test_health_endpoint():
    """Test health check endpoint"""
    response = client.get("/health")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert "timestamp" in data


def test_list_scenarios():
    """Test listing available scenarios"""
    response = client.get("/scenarios")
    assert response.status_code == 200
    data = response.json()
    assert "scenarios" in data
    assert len(data["scenarios"]) == 4
    
    scenario_names = [s["name"] for s in data["scenarios"]]
    assert "System Startup" in scenario_names
    assert "Data Flow Demo" in scenario_names
    assert "Failure Recovery" in scenario_names
    assert "Auto-scaling Demo" in scenario_names


def test_get_scenario():
    """Test getting scenario details"""
    response = client.get("/scenarios/startup")
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == "System Startup"
    assert "steps" in data
    assert len(data["steps"]) > 0


def test_get_nonexistent_scenario():
    """Test getting a scenario that doesn't exist"""
    response = client.get("/scenarios/nonexistent")
    assert response.status_code == 404


def test_scenario_status_idle():
    """Test scenario status when no scenario is running"""
    response = client.get("/scenarios/status")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "idle"


def test_scenarios_structure():
    """Test that all scenarios have required structure"""
    for scenario_name, scenario in SCENARIOS.items():
        assert scenario.name is not None
        assert scenario.description is not None
        assert len(scenario.steps) > 0
        
        for step in scenario.steps:
            assert step.action is not None
            assert step.target is not None
            assert isinstance(step.parameters, dict)
            assert isinstance(step.delay, int)
            assert step.delay >= 0
