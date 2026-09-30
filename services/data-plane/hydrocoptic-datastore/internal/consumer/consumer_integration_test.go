package consumer_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/events"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/testsupport"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/consumer"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/ledger"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	port := testsupport.StartContainer(t, testsupport.ContainerSpec{
		Image: "postgres:18",
		Port:  5432,
		Env: map[string]string{
			"POSTGRES_USER":     "hydrocoptic",
			"POSTGRES_PASSWORD": "marzlevane",
			"POSTGRES_DB":       "hydrocoptic",
		},
		Wait: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(3 * time.Minute),
	})
	ctx := testsupport.Context(t)
	st, err := store.New(ctx,
		fmt.Sprintf("postgres://hydrocoptic:marzlevane@127.0.0.1:%d/hydrocoptic?sslmode=disable", port))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(st.Close)
	if err := store.Migrate(ctx, st.Pool()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

func newNATS(t *testing.T) string {
	t.Helper()
	port := testsupport.StartContainer(t, testsupport.ContainerSpec{
		Image: "nats:2.12",
		Cmd:   []string{"--jetstream", "--store_dir=/tmp/jetstream"},
		Port:  4222,
		Wait:  wait.ForLog("Server is ready").WithStartupTimeout(3 * time.Minute),
	})
	return fmt.Sprintf("nats://127.0.0.1:%d", port)
}

// TestLedgerFollowsTheStream drives the real consumer over a real broker against
// a real database: the order's whole life arrives as events and the ledger ends
// up delivered, with a complete timeline.
func TestLedgerFollowsTheStream(t *testing.T) {
	st := newStore(t)
	url := newNATS(t)
	ctx, cancel := context.WithCancel(testsupport.Context(t))
	defer cancel()

	nc, js, err := events.Connect(url, "hydrocoptic-datastore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	stream, err := events.EnsureStream(ctx, js)
	if err != nil {
		t.Fatal(err)
	}

	// The order is written over HTTP first, as the processor does.
	if _, _, err := st.Create(ctx, store.NewOrder{
		ID: "ord_1", Customer: "wendy", Model: ledger.ModelTurbo, Quantity: 2,
		Options: ledger.Options{DingleArm: true},
		Quote:   &ledger.Quote{ID: "qte_1", Total: 4200, Currency: "USD"},
	}); err != nil {
		t.Fatal(err)
	}

	cons := consumer.New(st, slog.New(slog.DiscardHandler), nil, prometheus.NewRegistry())
	consumeCtx, err := cons.Start(ctx, stream)
	if err != nil {
		t.Fatal(err)
	}
	defer consumeCtx.Stop()

	pub := events.NewPublisher(js, "test-harness")
	steps := []struct {
		subject string
		data    any
	}{
		{events.SubjectOrderAccepted, map[string]any{"orderId": "ord_1", "customer": "wendy",
			"model": "turbo", "quantity": 2, "options": map[string]bool{"dingleArm": true},
			"total": 4200.0, "currency": "USD"}},
		{events.SubjectFabricationRequested, map[string]any{"orderId": "ord_1", "attempt": 1,
			"model": "turbo", "quantity": 2, "reservationId": "rsv_1"}},
		{events.SubjectFabricationStage, map[string]any{"orderId": "ord_1", "attempt": 1,
			"serial": "ENC-turbo-a", "stage": "prelaminate-base-plate", "stageIndex": 1, "durationMs": 300}},
		{events.SubjectFabricationCompleted, map[string]any{"orderId": "ord_1", "attempt": 1,
			"model": "turbo", "serials": []string{"ENC-turbo-a", "ENC-turbo-b"}, "durationMs": 3100}},
		{events.SubjectInspectionPassed, map[string]any{"orderId": "ord_1", "attempt": 1,
			"serials": []string{"ENC-turbo-a", "ENC-turbo-b"}}},
		{events.SubjectShipmentDispatched, map[string]any{"orderId": "ord_1",
			"carrier": "Tremie Pipe & Sons", "trackingNumber": "NT9876543210", "etaSeconds": 25}},
		{events.SubjectShipmentDelivered, map[string]any{"orderId": "ord_1",
			"carrier": "Tremie Pipe & Sons", "trackingNumber": "NT9876543210"}},
	}
	for _, step := range steps {
		if _, err := pub.Publish(ctx, step.subject, step.data); err != nil {
			t.Fatalf("publish %s: %v", step.subject, err)
		}
	}

	deadline := time.Now().Add(30 * time.Second)
	var final ledger.Order
	for time.Now().Before(deadline) {
		final, err = st.GetWithTimeline(ctx, "ord_1")
		if err != nil {
			t.Fatal(err)
		}
		if final.Status == ledger.StatusDelivered {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if final.Status != ledger.StatusDelivered {
		t.Fatalf("status = %q after 30s; timeline = %+v", final.Status, final.Timeline)
	}
	if final.Carrier != "Tremie Pipe & Sons" || final.TrackingNumber != "NT9876543210" {
		t.Errorf("shipment fields = %q / %q", final.Carrier, final.TrackingNumber)
	}
	if len(final.Serials) != 2 {
		t.Errorf("serials = %v", final.Serials)
	}
	// One entry from the HTTP create plus one per event.
	if len(final.Timeline) != len(steps)+1 {
		t.Errorf("timeline has %d entries, want %d: %+v", len(final.Timeline), len(steps)+1, final.Timeline)
	}
}

// TestRedeliveryDoesNotDoubleApply republishes the same envelope, which
// JetStream deduplicates by Nats-Msg-Id, and also drives the handler twice
// directly, which the ledger deduplicates by event id.
func TestRedeliveryDoesNotDoubleApply(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	if _, _, err := st.Create(ctx, store.NewOrder{
		ID: "ord_1", Customer: "wendy", Model: ledger.ModelTurbo, Quantity: 1,
	}); err != nil {
		t.Fatal(err)
	}

	cons := consumer.New(st, slog.New(slog.DiscardHandler), nil, prometheus.NewRegistry())
	env, err := events.NewEnvelope("test-harness", events.SubjectFabricationStage, map[string]any{
		"orderId": "ord_1", "attempt": 1, "serial": "ENC-a",
		"stage": "align-spurving-bearings", "stageIndex": 3, "durationMs": 500,
	})
	if err != nil {
		t.Fatal(err)
	}

	for range 4 {
		if err := cons.Handle(ctx, env); err != nil {
			t.Fatalf("handle: %v", err)
		}
	}

	full, err := st.GetWithTimeline(ctx, "ord_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Timeline) != 2 {
		t.Errorf("timeline has %d entries, want 2 (create plus one event): %+v",
			len(full.Timeline), full.Timeline)
	}
}

// TestOrphanEventIsNaked proves an event for an order that does not exist yet is
// reported as an error, so the kit naks it and JetStream brings it back.
func TestOrphanEventIsNaked(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	cons := consumer.New(st, slog.New(slog.DiscardHandler), nil, prometheus.NewRegistry())
	env, err := events.NewEnvelope("test-harness", events.SubjectFabricationStage, map[string]any{
		"orderId": "ord_missing", "attempt": 1, "serial": "ENC-a",
		"stage": "prelaminate-base-plate", "stageIndex": 1, "durationMs": 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cons.Handle(ctx, env); err == nil {
		t.Fatal("an orphan event must be reported as an error so it is redelivered")
	}
}

// TestUnknownSubjectIsAcked proves an event the ledger has no opinion about is
// acked rather than looping to MaxDeliver.
func TestUnknownSubjectIsAcked(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	cons := consumer.New(st, slog.New(slog.DiscardHandler), nil, prometheus.NewRegistry())
	env, err := events.NewEnvelope("test-harness", "encabulator.something.unexpected",
		map[string]any{"orderId": "ord_1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cons.Handle(ctx, env); err != nil {
		t.Fatalf("unknown subject should be acked, got %v", err)
	}
}
