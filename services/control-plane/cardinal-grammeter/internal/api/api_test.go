package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/engine"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/journey"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/traffic"
)

type nopRunner struct{}

func (nopRunner) Run(context.Context, string) {}

func newServer(t *testing.T) (*engine.Engine, http.Handler) {
	t.Helper()
	e, err := engine.New(engine.Options{
		Runner:      nopRunner{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Target:      "http://dingle-arm-gateway:8080",
		MaxInFlight: 128,
		Initial: engine.Config{
			RPS:       5,
			Customers: 50,
		},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	mux := http.NewServeMux()
	New(e, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
	return e, mux
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGetTraffic(t *testing.T) {
	_, h := newServer(t)
	rec := do(t, h, http.MethodGet, "/v1/traffic", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type = %q", ct)
	}

	var got engine.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Running {
		t.Error("a fresh grammeter must start stopped")
	}
	if got.RPS != 5 {
		t.Errorf("rps = %v, want 5", got.RPS)
	}
	if got.Pattern != traffic.PatternSteady {
		t.Errorf("pattern = %q, want steady", got.Pattern)
	}
	if got.Target != "http://dingle-arm-gateway:8080" {
		t.Errorf("target = %q", got.Target)
	}
	if got.MaxInFlight != 128 {
		t.Errorf("maxInFlight = %d, want 128", got.MaxInFlight)
	}
	if got.Customers != 50 {
		t.Errorf("customers = %d, want 50", got.Customers)
	}
	if len(got.Mix) != 4 {
		t.Errorf("mix = %v", got.Mix)
	}
	// The live block must be present even before any traffic has run.
	if got.Stats.Journeys == nil || got.Stats.Statuses == nil {
		t.Errorf("stats block is incomplete: %+v", got.Stats)
	}
}

func TestPutTrafficAppliesEveryField(t *testing.T) {
	e, h := newServer(t)
	body := `{"running":true,"rps":12.5,"pattern":"sine","periodSeconds":30,
	          "mix":{"browse":1,"order":3},"customers":10}`
	rec := do(t, h, http.MethodPut, "/v1/traffic", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	cfg := e.Config()
	if !cfg.Running || cfg.RPS != 12.5 || cfg.Pattern != traffic.PatternSine ||
		cfg.PeriodSeconds != 30 || cfg.Customers != 10 {
		t.Fatalf("configuration not applied: %+v", cfg)
	}
	if cfg.Mix[journey.Browse] != 1 || cfg.Mix[journey.Order] != 3 || len(cfg.Mix) != 2 {
		t.Errorf("mix = %v", cfg.Mix)
	}

	// The response echoes the new state.
	var got engine.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Running || got.RPS != 12.5 {
		t.Errorf("response did not echo the new state: %+v", got.Config)
	}
}

func TestPutTrafficIsAPartialUpdate(t *testing.T) {
	e, h := newServer(t)
	if rec := do(t, h, http.MethodPut, "/v1/traffic", `{"rps":42}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	cfg := e.Config()
	if cfg.RPS != 42 {
		t.Errorf("rps = %v, want 42", cfg.RPS)
	}
	if cfg.Customers != 50 || cfg.PeriodSeconds != 60 || cfg.Pattern != traffic.PatternSteady {
		t.Errorf("a partial update disturbed other fields: %+v", cfg)
	}

	// An empty body is a no-op, not an error.
	if rec := do(t, h, http.MethodPut, "/v1/traffic", `{}`); rec.Code != http.StatusOK {
		t.Errorf("empty patch status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutTrafficRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"malformed json":  `{`,
		"unknown field":   `{"target":"http://elsewhere"}`,
		"rps too high":    `{"rps":501}`,
		"rps too low":     `{"rps":0.01}`,
		"unknown pattern": `{"pattern":"chaos"}`,
		"bad period":      `{"periodSeconds":0}`,
		"unknown journey": `{"mix":{"sabotage":1}}`,
		"empty mix":       `{"mix":{}}`,
		"zero customers":  `{"customers":0}`,
		"wrong type":      `{"rps":"fast"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, h := newServer(t)
			rec := do(t, h, http.MethodPut, "/v1/traffic", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
			var got httpx.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Error.Code != "invalid_traffic" {
				t.Errorf("error code = %q, want invalid_traffic", got.Error.Code)
			}
			if got.Error.Message == "" {
				t.Error("error message is empty")
			}
		})
	}
}

// The target is fixed by configuration; there is no API to move it.
func TestTargetCannotBeChanged(t *testing.T) {
	e, h := newServer(t)
	do(t, h, http.MethodPut, "/v1/traffic", `{"target":"http://evil"}`)
	if e.Status().Target != "http://dingle-arm-gateway:8080" {
		t.Errorf("target changed to %q", e.Status().Target)
	}
}

func TestUnsupportedMethod(t *testing.T) {
	_, h := newServer(t)
	if rec := do(t, h, http.MethodDelete, "/v1/traffic", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
