package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/flags"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/testsupport"

	"github.com/cloud-native-encabulator/encabulator/services/support-plane/differential-girdlespring/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/support-plane/differential-girdlespring/internal/store"
)

// newStore brings up a real postgres:18 and returns a migrated store.
func newStore(t *testing.T) *store.Store {
	t.Helper()
	port := testsupport.StartContainer(t, testsupport.ContainerSpec{
		Image: "postgres:18",
		Port:  5432,
		Env: map[string]string{
			"POSTGRES_USER":     "girdlespring",
			"POSTGRES_PASSWORD": "prefamulated",
			"POSTGRES_DB":       "girdlespring",
		},
		// Postgres restarts once during first-time initialisation, so wait for the
		// second "ready to accept connections".
		Wait: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(3 * time.Minute),
	})

	ctx := testsupport.Context(t)
	dsn := fmt.Sprintf("postgres://girdlespring:prefamulated@127.0.0.1:%d/girdlespring?sslmode=disable", port)
	st, err := store.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(st.Close)

	if err := store.Migrate(ctx, st.Pool()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

func TestMigrateIsIdempotentAndSeedNeverOverwrites(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	// A second Migrate from another replica must be a no-op.
	if err := store.Migrate(ctx, st.Pool()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	if err := st.Seed(ctx, api.Seeds()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	list, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != len(api.Seeds()) {
		t.Fatalf("seeded %d flags, want %d", len(list), len(api.Seeds()))
	}

	// Simulate an operator edit, then re-seed: the edit must survive.
	if _, err := st.Put(ctx, flags.TurboEncabulation, store.Update{
		Value:     json.RawMessage("true"),
		UpdatedBy: "opal",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Seed(ctx, api.Seeds()); err != nil {
		t.Fatal(err)
	}
	f, err := st.Get(ctx, flags.TurboEncabulation)
	if err != nil {
		t.Fatal(err)
	}
	if string(f.Value) != "true" {
		t.Errorf("seeding overwrote an operator edit: value = %s", f.Value)
	}
	if f.UpdatedBy != "opal" {
		t.Errorf("updatedBy = %q", f.UpdatedBy)
	}
}

func TestListIsOrderedAndTyped(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)
	if err := st.Seed(ctx, api.Seeds()); err != nil {
		t.Fatal(err)
	}

	list, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].Key >= list[i].Key {
			t.Fatalf("flags are not ordered by key: %s then %s", list[i-1].Key, list[i].Key)
		}
	}
	for _, f := range list {
		switch f.Type {
		case flags.TypeBoolean:
			if _, ok := f.Bool(); !ok {
				t.Errorf("%s is a boolean but holds %s", f.Key, f.Value)
			}
		case flags.TypeNumber:
			if _, ok := f.Number(); !ok {
				t.Errorf("%s is a number but holds %s", f.Key, f.Value)
			}
		default:
			t.Errorf("%s has unexpected type %q", f.Key, f.Type)
		}
		if f.UpdatedAt.IsZero() {
			t.Errorf("%s has no updatedAt", f.Key)
		}
		if f.RolloutPercent != nil {
			t.Errorf("%s is seeded with a rolloutPercent (%d)", f.Key, *f.RolloutPercent)
		}
	}
}

func TestPutRoundTripsRolloutAndDescription(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)
	if err := st.Seed(ctx, api.Seeds()); err != nil {
		t.Fatal(err)
	}

	pct := 25
	desc := "quarter of the factory"
	got, err := st.Put(ctx, flags.StrictSideFumbling, store.Update{
		Value:          json.RawMessage("true"),
		RolloutPercent: &pct,
		Description:    &desc,
		UpdatedBy:      "opal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RolloutPercent == nil || *got.RolloutPercent != 25 {
		t.Fatalf("rolloutPercent = %v", got.RolloutPercent)
	}
	if got.Description != desc {
		t.Errorf("description = %q", got.Description)
	}

	// Reload to confirm it persisted, and confirm clearing the rollout works.
	reloaded, err := st.Get(ctx, flags.StrictSideFumbling)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.RolloutPercent == nil || *reloaded.RolloutPercent != 25 {
		t.Fatalf("reloaded rolloutPercent = %v", reloaded.RolloutPercent)
	}

	cleared, err := st.Put(ctx, flags.StrictSideFumbling, store.Update{
		Value:     json.RawMessage("false"),
		UpdatedBy: "opal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.RolloutPercent != nil {
		t.Errorf("rolloutPercent not cleared: %v", *cleared.RolloutPercent)
	}
	if cleared.Description != desc {
		t.Errorf("omitting description should preserve it, got %q", cleared.Description)
	}
}

func TestUnknownKeys(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	if _, err := st.Get(ctx, "no-such-flag"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get unknown = %v, want ErrNotFound", err)
	}
	if _, err := st.Put(ctx, "no-such-flag", store.Update{
		Value: json.RawMessage("true"), UpdatedBy: "opal",
	}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Put unknown = %v, want ErrNotFound", err)
	}
}

func TestPingIsTheReadinessCheck(t *testing.T) {
	st := newStore(t)
	if err := st.Ping(testsupport.Context(t)); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	// A closed pool must report unready rather than pretend.
	bad, err := store.New(context.Background(), "postgres://nobody:nothing@127.0.0.1:1/none?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bad.Ping(ctx); err == nil {
		t.Error("Ping against a dead Postgres returned nil")
	}
}
