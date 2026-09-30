// Package metrics holds the grammeter's Prometheus surface: the domain metrics
// from design section 8.12 plus the conventional outbound HTTP histogram.
package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DurationBuckets are the histogram buckets fixed by design section 7.
var DurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Metrics is the grammeter's metric set.
type Metrics struct {
	reg       prometheus.Registerer
	target    string
	requests  *prometheus.CounterVec
	targetRPS prometheus.Gauge
	dropped   prometheus.Counter
	client    *prometheus.HistogramVec
}

// New registers the metrics with reg. target is the upstream service name that
// appears as the server_address label, normally "dingle-arm-gateway".
func New(reg prometheus.Registerer, target string) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	m := &Metrics{
		reg:    reg,
		target: target,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "encabulator_grammeter_requests_total",
			Help: "Gateway requests made by synthetic customers.",
		}, []string{"journey", "status"}),
		targetRPS: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "encabulator_grammeter_target_rps",
			Help: "Instantaneous target request rate, after the pattern is applied.",
		}),
		dropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "encabulator_grammeter_dropped_total",
			Help: "Journeys not started because the in-flight cap was reached.",
		}),
		// The grammeter builds no kit client, so it owns this histogram itself.
		client: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_client_request_duration_seconds",
			Help:    "Duration of outbound HTTP requests.",
			Buckets: DurationBuckets,
		}, []string{"http_request_method", "server_address", "http_response_status_code"}),
	}
	reg.MustRegister(m.requests, m.targetRPS, m.dropped, m.client)
	return m
}

// ObserveRequest counts one gateway call. It satisfies journey.Metrics.
func (m *Metrics) ObserveRequest(journey, status string) {
	m.requests.WithLabelValues(journey, status).Inc()
}

// ObserveClientRequest records one outbound HTTP call. It satisfies
// gateway.Observer.
func (m *Metrics) ObserveClientRequest(method string, status int, d time.Duration) {
	code := "error"
	if status > 0 {
		code = strconv.Itoa(status)
	}
	m.client.WithLabelValues(method, m.target, code).Observe(d.Seconds())
}

// TargetRPS returns the gauge the engine keeps current. It satisfies engine.Gauge.
func (m *Metrics) TargetRPS() prometheus.Gauge { return m.targetRPS }

// Dropped returns the counter the engine increments. It satisfies engine.Counter.
func (m *Metrics) Dropped() prometheus.Counter { return m.dropped }

// RegisterInFlight publishes an in-flight gauge backed by fn.
func (m *Metrics) RegisterInFlight(fn func() float64) {
	m.reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "encabulator_grammeter_in_flight",
		Help: "Journeys currently executing.",
	}, fn))
}
