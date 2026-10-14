package store_test

import (
	"fmt"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/testsupport"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/malaise"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/store"
)

// valkeyImage is the backing store image from design section 2.
const valkeyImage = "valkey/valkey:9"

// TestStoreAgainstRealValkey runs the store's whole surface against a real
// Valkey, because miniredis is a good imitation and not a guarantee. It skips
// cleanly when no container runtime is available.
func TestStoreAgainstRealValkey(t *testing.T) {
	testsupport.RequireContainerRuntime(t)
	ctx := testsupport.Context(t)

	// The host port is reserved and bound explicitly: Finch's containerd/CNI
	// stack rejects the "pick any free port" binding Testcontainers defaults to.
	hostPort := testsupport.FreePort(t)
	valkey, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        valkeyImage,
			ExposedPorts: []string{"6379/tcp"},
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.PortBindings = network.PortMap{
					network.MustParsePort("6379/tcp"): []network.PortBinding{{
						HostIP:   netip.MustParseAddr("127.0.0.1"),
						HostPort: strconv.Itoa(hostPort),
					}},
				}
			},
			WaitingFor: wait.ForLog("Ready to accept connections").WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start valkey: %v", err)
	}
	t.Cleanup(func() {
		if terr := testcontainers.TerminateContainer(valkey); terr != nil {
			t.Logf("terminate valkey: %v", terr)
		}
	})

	url := fmt.Sprintf("redis://127.0.0.1:%d/0", hostPort)
	s, err := store.New(store.Config{URL: url})
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() {
		if cerr := s.Close(); cerr != nil {
			t.Logf("close store: %v", cerr)
		}
	})

	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	seconds := 120
	active, err := malaise.Build(malaise.Request{
		Target: "dingle-arm-gateway", Kind: malaise.KindLatency,
		Params: malaise.Params{Ms: 400, JitterMs: 100}, DurationSeconds: &seconds,
	}, time.Now())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	short := 1
	soon, err := malaise.Build(malaise.Request{
		Target: "spurving-bearing-quoter", Kind: malaise.KindError,
		Params: malaise.Params{Status: 503}, DurationSeconds: &short,
	}, time.Now())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, m := range []malaise.Malaise{active, soon} {
		if err := s.Put(ctx, m); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	got, err := s.Get(ctx, active.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Target != active.Target || got.Params.Ms != 400 {
		t.Errorf("record did not survive the round trip: %+v", got)
	}

	list, err := s.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("active = %d records, want 2", len(list))
	}
	// Soonest expiry first.
	if list[0].ID != soon.ID {
		t.Errorf("active order = %s first, want %s", list[0].ID, soon.ID)
	}

	due, err := s.DueBefore(ctx, time.Now().Add(5*time.Second))
	if err != nil {
		t.Fatalf("DueBefore: %v", err)
	}
	if len(due) != 1 || due[0] != soon.ID {
		t.Errorf("due = %v, want just %s", due, soon.ID)
	}

	soon.State = malaise.StateExpired
	ended := time.Now().UTC()
	soon.EndedAt = &ended
	if err := s.Put(ctx, soon); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if list, err = s.Active(ctx); err != nil || len(list) != 1 {
		t.Fatalf("active = %d records (err %v), want 1", len(list), err)
	}

	recent, err := s.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("recent = %d records, want both", len(recent))
	}

	if err := s.Deactivate(ctx, active.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if list, err = s.Active(ctx); err != nil || len(list) != 0 {
		t.Fatalf("active = %d records (err %v), want none", len(list), err)
	}

	// The keys this service owns, and nothing else.
	client := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("127.0.0.1:%d", hostPort)})
	t.Cleanup(func() { _ = client.Close() })
	keys, err := client.Keys(ctx, "*").Result()
	if err != nil {
		t.Fatalf("KEYS: %v", err)
	}
	for _, key := range keys {
		if len(key) < len("{malaise}:") || key[:len("{malaise}:")] != "{malaise}:" {
			t.Errorf("key %q is outside the {malaise}:* namespace", key)
		}
	}
}
