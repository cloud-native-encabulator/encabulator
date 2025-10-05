package health

import (
	"encoding/json"
	"net/http"
	"time"
)

type HealthCheck struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

func LivenessHandler(w http.ResponseWriter, r *http.Request) {
	health := HealthCheck{
		Status:    "healthy",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

func ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	// Gateway is ready if it's running
	health := HealthCheck{
		Status:    "healthy",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}
