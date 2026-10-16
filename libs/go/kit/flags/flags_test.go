package flags_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/flags"
)

// girdlespring is a stand-in for differential-girdlespring.
type girdlespring struct {
	mu     sync.Mutex
	body   flags.ListResponse
	etag   string
	hits   atomic.Int32
	notMod atomic.Int32
	srv    *httptest.Server
}

func newGirdlespring(t *testing.T) *girdlespring {
	t.Helper()
	g := &girdlespring{etag: `"v1"`}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.hits.Add(1)
		g.mu.Lock()
		body, etag := g.body, g.etag
		g.mu.Unlock()
		if r.Header.Get("If-None-Match") == etag {
			g.notMod.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *girdlespring) set(etag string, fs ...flags.Flag) {
	g.mu.Lock()
	g.etag = etag
	g.body = flags.ListResponse{Flags: fs}
	g.mu.Unlock()
}

func flag(key, typ string, value any, rollout *int) flags.Flag {
	raw, _ := json.Marshal(value)
	return flags.Flag{Key: key, Type: typ, Value: raw, RolloutPercent: rollout}
}

func TestDefaultsWhenURLUnset(t *testing.T) {
	c := flags.New(flags.Config{})
	c.Start(context.Background())

	if c.Bool(flags.TurboEncabulation) {
		t.Error("turbo-encabulation default must be false")
	}
	if !c.Bool(flags.QuoteCache) {
		t.Error("quote-cache default must be true")
	}
	if !c.Bool(flags.RetroEncabulatorSales) {
		t.Error("retro-encabulator-sales default must be true")
	}
	if c.Bool(flags.StrictSideFumbling) {
		t.Error("strict-side-fumbling default must be false")
	}
	if got := c.Number(flags.AmuliteFoundryRate); got != 20 {
		t.Errorf("amulite-foundry-rate = %v, want 20", got)
	}
	if got := c.Number(flags.GatewayRateLimitRPS); got != 50 {
		t.Errorf("gateway-rate-limit-rps = %v, want 50", got)
	}
	if got := c.Number(flags.DingleArmDamping); got != 50 {
		t.Errorf("dingle-arm-damping = %v, want 50", got)
	}
	if c.Fetched() {
		t.Error("Fetched must be false with no girdlespring")
	}
}

func TestDefaultsWhenServiceUnreachableAtStartup(t *testing.T) {
	c := flags.New(flags.Config{
		BaseURL:    "http://127.0.0.1:1", // nothing listens here
		HTTPClient: &http.Client{Timeout: 100 * time.Millisecond},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)

	if !c.Bool(flags.QuoteCache) {
		t.Error("must fall back to the built-in default")
	}
	if c.Fetched() {
		t.Error("Fetched must be false after a failed fetch")
	}
}

func TestFetchAndOverride(t *testing.T) {
	g := newGirdlespring(t)
	g.set(`"v1"`,
		flag(flags.TurboEncabulation, flags.TypeBoolean, true, nil),
		flag(flags.GatewayRateLimitRPS, flags.TypeNumber, 125.5, nil),
	)

	c := flags.New(flags.Config{BaseURL: g.srv.URL, Interval: 10 * time.Millisecond})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !c.Bool(flags.TurboEncabulation) {
		t.Error("turbo-encabulation should now be true")
	}
	if got := c.Number(flags.GatewayRateLimitRPS); got != 125.5 {
		t.Errorf("gateway-rate-limit-rps = %v", got)
	}
	// Keys the girdlespring did not send still come from the defaults.
	if !c.Bool(flags.QuoteCache) {
		t.Error("unsent key lost its default")
	}
	if !c.Fetched() {
		t.Error("Fetched should be true")
	}
}

func TestETagPollingAndLastGoodSet(t *testing.T) {
	g := newGirdlespring(t)
	g.set(`"v1"`, flag(flags.TurboEncabulation, flags.TypeBoolean, true, nil))

	c := flags.New(flags.Config{BaseURL: g.srv.URL, Interval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for g.notMod.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if g.notMod.Load() == 0 {
		t.Fatal("client never sent If-None-Match with a matching ETag")
	}
	if !c.Bool(flags.TurboEncabulation) {
		t.Error("a 304 must leave the last good set in place")
	}

	// A new ETag must be picked up.
	g.set(`"v2"`, flag(flags.TurboEncabulation, flags.TypeBoolean, false, nil))
	deadline = time.Now().Add(2 * time.Second)
	for c.Bool(flags.TurboEncabulation) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if c.Bool(flags.TurboEncabulation) {
		t.Error("client did not pick up the new flag set")
	}
}

func TestRolloutPercentBoundaries(t *testing.T) {
	if flags.InRollout("k", "s", 0) {
		t.Error("rolloutPercent 0 must exclude everyone")
	}
	if !flags.InRollout("k", "s", 100) {
		t.Error("rolloutPercent 100 must include everyone")
	}
	if flags.InRollout("k", "s", 50) != flags.InRollout("k", "s", 50) {
		t.Error("rollout evaluation must be deterministic")
	}
}

func TestRolloutPercentMatchesFNV1a32(t *testing.T) {
	// fnv1a32("turbo-encabulation:wendy") % 100 is a fixed number; assert the
	// predicate flips exactly at that bucket.
	const key, subject = flags.TurboEncabulation, "wendy"
	bucket := -1
	for p := 1; p <= 100; p++ {
		if flags.InRollout(key, subject, p) {
			bucket = p - 1
			break
		}
	}
	if bucket < 0 {
		t.Fatal("subject never entered the rollout")
	}
	if flags.InRollout(key, subject, bucket) {
		t.Errorf("subject in bucket %d must be excluded at rolloutPercent %d", bucket, bucket)
	}
	if !flags.InRollout(key, subject, bucket+1) {
		t.Errorf("subject in bucket %d must be included at rolloutPercent %d", bucket, bucket+1)
	}
}

func TestRolloutSpreadsSubjects(t *testing.T) {
	in := 0
	const n = 2000
	for i := range n {
		if flags.InRollout(flags.StrictSideFumbling, fmt.Sprintf("customer-%d", i), 25) {
			in++
		}
	}
	// 25% of 2000 is 500; allow generous slack for a 32-bit hash.
	if in < 400 || in > 600 {
		t.Errorf("25%% rollout selected %d of %d subjects", in, n)
	}
}

func TestBoolForHonorsRollout(t *testing.T) {
	g := newGirdlespring(t)
	rollout := 100
	g.set(`"v1"`, flag(flags.TurboEncabulation, flags.TypeBoolean, true, &rollout))
	c := flags.New(flags.Config{BaseURL: g.srv.URL})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !c.BoolFor(flags.TurboEncabulation, "wendy") {
		t.Error("rolloutPercent 100 must apply to everyone")
	}

	zero := 0
	g.set(`"v2"`, flag(flags.TurboEncabulation, flags.TypeBoolean, true, &zero))
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.BoolFor(flags.TurboEncabulation, "wendy") {
		t.Error("rolloutPercent 0 must apply to nobody")
	}
}

func TestSnapshot(t *testing.T) {
	g := newGirdlespring(t)
	g.set(`"v1"`, flag(flags.QuoteCache, flags.TypeBoolean, false, nil))
	c := flags.New(flags.Config{BaseURL: g.srv.URL})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := c.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot = %v", snap)
	}
	// Mutating the snapshot must not affect the client.
	delete(snap, flags.QuoteCache)
	if c.Bool(flags.QuoteCache) {
		t.Error("client state changed through the snapshot")
	}
}
