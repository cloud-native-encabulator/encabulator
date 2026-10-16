package settings_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/engine"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/settings"
)

type nopRunner struct{}

func (nopRunner) Run(context.Context, string) {}

// stalledStore never finishes a write until its context ends.
type stalledStore struct{}

func (s *stalledStore) Load(context.Context) ([]byte, error) { return nil, settings.ErrNotSaved }

func (s *stalledStore) Save(ctx context.Context, _ []byte) error {
	<-ctx.Done()
	return ctx.Err()
}

// TestPutTrafficDoesNotWaitForTheSave wires the API, the engine and a Saver the
// way main does, against a store that never answers, and checks the PUT still
// returns at once with the new settings in effect.
func TestPutTrafficDoesNotWaitForTheSave(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	saver := settings.NewSaver(&stalledStore{}, log)
	saver.Timeout = time.Hour
	go saver.Run()

	eng, err := engine.New(engine.Options{
		Runner:   nopRunner{},
		Logger:   log,
		Initial:  engine.Config{RPS: 5, Customers: 50},
		OnChange: saver.Submit,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	api.New(eng, log).Register(mux)

	for i := range 20 {
		req := httptest.NewRequest(http.MethodPut, "/v1/traffic",
			strings.NewReader(`{"running":true,"rps":`+string(rune('1'+i%9))+`,"pattern":"sine"}`))
		rec := httptest.NewRecorder()
		start := time.Now()
		mux.ServeHTTP(rec, req)
		if took := time.Since(start); took > 100*time.Millisecond {
			t.Fatalf("PUT %d took %v with the save stalled", i, took)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %d = %d: %s", i, rec.Code, rec.Body.String())
		}
	}
	if cfg := eng.Config(); !cfg.Running || cfg.Pattern != "sine" {
		t.Errorf("settings in effect = %+v", cfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = saver.Close(ctx) // the stalled write ends with ctx; nothing here may hang
}
