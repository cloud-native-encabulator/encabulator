package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/sinusoidal-recorder/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/sinusoidal-recorder/internal/influx"
)

// fakeRecorder records what it was asked to do.
type fakeRecorder struct {
	written    []influx.Reading
	lastSerial string
	lastLimit  int
	lastWindow time.Duration
	readings   []influx.Reading
	summary    []influx.MetricSummary
	err        error
}

func (f *fakeRecorder) Write(_ context.Context, readings []influx.Reading) error {
	if f.err != nil {
		return f.err
	}
	f.written = append(f.written, readings...)
	return nil
}

func (f *fakeRecorder) ReadingsBySerial(_ context.Context, serial string, limit int) ([]influx.Reading, error) {
	f.lastSerial, f.lastLimit = serial, limit
	if f.err != nil {
		return nil, f.err
	}
	return f.readings, nil
}

func (f *fakeRecorder) Summary(_ context.Context, window time.Duration) ([]influx.MetricSummary, error) {
	f.lastWindow = window
	if f.err != nil {
		return nil, f.err
	}
	return f.summary, nil
}

func newServer(t *testing.T, r api.Recorder) *httpx.Server {
	t.Helper()
	srv := httpx.NewServer(httpx.ServerConfig{
		Service: "sinusoidal-recorder", Addr: "127.0.0.1:0",
		Logger: slog.New(slog.DiscardHandler), Registry: prometheus.NewRegistry(),
	})
	api.New(r, slog.New(slog.DiscardHandler), srv.Registry()).Routes(srv)
	return srv
}

