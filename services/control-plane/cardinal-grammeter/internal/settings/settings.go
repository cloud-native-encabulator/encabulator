// Package settings keeps the last traffic configuration applied through the API,
// so that a grammeter whose pod is replaced picks up where the last one stopped.
//
// The configuration lives as JSON under one key of a Kubernetes ConfigMap that the
// deployment creates empty. Restore reads it once at startup; a Saver writes every
// accepted change from the background. Nothing here may stop the grammeter: a
// missing, empty, unreadable or invalid saved configuration means the declared
// defaults, exactly as before settings were kept at all.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/engine"
)

// Key is the ConfigMap data key that holds the saved configuration.
const Key = "settings.json"

// ErrNotSaved means there is no saved configuration yet.
var ErrNotSaved = errors.New("no saved traffic settings")

// Store reads and writes the saved configuration as raw bytes.
type Store interface {
	// Load returns the saved bytes, or ErrNotSaved when nothing has been saved.
	Load(ctx context.Context) ([]byte, error)
	// Save replaces the saved bytes.
	Save(ctx context.Context, data []byte) error
}

// Encode renders a configuration the way it is saved.
func Encode(c engine.Config) ([]byte, error) {
	return json.Marshal(c)
}

// Decode reads a saved configuration over defaults: a field the saved JSON does not
// name keeps its default, and fields it does not know are ignored, so a rollback to
// an older grammeter still reads what a newer one saved. The result must pass the
// same validation as PUT /v1/traffic, or Decode returns an error and no
// configuration.
func Decode(data []byte, defaults engine.Config) (engine.Config, error) {
	var saved engine.Patch
	if err := json.Unmarshal(data, &saved); err != nil {
		return engine.Config{}, fmt.Errorf("saved traffic settings do not parse: %w", err)
	}
	cfg := defaults.With(saved)
	if err := engine.Validate(cfg); err != nil {
		return engine.Config{}, fmt.Errorf("saved traffic settings are out of range: %w", err)
	}
	return cfg, nil
}

// Restore attempts, how long each may take, and the pause between them. The
// API server is normally a few milliseconds away; this only rides out a blip
// right after the pod starts.
const (
	restoreAttempts = 3
	restoreTimeout  = 2 * time.Second
	restorePause    = 500 * time.Millisecond
)

// Restore returns the saved configuration when there is a usable one, and
// defaults otherwise. It never fails: every problem is logged and answered with
// the defaults.
func Restore(ctx context.Context, store Store, defaults engine.Config, log *slog.Logger) engine.Config {
	if log == nil {
		log = slog.Default()
	}
	data, err := load(ctx, store)
	switch {
	case errors.Is(err, ErrNotSaved):
		log.Info("no saved traffic settings; starting from the defaults")
		return defaults
	case err != nil:
		log.Warn("could not read the saved traffic settings; starting from the defaults", "error", err.Error())
		return defaults
	}
	cfg, err := Decode(data, defaults)
	if err != nil {
		log.Warn("saved traffic settings are unusable; starting from the defaults", "error", err.Error())
		return defaults
	}
	log.Info("restored the saved traffic settings",
		"running", cfg.Running, "rps", cfg.RPS, "pattern", string(cfg.Pattern),
		"periodSeconds", cfg.PeriodSeconds, "customers", cfg.Customers)
	return cfg
}

// load reads the store, retrying a failed read a couple of times. A definite
// answer (the bytes, or nothing saved) ends it at once.
func load(ctx context.Context, store Store) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		loadCtx, cancel := context.WithTimeout(ctx, restoreTimeout)
		data, err := store.Load(loadCtx)
		cancel()
		if err == nil || errors.Is(err, ErrNotSaved) || attempt == restoreAttempts {
			return data, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(restorePause):
		}
	}
}
