package httpx

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// DurationBuckets are the histogram buckets required by design section 7.
var DurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// NewRegistry returns a registry pre-loaded with the Go runtime and process
// collectors ("the runtime's default process and runtime metrics").
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
}

// serverDuration returns the http_server_request_duration_seconds histogram for
// reg, creating it on first use.
func serverDuration(reg prometheus.Registerer) *prometheus.HistogramVec {
	return registerHistogram(reg, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "Duration of inbound HTTP requests.",
		Buckets: DurationBuckets,
	}, []string{"http_request_method", "http_route", "http_response_status_code"}))
}

// clientDuration returns the http_client_request_duration_seconds histogram for
// reg. Every outbound client in a process shares one histogram, distinguished by
// the server_address label.
func clientDuration(reg prometheus.Registerer) *prometheus.HistogramVec {
	return registerHistogram(reg, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_client_request_duration_seconds",
		Help:    "Duration of outbound HTTP requests.",
		Buckets: DurationBuckets,
	}, []string{"http_request_method", "server_address", "http_response_status_code"}))
}

// registerHistogram registers h, or returns the equivalent histogram already
// present in reg. A nil registerer means "metrics disabled"; the caller still
// gets a usable (unexported) histogram.
func registerHistogram(reg prometheus.Registerer, h *prometheus.HistogramVec) *prometheus.HistogramVec {
	if reg == nil {
		return h
	}
	err := reg.Register(h)
	if err == nil {
		return h
	}
	var already prometheus.AlreadyRegisteredError
	if errors.As(err, &already) {
		if existing, ok := already.ExistingCollector.(*prometheus.HistogramVec); ok {
			return existing
		}
	}
	panic(err)
}

// RegisterCounterVec registers c, or returns the equivalent counter already in
// reg. Services use it for their encabulator_* domain metrics so that repeated
// wiring in tests does not panic.
func RegisterCounterVec(reg prometheus.Registerer, c *prometheus.CounterVec) *prometheus.CounterVec {
	if reg == nil {
		return c
	}
	err := reg.Register(c)
	if err == nil {
		return c
	}
	var already prometheus.AlreadyRegisteredError
	if errors.As(err, &already) {
		if existing, ok := already.ExistingCollector.(*prometheus.CounterVec); ok {
			return existing
		}
	}
	panic(err)
}
