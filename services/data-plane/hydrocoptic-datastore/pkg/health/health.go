package health

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
)

type HealthCheck struct {
	Status    Status            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

type Checker interface {
	Check() error
}

type HealthHandler struct {
	checkers map[string]Checker
	mu       sync.RWMutex
}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		checkers: make(map[string]Checker),
	}
}

func (h *HealthHandler) AddChecker(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = checker
}

func (h *HealthHandler) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	// Liveness just checks if the service is running
	health := HealthCheck{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

func (h *HealthHandler) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	checks := make(map[string]string)
	allHealthy := true

	for name, checker := range h.checkers {
		if err := checker.Check(); err != nil {
			checks[name] = err.Error()
			allHealthy = false
		} else {
			checks[name] = "ok"
		}
	}

	status := StatusHealthy
	httpStatus := http.StatusOK
	if !allHealthy {
		status = StatusUnhealthy
		httpStatus = http.StatusServiceUnavailable
	}

	health := HealthCheck{
		Status:    status,
		Timestamp: time.Now(),
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(health)
}

type DBChecker struct {
	pinger interface{ Ping() error }
}

func NewDBChecker(pinger interface{ Ping() error }) *DBChecker {
	return &DBChecker{pinger: pinger}
}

func (d *DBChecker) Check() error {
	return d.pinger.Ping()
}
