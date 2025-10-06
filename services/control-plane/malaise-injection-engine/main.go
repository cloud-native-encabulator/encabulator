package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	failureInjections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "malaise_injections_total",
			Help: "Total number of failure injections",
		},
		[]string{"type", "target"},
	)
	activeFailures = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "malaise_active_failures",
			Help: "Number of currently active failures",
		},
	)
)

func init() {
	prometheus.MustRegister(failureInjections)
	prometheus.MustRegister(activeFailures)
}

type FailureScenario struct {
	Type     string `json:"type"`
	Target   string `json:"target"`
	Duration int    `json:"duration"` // seconds
}

type MalaiseEngine struct {
	clientset *kubernetes.Clientset
	namespace string
}

func NewMalaiseEngine() (*MalaiseEngine, error) {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &MalaiseEngine{
		clientset: clientset,
		namespace: getEnv("NAMESPACE", "encabulator-system"),
	}, nil
}

func (m *MalaiseEngine) InjectPodFailure(ctx context.Context, target string, duration int) error {
	log.Printf("Injecting pod failure: target=%s, duration=%ds", target, duration)

	// Get pods matching the target
	pods, err := m.clientset.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", target),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for target: %s", target)
	}

	// Delete the first pod to simulate failure
	podName := pods.Items[0].Name
	err = m.clientset.CoreV1().Pods(m.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	failureInjections.WithLabelValues("pod_failure", target).Inc()
	log.Printf("Deleted pod %s for failure injection", podName)

	return nil
}

func (m *MalaiseEngine) InjectNetworkLatency(ctx context.Context, target string, duration int) error {
	log.Printf("Injecting network latency: target=%s, duration=%ds", target, duration)

	// In a real implementation, this would use network policies or a service mesh
	// For now, we'll just log the action
	failureInjections.WithLabelValues("network_latency", target).Inc()

	return nil
}

func (m *MalaiseEngine) InjectResourceStress(ctx context.Context, target string, duration int) error {
	log.Printf("Injecting resource stress: target=%s, duration=%ds", target, duration)

	// In a real implementation, this would modify resource limits or inject stress
	failureInjections.WithLabelValues("resource_stress", target).Inc()

	return nil
}

func (m *MalaiseEngine) handleInjectFailure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var scenario FailureScenario
	if err := json.NewDecoder(r.Body).Decode(&scenario); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var err error

	switch scenario.Type {
	case "pod_failure":
		err = m.InjectPodFailure(ctx, scenario.Target, scenario.Duration)
	case "network_latency":
		err = m.InjectNetworkLatency(ctx, scenario.Target, scenario.Duration)
	case "resource_stress":
		err = m.InjectResourceStress(ctx, scenario.Target, scenario.Duration)
	default:
		http.Error(w, "Unknown failure type", http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Printf("Error injecting failure: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Injected %s failure into %s", scenario.Type, scenario.Target),
	})
}

func (m *MalaiseEngine) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (m *MalaiseEngine) handleReady(w http.ResponseWriter, r *http.Request) {
	// Check Kubernetes connectivity
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := m.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		http.Error(w, "Not ready", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	engine, err := NewMalaiseEngine()
	if err != nil {
		log.Fatalf("Failed to create Malaise Engine: %v", err)
	}

	http.HandleFunc("/inject", engine.handleInjectFailure)
	http.HandleFunc("/health", engine.handleHealth)
	http.HandleFunc("/ready", engine.handleReady)
	http.Handle("/metrics", promhttp.Handler())

	port := getEnv("PORT", "8080")
	log.Printf("Malaise Injection Engine starting on port %s", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
