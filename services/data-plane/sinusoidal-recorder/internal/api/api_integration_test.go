package api_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/testsupport"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/sinusoidal-recorder/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/sinusoidal-recorder/internal/influx"
)

// TestRecorderEndToEnd drives the real HTTP API against a real InfluxDB 3 Core:
// a stage's worth of readings in, the same readings back out, and a summary over
// them.
func TestRecorderEndToEnd(t *testing.T) {
	port := testsupport.StartContainer(t, testsupport.ContainerSpec{
		Image: "influxdb:3-core",
		Cmd: []string{
			"influxdb3", "serve",
			"--node-id=recorder-api-test",
			"--object-store=memory",
			"--without-auth",
		},
		Port: 8181,
		Wait: wait.ForHTTP("/health").WithPort("8181/tcp").WithStartupTimeout(3 * time.Minute),
	})

	client, err := influx.New(influx.Config{
		URL:      fmt.Sprintf("http://127.0.0.1:%d", port),
		Database: "sinusoidal",
		Logger:   slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx := testsupport.Context(t)
	if err := client.EnsureDatabase(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.EnsureTable(ctx); err != nil {
		t.Fatal(err)
	}

	srv := httpx.NewServer(httpx.ServerConfig{
		Service: "sinusoidal-recorder", Addr: "127.0.0.1:0",
		Logger: slog.New(slog.DiscardHandler), Registry: prometheus.NewRegistry(),
	})
	srv.AddReadyCheck("influxdb", client.Health)
	api.New(client, slog.New(slog.DiscardHandler), srv.Registry()).Routes(srv)

	// Readiness must be green against a live server.
	if rec := do(t, srv, httptest.NewRequest(http.MethodGet, "/readyz", nil)); rec.Code != http.StatusOK {
		t.Fatalf("/readyz = %d body=%s", rec.Code, rec.Body)
	}

	// One stage: 20 samples of three metrics, 10ms apart, as the casing line sends.
	base := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Millisecond)
	readings := make([]influx.Reading, 0, 60)
	for i := range 20 {
		at := base.Add(time.Duration(i) * 10 * time.Millisecond)
		for _, m := range []struct {
			name  string
			value float64
		}{
			{"repleneration", 1.0},
			{"marzlevane_alignment", 0.1},
			{"grammeter_drift", float64(i)},
		} {
			readings = append(readings, influx.Reading{
				Serial: "ENC-turbo-e2e", OrderID: "ord_e2e", Model: "turbo",
				Stage: "fit-marzlevanes", Metric: m.name, Value: m.value, Time: at,
			})
		}
	}
	body, err := json.Marshal(api.WriteRequest{Readings: readings})
	if err != nil {
		t.Fatal(err)
	}
	rec := do(t, srv, httptest.NewRequest(http.MethodPost, "/v1/readings", strings.NewReader(string(body))))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /v1/readings = %d body=%s", rec.Code, rec.Body)
	}

	// Read them back through the API.
	var readBack api.ReadingsResponse
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		rec = do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/readings?serial=ENC-turbo-e2e", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v1/readings = %d body=%s", rec.Code, rec.Body)
		}
		readBack = api.ReadingsResponse{}
		if err := json.Unmarshal(rec.Body.Bytes(), &readBack); err != nil {
			t.Fatal(err)
		}
		if len(readBack.Readings) >= 60 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if len(readBack.Readings) != 60 {
		t.Fatalf("read back %d readings, want 60", len(readBack.Readings))
	}
	if readBack.Serial != "ENC-turbo-e2e" {
		t.Errorf("serial = %q", readBack.Serial)
	}
	for i := 1; i < len(readBack.Readings); i++ {
		if readBack.Readings[i].Time.Before(readBack.Readings[i-1].Time) {
			t.Fatal("readings came back out of time order")
		}
	}

	// Summarize.
	rec = do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/summary?window=5m", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/summary = %d body=%s", rec.Code, rec.Body)
	}
	var summary api.SummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Metrics) != 3 {
		t.Fatalf("summary = %+v, want three metrics", summary.Metrics)
	}
	for _, m := range summary.Metrics {
		if m.Count != 20 {
			t.Errorf("%s count = %d, want 20", m.Metric, m.Count)
		}
	}
}

// TestReadingsBeforeAnyWrite proves the endpoints work from the moment the works
// opens, which they only do because the table is created at startup.
func TestReadingsBeforeAnyWrite(t *testing.T) {
	port := testsupport.StartContainer(t, testsupport.ContainerSpec{
		Image: "influxdb:3-core",
		Cmd: []string{
			"influxdb3", "serve",
			"--node-id=recorder-empty-test",
			"--object-store=memory",
			"--without-auth",
		},
		Port: 8181,
		Wait: wait.ForHTTP("/health").WithPort("8181/tcp").WithStartupTimeout(3 * time.Minute),
	})

	client, err := influx.New(influx.Config{
		URL:      fmt.Sprintf("http://127.0.0.1:%d", port),
		Database: "sinusoidal",
		Logger:   slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx := testsupport.Context(t)
	if err := client.EnsureDatabase(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.EnsureTable(ctx); err != nil {
		t.Fatal(err)
	}

	srv := httpx.NewServer(httpx.ServerConfig{
		Service: "sinusoidal-recorder", Addr: "127.0.0.1:0",
		Logger: slog.New(slog.DiscardHandler), Registry: prometheus.NewRegistry(),
	})
	api.New(client, slog.New(slog.DiscardHandler), srv.Registry()).Routes(srv)

	rec := do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/summary", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/summary on an empty database = %d body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"metrics":[]`) {
		t.Errorf("summary should be an empty array: %s", rec.Body)
	}

	rec = do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/readings?serial=ENC-nothing", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/readings on an empty database = %d body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"readings":[]`) {
		t.Errorf("readings should be an empty array: %s", rec.Body)
	}
}