func do(t *testing.T, srv *httpx.Server, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func postReadings(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/v1/readings", strings.NewReader(body))
}

func reading(serial, metric string, value float64) influx.Reading {
	return influx.Reading{
		Serial: serial, OrderID: "ord_1", Model: "turbo", Stage: "fit-marzlevanes",
		Metric: metric, Value: value, Time: time.Now().UTC(),
	}
}

func batchJSON(t *testing.T, readings ...influx.Reading) string {
	t.Helper()
	b, err := json.Marshal(api.WriteRequest{Readings: readings})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteReadings(t *testing.T) {
	fr := &fakeRecorder{}
	srv := newServer(t, fr)

	body := batchJSON(t,
		reading("ENC-a", "repleneration", 1.01),
		reading("ENC-a", "marzlevane_alignment", -0.2),
		reading("ENC-a", "grammeter_drift", 3.5),
	)
	rec := do(t, srv, postReadings(body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var resp api.WriteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Written != 3 {
		t.Errorf("written = %d", resp.Written)
	}
	if len(fr.written) != 3 {
		t.Fatalf("recorder received %d readings", len(fr.written))
	}
	if fr.written[0].Serial != "ENC-a" || fr.written[0].Metric != "repleneration" {
		t.Errorf("reading = %+v", fr.written[0])
	}
}

func TestWriteReadingsValidation(t *testing.T) {
	srv := newServer(t, &fakeRecorder{})

	oversized := make([]influx.Reading, influx.MaxBatch+1)
	for i := range oversized {
		oversized[i] = reading("ENC-a", "repleneration", 1)
	}

	cases := []struct {
		name, body string
	}{
		{"empty batch", `{"readings":[]}`},
		{"no readings field", `{}`},
		{"over the batch ceiling", batchJSON(t, oversized...)},
		{"missing serial", batchJSON(t, influx.Reading{Metric: "repleneration", Stage: "s", Value: 1})},
		{"missing metric", batchJSON(t, influx.Reading{Serial: "ENC-a", Stage: "s", Value: 1})},
		{"missing stage", batchJSON(t, influx.Reading{Serial: "ENC-a", Metric: "m", Value: 1})},
		{"unknown field", `{"readings":[],"nonsense":true}`},
		{"not json", `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, srv, postReadings(tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
			}
			var env httpx.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if env.Error.Code == "" {
				t.Error("error has no code")
			}
		})
	}
}

func TestWriteReadingsRejectsNonFiniteValues(t *testing.T) {
	srv := newServer(t, &fakeRecorder{})
	// NaN and Inf cannot survive JSON, so drive Validate directly for those and
	// check the handler rejects the string forms a buggy client might send.
	for _, body := range []string{
		`{"readings":[{"serial":"ENC-a","metric":"m","stage":"s","value":"NaN"}]}`,
		`{"readings":[{"serial":"ENC-a","metric":"m","stage":"s","value":1e999}]}`,
	} {
		if rec := do(t, srv, postReadings(body)); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s -> %d, want 400", body, rec.Code)
		}
	}
	if err := (influx.Reading{Serial: "a", Metric: "m", Stage: "s", Value: math.NaN()}).Validate(); err == nil {
		t.Error("NaN should not validate")
	}
	if err := (influx.Reading{Serial: "a", Metric: "m", Stage: "s",
		Value: math.Inf(1)}).Validate(); err == nil {
		t.Error("Inf should not validate")
	}
}

func TestWriteFailureIs503(t *testing.T) {
	fr := &fakeRecorder{err: errors.New("influxdb is asleep")}
	srv := newServer(t, fr)

	rec := do(t, srv, postReadings(batchJSON(t, reading("ENC-a", "repleneration", 1))))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "asleep") {
		t.Error("internal error detail leaked to the client")
	}
}

func TestGetReadings(t *testing.T) {
	fr := &fakeRecorder{readings: []influx.Reading{
		reading("ENC-a", "repleneration", 1.0),
		reading("ENC-a", "repleneration", 1.02),
	}}
	srv := newServer(t, fr)

	rec := do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/readings?serial=ENC-a", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp api.ReadingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Serial != "ENC-a" || len(resp.Readings) != 2 {
		t.Errorf("response = %+v", resp)
	}
	if fr.lastLimit != influx.DefaultReadLimit {
		t.Errorf("default limit = %d, want %d", fr.lastLimit, influx.DefaultReadLimit)
	}
}

func TestGetReadingsRequiresSerial(t *testing.T) {
	srv := newServer(t, &fakeRecorder{})
	for _, target := range []string{"/v1/readings", "/v1/readings?serial=", "/v1/readings?serial=%20"} {
		rec := do(t, srv, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", target, rec.Code)
		}
	}
}

func TestGetReadingsLimitBounds(t *testing.T) {
	fr := &fakeRecorder{}
	srv := newServer(t, fr)

	for _, target := range []string{
		"/v1/readings?serial=ENC-a&limit=0",
		"/v1/readings?serial=ENC-a&limit=-1",
		"/v1/readings?serial=ENC-a&limit=abc",
		fmt.Sprintf("/v1/readings?serial=ENC-a&limit=%d", influx.MaxReadLimit+1),
	} {
		if rec := do(t, srv, httptest.NewRequest(http.MethodGet, target, nil)); rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", target, rec.Code)
		}
	}

	do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/readings?serial=ENC-a&limit=42", nil))
	if fr.lastLimit != 42 {
		t.Errorf("limit = %d", fr.lastLimit)
	}
}

func TestGetReadingsEmptyIsAnEmptyArray(t *testing.T) {
	srv := newServer(t, &fakeRecorder{})
	rec := do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/readings?serial=ENC-nothing", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"readings":[]`) {
		t.Errorf("empty result should be [] not null: %s", rec.Body)
	}
}

func TestGetSummary(t *testing.T) {
	fr := &fakeRecorder{summary: []influx.MetricSummary{
		{Metric: "grammeter_drift", Count: 200, Mean: 0.4, P95: 3.2},
		{Metric: "repleneration", Count: 200, Mean: 1.0, P95: 1.1},
	}}
	srv := newServer(t, fr)

	rec := do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/summary", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp api.SummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Metrics) != 2 {
		t.Fatalf("metrics = %+v", resp.Metrics)
	}
	if fr.lastWindow != api.DefaultSummaryWindow {
		t.Errorf("default window = %v, want %v", fr.lastWindow, api.DefaultSummaryWindow)
	}
	if resp.Window != "5m0s" {
		t.Errorf("window = %q", resp.Window)
	}
	if time.Since(resp.Since) < 4*time.Minute {
		t.Errorf("since = %v, should be about five minutes ago", resp.Since)
	}
}

func TestGetSummaryWindow(t *testing.T) {
	fr := &fakeRecorder{}
	srv := newServer(t, fr)

	do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/summary?window=90s", nil))
	if fr.lastWindow != 90*time.Second {
		t.Errorf("window = %v", fr.lastWindow)
	}

	for _, bad := range []string{"0s", "500ms", "48h", "banana", "-5m"} {
		rec := do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/summary?window="+bad, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("window %q -> %d, want 400", bad, rec.Code)
		}
	}
}

func TestReadFailureIs503(t *testing.T) {
	fr := &fakeRecorder{err: errors.New("flight is grounded")}
	srv := newServer(t, fr)

	for _, target := range []string{"/v1/readings?serial=ENC-a", "/v1/summary"} {
		rec := do(t, srv, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s -> %d, want 503", target, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "grounded") {
			t.Error("internal error detail leaked to the client")
		}
	}
}

func TestParseWindow(t *testing.T) {
	if got, err := influx.ParseWindow("", 5*time.Minute); err != nil || got != 5*time.Minute {
		t.Errorf("empty window = %v, %v", got, err)
	}
	if got, err := influx.ParseWindow("1h", time.Minute); err != nil || got != time.Hour {
		t.Errorf("1h = %v, %v", got, err)
	}
	for _, bad := range []string{"0s", "25h", "nope"} {
		if _, err := influx.ParseWindow(bad, time.Minute); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestMetricsAreRegistered(t *testing.T) {
	srv := newServer(t, &fakeRecorder{})
	do(t, srv, postReadings(batchJSON(t, reading("ENC-a", "repleneration", 1))))
	do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/summary", nil))

	body := do(t, srv, httptest.NewRequest(http.MethodGet, "/metrics", nil)).Body.String()
	for _, want := range []string{
		"encabulator_readings_written_total 1",
		"encabulator_readings_write_seconds",
		`encabulator_recorder_queries_total{endpoint="summary",result="ok"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics is missing %q", want)
		}
	}
}
