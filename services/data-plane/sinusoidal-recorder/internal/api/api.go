// Package api serves the sinusoidal recorder's HTTP API (design 8.8).
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-recorder/internal/influx"
)

// DefaultSummaryWindow is the window GET /v1/summary uses when none is given.
const DefaultSummaryWindow = 5 * time.Minute

// Recorder is the telemetry store the API needs.
type Recorder interface {
	Write(ctx context.Context, readings []influx.Reading) error
	ReadingsBySerial(ctx context.Context, serial string, limit int) ([]influx.Reading, error)
	Summary(ctx context.Context, window time.Duration) ([]influx.MetricSummary, error)
}

// API holds the handlers.
type API struct {
	recorder Recorder
	log      *slog.Logger

	written   prometheus.Counter
	writeTime prometheus.Histogram
	queries   *prometheus.CounterVec
}

// New builds the API and registers its domain metrics.
func New(r Recorder, log *slog.Logger, reg prometheus.Registerer) *API {
	a := &API{recorder: r, log: log}

	a.written = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "encabulator_readings_written_total",
		Help: "Telemetry samples written to InfluxDB.",
	})
	a.writeTime = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "encabulator_readings_write_seconds",
		Help:    "Duration of a batch write to InfluxDB.",
		Buckets: httpx.DurationBuckets,
	})
	a.queries = httpx.RegisterCounterVec(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "encabulator_recorder_queries_total",
		Help: "Telemetry reads, by endpoint and outcome.",
	}, []string{"endpoint", "result"}))

	if reg != nil {
		reg.MustRegister(a.written, a.writeTime)
	}
	return a
}

// Routes registers every route on the kit's HTTP server.
func (a *API) Routes(s *httpx.Server) {
	s.Handle("POST /v1/readings", http.HandlerFunc(a.WriteReadings))
	s.Handle("GET /v1/readings", http.HandlerFunc(a.GetReadings))
	s.Handle("GET /v1/summary", http.HandlerFunc(a.GetSummary))
}

// WriteRequest is the body of POST /v1/readings.
type WriteRequest struct {
	Readings []influx.Reading `json:"readings"`
}

// WriteResponse acknowledges a batch.
type WriteResponse struct {
	Written int `json:"written"`
}

// WriteReadings serves POST /v1/readings.
func (a *API) WriteReadings(w http.ResponseWriter, r *http.Request) {
	var req WriteRequest
	// Twenty samples of three metrics per stage, batched per stage, is small; a
	// 4 MiB ceiling leaves room for the maximum batch and nothing more.
	if err := httpx.DecodeJSON(r, &req, 4<<20); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := validateBatch(req.Readings); err != nil {
		a.fail(w, r, err)
		return
	}

	start := time.Now()
	if err := a.recorder.Write(r.Context(), req.Readings); err != nil {
		a.queries.WithLabelValues("write", "error").Inc()
		a.log.ErrorContext(r.Context(), "telemetry write failed",
			slog.Int("readings", len(req.Readings)), slog.Any("error", err))
		httpx.WriteError(w, http.StatusServiceUnavailable, httpx.CodeUnavailable,
			"The recorder could not store the readings")
		return
	}
	a.writeTime.Observe(time.Since(start).Seconds())
	a.written.Add(float64(len(req.Readings)))
	a.queries.WithLabelValues("write", "ok").Inc()

	httpx.WriteJSON(w, http.StatusAccepted, WriteResponse{Written: len(req.Readings)})
}

func validateBatch(readings []influx.Reading) error {
	if len(readings) == 0 {
		return httpx.Errorf(http.StatusBadRequest, "invalid_readings", "readings must not be empty")
	}
	if len(readings) > influx.MaxBatch {
		return httpx.Errorf(http.StatusBadRequest, "invalid_readings",
			"a batch holds at most %d readings, got %d", influx.MaxBatch, len(readings))
	}
	for i, reading := range readings {
		if err := reading.Validate(); err != nil {
			return httpx.Errorf(http.StatusBadRequest, "invalid_readings",
				"readings[%d]: %v", i, err)
		}
	}
	return nil
}

// ReadingsResponse is the body of GET /v1/readings.
type ReadingsResponse struct {
	Serial   string           `json:"serial"`
	Readings []influx.Reading `json:"readings"`
}

// GetReadings serves GET /v1/readings?serial=...&limit=.
func (a *API) GetReadings(w http.ResponseWriter, r *http.Request) {
	serial := strings.TrimSpace(r.URL.Query().Get("serial"))
	if serial == "" {
		a.fail(w, r, httpx.Errorf(http.StatusBadRequest, "missing_serial", "serial is required"))
		return
	}
	if len(serial) > 128 {
		a.fail(w, r, httpx.Errorf(http.StatusBadRequest, "invalid_serial",
			"serial must be 128 characters or fewer"))
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		a.fail(w, r, err)
		return
	}

	readings, err := a.recorder.ReadingsBySerial(r.Context(), serial, limit)
	if err != nil {
		a.queries.WithLabelValues("readings", "error").Inc()
		a.log.ErrorContext(r.Context(), "telemetry read failed",
			slog.String("serial", serial), slog.Any("error", err))
		httpx.WriteError(w, http.StatusServiceUnavailable, httpx.CodeUnavailable,
			"The recorder could not read the readings")
		return
	}
	a.queries.WithLabelValues("readings", "ok").Inc()
	if readings == nil {
		readings = []influx.Reading{}
	}
	httpx.WriteJSON(w, http.StatusOK, ReadingsResponse{Serial: serial, Readings: readings})
}

// SummaryResponse is the body of GET /v1/summary.
type SummaryResponse struct {
	Window  string                 `json:"window"`
	Since   time.Time              `json:"since"`
	Metrics []influx.MetricSummary `json:"metrics"`
}

// GetSummary serves GET /v1/summary?window=5m.
func (a *API) GetSummary(w http.ResponseWriter, r *http.Request) {
	window, err := influx.ParseWindow(r.URL.Query().Get("window"), DefaultSummaryWindow)
	if err != nil {
		a.fail(w, r, httpx.Errorf(http.StatusBadRequest, "invalid_window", "%v", err))
		return
	}

	metrics, err := a.recorder.Summary(r.Context(), window)
	if err != nil {
		a.queries.WithLabelValues("summary", "error").Inc()
		a.log.ErrorContext(r.Context(), "telemetry summary failed",
			slog.Duration("window", window), slog.Any("error", err))
		httpx.WriteError(w, http.StatusServiceUnavailable, httpx.CodeUnavailable,
			"The recorder could not summarize the readings")
		return
	}
	a.queries.WithLabelValues("summary", "ok").Inc()
	if metrics == nil {
		metrics = []influx.MetricSummary{}
	}
	httpx.WriteJSON(w, http.StatusOK, SummaryResponse{
		Window:  window.String(),
		Since:   time.Now().UTC().Add(-window),
		Metrics: metrics,
	})
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return influx.DefaultReadLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > influx.MaxReadLimit {
		return 0, httpx.Errorf(http.StatusBadRequest, "invalid_limit",
			"limit must be between 1 and %d", influx.MaxReadLimit)
	}
	return n, nil
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	var se *httpx.StatusError
	if errors.As(err, &se) {
		httpx.WriteErr(w, se)
		return
	}
	a.log.ErrorContext(r.Context(), "recorder request failed", slog.Any("error", err))
	httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "Internal error")
}
