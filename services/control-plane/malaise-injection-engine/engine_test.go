package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	engine := &MalaiseEngine{}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	engine.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", response["status"])
	}

	if _, ok := response["timestamp"]; !ok {
		t.Error("Expected timestamp in response")
	}
}

func TestInjectFailureInvalidMethod(t *testing.T) {
	engine := &MalaiseEngine{}

	req := httptest.NewRequest(http.MethodGet, "/inject", nil)
	w := httptest.NewRecorder()

	engine.handleInjectFailure(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestInjectFailureInvalidJSON(t *testing.T) {
	engine := &MalaiseEngine{}

	req := httptest.NewRequest(http.MethodPost, "/inject", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	engine.handleInjectFailure(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestInjectFailureUnknownType(t *testing.T) {
	engine := &MalaiseEngine{}

	scenario := FailureScenario{
		Type:     "unknown_type",
		Target:   "test-service",
		Duration: 60,
	}

	body, _ := json.Marshal(scenario)
	req := httptest.NewRequest(http.MethodPost, "/inject", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	engine.handleInjectFailure(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	result := getEnv("NONEXISTENT_VAR", "default_value")
	if result != "default_value" {
		t.Errorf("Expected 'default_value', got '%s'", result)
	}
}

func TestFailureScenarioStructure(t *testing.T) {
	scenario := FailureScenario{
		Type:     "pod_failure",
		Target:   "test-service",
		Duration: 60,
	}

	if scenario.Type != "pod_failure" {
		t.Errorf("Expected type 'pod_failure', got '%s'", scenario.Type)
	}

	if scenario.Target != "test-service" {
		t.Errorf("Expected target 'test-service', got '%s'", scenario.Target)
	}

	if scenario.Duration != 60 {
		t.Errorf("Expected duration 60, got %d", scenario.Duration)
	}
}
