package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/malaise-injection-engine/internal/malaise"
)

var epoch = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

// newStore returns a store over miniredis, plus the fake clock driving it.
func newStore(t *testing.T) (*Store, *clock, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	c := &clock{now: epoch}
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	s, err := New(Config{Client: client, Now: c.Now})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, c, server
}

func build(t *testing.T, req malaise.Request, at time.Time) malaise.Malaise {
	t.Helper()
	m, err := malaise.Build(req, at)
	if err != nil {
		t.Fatalf("malaise.Build: %v", err)
	}
	return m
}

func latency(t *testing.T, target string, at time.Time, seconds int) malaise.Malaise {
	t.Helper()
	return build(t, malaise.Request{
		Target: target, Kind: malaise.KindLatency,
		Params:          malaise.Params{Ms: 400},
		DurationSeconds: &seconds,
	}, at)
}

func TestNewRequiresAURL(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected an error without a URL")
	}
	if _, err := New(Config{URL: "not a url at all"}); err == nil {
		t.Fatal("expected an error for an unparseable URL")
	}
	s, err := New(Config{URL: "redis://localhost:6379/0"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestPutAndGet(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()
	m := latency(t, "dingle-arm-gateway", epoch, 120)

	if err := s.Put(ctx, m); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != m.ID || got.Target != m.Target || got.Kind != m.Kind {
		t.Errorf("got %+v, want %+v", got, m)
	}
	if !got.ExpiresAt.Equal(m.ExpiresAt) || !got.CreatedAt.Equal(m.CreatedAt) {
		t.Errorf("timestamps did not survive: %v/%v", got.CreatedAt, got.ExpiresAt)
	}
	if got.Params.Percent == nil || *got.Params.Percent != 100 {
		t.Errorf("params did not survive: %+v", got.Params)
	}
}

func TestGetMissing(t *testing.T) {
	s, _, _ := newStore(t)
	_, err := s.Get(context.Background(), "mal_nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestActiveIndexTracksState(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()

	active := latency(t, "dingle-arm-gateway", epoch, 120)
	instant := build(t, malaise.Request{Target: "logarithmic-casing-line",
		Kind: malaise.KindPodKill}, epoch)

	if err := s.Put(ctx, active); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Put(ctx, instant); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if len(got) != 1 || got[0].ID != active.ID {
		t.Fatalf("active = %v, want only the latency malaise", ids(got))
	}

	// Ending it takes it out of the active index but leaves the record.
	active.State = malaise.StateCleared
	if err := s.Put(ctx, active); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got, err = s.Active(ctx); err != nil || len(got) != 0 {
		t.Fatalf("active = %v (err %v), want none", ids(got), err)
	}
	if _, err := s.Get(ctx, active.ID); err != nil {
		t.Fatalf("the record should still be readable: %v", err)
	}
}

func TestActiveIsOrderedByExpiry(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()

	late := latency(t, "a", epoch, 900)
	soon := latency(t, "b", epoch, 10)
	middle := latency(t, "c", epoch, 120)
	for _, m := range []malaise.Malaise{late, soon, middle} {
		if err := s.Put(ctx, m); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	got, err := s.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	want := []string{soon.ID, middle.ID, late.ID}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("active order = %v, want %v", ids(got), want)
		}
	}
}

func TestRecentIsNewestFirstAndCapped(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()

	var last string
	for i := range 5 {
		m := latency(t, "dingle-arm-gateway", epoch.Add(time.Duration(i)*time.Second), 120)
		if err := s.Put(ctx, m); err != nil {
			t.Fatalf("Put: %v", err)
		}
		last = m.ID
	}

	got, err := s.Recent(ctx, 3)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("recent = %d records, want 3", len(got))
	}
	if got[0].ID != last {
		t.Errorf("first record = %s, want the newest %s", got[0].ID, last)
	}

	all, err := s.Recent(ctx, 0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("recent(0) = %d records, want all 5", len(all))
	}
}

func TestHistoryIsTrimmed(t *testing.T) {
	s, _, server := newStore(t)
	ctx := context.Background()

	for i := range HistoryLimit + 25 {
		m := latency(t, "dingle-arm-gateway", epoch.Add(time.Duration(i)*time.Millisecond), 120)
		if err := s.Put(ctx, m); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	members, err := server.ZMembers(historyKey)
	if err != nil {
		t.Fatalf("ZMembers: %v", err)
	}
	if len(members) != HistoryLimit {
		t.Errorf("history holds %d entries, want the cap of %d", len(members), HistoryLimit)
	}
}

func TestDueBefore(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()

	soon := latency(t, "a", epoch, 10)
	late := latency(t, "b", epoch, 600)
	for _, m := range []malaise.Malaise{soon, late} {
		if err := s.Put(ctx, m); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	due, err := s.DueBefore(ctx, epoch.Add(30*time.Second))
	if err != nil {
		t.Fatalf("DueBefore: %v", err)
	}
	if len(due) != 1 || due[0] != soon.ID {
		t.Errorf("due = %v, want just %s", due, soon.ID)
	}

	// Exactly at the expiry counts as due.
	due, err = s.DueBefore(ctx, soon.ExpiresAt)
	if err != nil {
		t.Fatalf("DueBefore: %v", err)
	}
	if len(due) != 1 {
		t.Errorf("due at the expiry instant = %v, want one", due)
	}

	if due, err = s.DueBefore(ctx, epoch); err != nil || len(due) != 0 {
		t.Errorf("due = %v (err %v), want none yet", due, err)
	}
}

func TestDeactivate(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()
	m := latency(t, "a", epoch, 120)
	if err := s.Put(ctx, m); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := s.Deactivate(ctx, m.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	active, err := s.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("active = %v, want none", ids(active))
	}
	// Deactivating something absent is not an error.
	if err := s.Deactivate(ctx, "mal_nope"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
}

// A record can expire out of Valkey while its id is still indexed. Loading must
// skip it and prune the index rather than fail.
func TestLoadPrunesOrphanedIndexEntries(t *testing.T) {
	s, _, server := newStore(t)
	ctx := context.Background()

	alive := latency(t, "a", epoch, 120)
	orphan := latency(t, "b", epoch, 120)
	for _, m := range []malaise.Malaise{alive, orphan} {
		if err := s.Put(ctx, m); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	server.Del(recordPrefix + orphan.ID)

	got, err := s.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if len(got) != 1 || got[0].ID != alive.ID {
		t.Fatalf("active = %v, want only %s", ids(got), alive.ID)
	}
	members, err := server.ZMembers(activeKey)
	if err != nil {
		t.Fatalf("ZMembers: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("active index holds %v, want the orphan pruned", members)
	}
}

func TestRecordTTLOutlivesTheMalaise(t *testing.T) {
	s, c, server := newStore(t)
	ctx := context.Background()

	m := latency(t, "a", epoch, 900)
	if err := s.Put(ctx, m); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ttl := server.TTL(recordPrefix + m.ID)
	if ttl <= 900*time.Second {
		t.Errorf("record TTL = %v, want more than the 900s duration", ttl)
	}

	// A record written after its expiry still gets the full history window.
	c.now = epoch.Add(time.Hour)
	ended := m
	ended.State = malaise.StateExpired
	if err := s.Put(ctx, ended); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if ttl = server.TTL(recordPrefix + m.ID); ttl < HistoryTTL {
		t.Errorf("record TTL = %v, want at least %v", ttl, HistoryTTL)
	}
}

func TestPingAndClose(t *testing.T) {
	s, _, server := newStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	server.Close()
	if err := s.Ping(context.Background()); err == nil {
		t.Fatal("Ping should fail once Valkey is gone")
	}
}

func TestEmptyStore(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()
	if got, err := s.Active(ctx); err != nil || len(got) != 0 {
		t.Errorf("Active = %v, %v", got, err)
	}
	if got, err := s.Recent(ctx, 10); err != nil || len(got) != 0 {
		t.Errorf("Recent = %v, %v", got, err)
	}
	if got, err := s.DueBefore(ctx, epoch); err != nil || len(got) != 0 {
		t.Errorf("DueBefore = %v, %v", got, err)
	}
}

func ids(list []malaise.Malaise) []string {
	out := make([]string, len(list))
	for i, m := range list {
		out[i] = m.ID
	}
	return out
}
