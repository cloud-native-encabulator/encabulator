package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/testsupport"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/ledger"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/store"
)

// newStore brings up a real postgres:18 and returns a migrated ledger.
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
		// Postgres restarts once while initialising, so wait for the second
		// "ready to accept connections".
		Wait: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(3 * time.Minute),
	})

	ctx := testsupport.Context(t)
	dsn := fmt.Sprintf("postgres://hydrocoptic:marzlevane@127.0.0.1:%d/hydrocoptic?sslmode=disable", port)
	st, err := store.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(st.Close)

	if err := store.Migrate(ctx, st.Pool()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// A second run must be a no-op, as it is for a second replica.
	if err := store.Migrate(ctx, st.Pool()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	return st
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newOrder(id string) store.NewOrder {
	return store.NewOrder{
		ID: id, Customer: "wendy", Model: ledger.ModelTurbo, Quantity: 2,
		Options:       ledger.Options{DingleArm: true},
		Quote:         &ledger.Quote{ID: "qte_" + id, Total: 4200, Currency: "USD"},
		ReservationID: "rsv_" + id,
	}
}

func TestCreateIsIdempotent(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	first, created, err := st.Create(ctx, newOrder("ord_1"))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first Create reported no insert")
	}
	if first.Status != ledger.StatusAccepted || first.Attempt != 1 {
		t.Errorf("order = %+v", first)
	}
	if !first.Options.DingleArm || first.Quote == nil || first.Quote.Total != 4200 {
		t.Errorf("options or quote lost: %+v", first)
	}

	second, created, err := st.Create(ctx, newOrder("ord_1"))
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("replay reported an insert")
	}
	if second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("replay returned a different order: %+v vs %+v", second, first)
	}

	// Exactly one timeline entry, not two.
	full, err := st.GetWithTimeline(ctx, "ord_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Timeline) != 1 {
		t.Errorf("timeline = %+v, want one entry", full.Timeline)
	}
}

func TestApplyDeduplicatesOnEventID(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)
	if _, _, err := st.Create(ctx, newOrder("ord_1")); err != nil {
		t.Fatal(err)
	}

	body := raw(t, map[string]any{"orderId": "ord_1", "attempt": 1, "model": "turbo",
		"quantity": 2, "reservationId": "rsv_ord_1"})

	got, err := st.Apply(ctx, "evt_1", ledger.SubjectFabricationRequested, time.Now().UTC(), body)
	if err != nil {
		t.Fatal(err)
	}
	if got != store.Applied {
		t.Fatalf("first apply = %s, want applied", got)
	}

	// JetStream redelivers; the ledger must not act twice.
	for range 3 {
		got, err = st.Apply(ctx, "evt_1", ledger.SubjectFabricationRequested, time.Now().UTC(), body)
		if err != nil {
			t.Fatal(err)
		}
		if got != store.Duplicate {
			t.Fatalf("redelivery = %s, want duplicate", got)
		}
	}

	full, err := st.GetWithTimeline(ctx, "ord_1")
	if err != nil {
		t.Fatal(err)
	}
	if full.Status != ledger.StatusFabricating {
		t.Errorf("status = %q", full.Status)
	}
	// One entry from Create, one from the event: redeliveries added nothing.
	if len(full.Timeline) != 2 {
		t.Errorf("timeline has %d entries, want 2: %+v", len(full.Timeline), full.Timeline)
	}
}

func TestApplyFullHappyPathLifecycle(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)
	if _, _, err := st.Create(ctx, newOrder("ord_1")); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		id, subject string
		data        any
		wantStatus  string
	}{
		{"evt_1", ledger.SubjectFabricationRequested,
			map[string]any{"orderId": "ord_1", "attempt": 1, "reservationId": "rsv_ord_1"},
			ledger.StatusFabricating},
		{"evt_2", ledger.SubjectFabricationStage,
			map[string]any{"orderId": "ord_1", "attempt": 1, "serial": "ENC-a",
				"stage": "prelaminate-base-plate", "stageIndex": 1, "durationMs": 300},
			ledger.StatusFabricating},
		{"evt_3", ledger.SubjectFabricationStage,
			map[string]any{"orderId": "ord_1", "attempt": 1, "serial": "ENC-b",
				"stage": "surmount-logarithmic-casing", "stageIndex": 2, "durationMs": 400},
			ledger.StatusFabricating},
		{"evt_4", ledger.SubjectFabricationCompleted,
			map[string]any{"orderId": "ord_1", "attempt": 1, "model": "turbo",
				"serials": []string{"ENC-a", "ENC-b"}, "durationMs": 3200},
			ledger.StatusInspecting},
		{"evt_5", ledger.SubjectInspectionPassed,
			map[string]any{"orderId": "ord_1", "attempt": 1, "serials": []string{"ENC-a", "ENC-b"}},
			ledger.StatusInspecting},
		{"evt_6", ledger.SubjectShipmentDispatched,
			map[string]any{"orderId": "ord_1", "carrier": "Waneshaft Express",
				"trackingNumber": "NT0123456789", "etaSeconds": 30},
			ledger.StatusDispatched},
		{"evt_7", ledger.SubjectShipmentDelivered,
			map[string]any{"orderId": "ord_1", "carrier": "Waneshaft Express",
				"trackingNumber": "NT0123456789"},
			ledger.StatusDelivered},
	}
	for _, step := range steps {
		if _, err := st.Apply(ctx, step.id, step.subject, time.Now().UTC(), raw(t, step.data)); err != nil {
			t.Fatalf("%s: %v", step.subject, err)
		}
		o, err := st.Get(ctx, "ord_1")
		if err != nil {
			t.Fatal(err)
		}
		if o.Status != step.wantStatus {
			t.Fatalf("after %s status = %q, want %q", step.subject, o.Status, step.wantStatus)
		}
	}

	full, err := st.GetWithTimeline(ctx, "ord_1")
	if err != nil {
		t.Fatal(err)
	}
	if full.Carrier != "Waneshaft Express" || full.TrackingNumber != "NT0123456789" {
		t.Errorf("shipment fields = %q / %q", full.Carrier, full.TrackingNumber)
	}
	if len(full.Serials) != 2 {
		t.Errorf("serials = %v", full.Serials)
	}
	if full.Stage != "" {
		t.Errorf("stage should be cleared once fabrication completes, got %q", full.Stage)
	}
	if len(full.Timeline) != len(steps)+1 {
		t.Errorf("timeline has %d entries, want %d", len(full.Timeline), len(steps)+1)
	}
	for i := 1; i < len(full.Timeline); i++ {
		if full.Timeline[i].Time.Before(full.Timeline[i-1].Time) {
			t.Error("timeline is not in time order")
		}
	}
}

func TestApplyRefabricationDetour(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)
	if _, _, err := st.Create(ctx, newOrder("ord_1")); err != nil {
		t.Fatal(err)
	}

	apply := func(id, subject string, data any) store.ApplyResult {
		t.Helper()
		got, err := st.Apply(ctx, id, subject, time.Now().UTC(), raw(t, data))
		if err != nil {
			t.Fatalf("%s: %v", subject, err)
		}
		return got
	}

	apply("e1", ledger.SubjectFabricationRequested, map[string]any{"orderId": "ord_1", "attempt": 1})
	apply("e2", ledger.SubjectFabricationCompleted,
		map[string]any{"orderId": "ord_1", "attempt": 1, "serials": []string{"ENC-a"}})
	apply("e3", ledger.SubjectInspectionFailed, map[string]any{
		"orderId": "ord_1", "attempt": 1, "serials": []string{"ENC-a"},
		"findings": []map[string]any{{"serial": "ENC-a", "stage": "fit-marzlevanes",
			"metric": "repleneration", "score": 4.7, "reason": "spike"}},
	})
	if o, _ := st.Get(ctx, "ord_1"); o.Status != ledger.StatusRefabricating {
		t.Fatalf("status = %q, want refabricating", o.Status)
	}

	apply("e4", ledger.SubjectFabricationRequested, map[string]any{"orderId": "ord_1", "attempt": 2})
	o, err := st.Get(ctx, "ord_1")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != ledger.StatusFabricating {
		t.Errorf("status = %q, want fabricating", o.Status)
	}
	if o.Attempt != 2 {
		t.Errorf("attempt = %d, want 2", o.Attempt)
	}
}

func TestApplyToleratesOutOfOrderDelivery(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)
	if _, _, err := st.Create(ctx, newOrder("ord_1")); err != nil {
		t.Fatal(err)
	}

	// Delivered arrives before dispatched, and an old fabrication event arrives
	// after both. The ledger must end up delivered, not walk backwards.
	if _, err := st.Apply(ctx, "e1", ledger.SubjectShipmentDelivered, time.Now().UTC(),
		raw(t, map[string]any{"orderId": "ord_1", "carrier": "Grammeter Parcel", "trackingNumber": "NT1"})); err != nil {
		t.Fatal(err)
	}
	got, err := st.Apply(ctx, "e2", ledger.SubjectShipmentDispatched, time.Now().UTC(),
		raw(t, map[string]any{"orderId": "ord_1", "carrier": "Grammeter Parcel", "trackingNumber": "NT1"}))
	if err != nil {
		t.Fatal(err)
	}
	if got != store.Stale {
		t.Errorf("late dispatched = %s, want stale", got)
	}
	got, err = st.Apply(ctx, "e3", ledger.SubjectFabricationStage, time.Now().UTC(),
		raw(t, map[string]any{"orderId": "ord_1", "attempt": 1, "serial": "ENC-a",
			"stage": "wind-lotus-o-delta", "stageIndex": 5, "durationMs": 600}))
	if err != nil {
		t.Fatal(err)
	}
	// The serial is still recorded even though the status does not move.
	if got != store.Applied {
		t.Errorf("late stage = %s", got)
	}

	o, err := st.Get(ctx, "ord_1")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != ledger.StatusDelivered {
		t.Errorf("status = %q, want delivered", o.Status)
	}
	if len(o.Serials) != 1 {
		t.Errorf("serials = %v", o.Serials)
	}
}

func TestApplyOrderAcceptedCanCreateTheRow(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	// The event wins the race against the processor's POST.
	got, err := st.Apply(ctx, "e1", ledger.SubjectOrderAccepted, time.Now().UTC(),
		raw(t, map[string]any{"orderId": "ord_9", "customer": "otto", "model": "hyper",
			"quantity": 4, "options": map[string]bool{"dingleArm": false},
			"total": 16384.0, "currency": "USD"}))
	if err != nil {
		t.Fatal(err)
	}
	if got != store.Applied && got != store.Stale {
		t.Fatalf("apply = %s", got)
	}
	o, err := st.Get(ctx, "ord_9")
	if err != nil {
		t.Fatalf("order was not created from the event: %v", err)
	}
	if o.Customer != "otto" || o.Model != "hyper" || o.Quantity != 4 {
		t.Errorf("order = %+v", o)
	}
	if o.Status != ledger.StatusAccepted {
		t.Errorf("status = %q", o.Status)
	}
}

func TestApplyOrphanEventIsNotSwallowed(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	got, err := st.Apply(ctx, "e1", ledger.SubjectFabricationStage, time.Now().UTC(),
		raw(t, map[string]any{"orderId": "ord_missing", "attempt": 1, "serial": "ENC-a",
			"stage": "prelaminate-base-plate", "stageIndex": 1, "durationMs": 300}))
	if err != nil {
		t.Fatal(err)
	}
	if got != store.Orphan {
		t.Fatalf("apply = %s, want orphan", got)
	}

	// The claim must have rolled back, so a redelivery after the order exists
	// still applies.
	if _, _, err := st.Create(ctx, newOrder("ord_missing")); err != nil {
		t.Fatal(err)
	}
	got, err = st.Apply(ctx, "e1", ledger.SubjectFabricationStage, time.Now().UTC(),
		raw(t, map[string]any{"orderId": "ord_missing", "attempt": 1, "serial": "ENC-a",
			"stage": "prelaminate-base-plate", "stageIndex": 1, "durationMs": 300}))
	if err != nil {
		t.Fatal(err)
	}
	if got != store.Applied {
		t.Fatalf("redelivery after creation = %s, want applied", got)
	}
}

func TestListFilteringOrderingAndCursor(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	for i := range 6 {
		in := newOrder(fmt.Sprintf("ord_%d", i))
		if i%2 == 0 {
			in.Customer = "otto"
		}
		if _, _, err := st.Create(ctx, in); err != nil {
			t.Fatal(err)
		}
		// Distinct created_at values so the ordering is unambiguous.
		time.Sleep(5 * time.Millisecond)
	}

	all, err := st.List(ctx, store.ListQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 6 {
		t.Fatalf("got %d orders", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].CreatedAt.After(all[i-1].CreatedAt) {
			t.Fatal("orders are not newest first")
		}
	}

	mine, err := st.List(ctx, store.ListQuery{Customer: "otto", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 3 {
		t.Errorf("otto has %d orders, want 3", len(mine))
	}
	for _, o := range mine {
		if o.Customer != "otto" {
			t.Errorf("customer filter leaked %+v", o)
		}
	}

	byStatus, err := st.List(ctx, store.ListQuery{Status: ledger.StatusFabricating, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(byStatus) != 0 {
		t.Errorf("nothing is fabricating yet, got %d", len(byStatus))
	}

	// Cursor by order id, then by timestamp: both must page.
	page1, err := st.List(ctx, store.ListQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	page2, err := st.List(ctx, store.ListQuery{Limit: 2, Before: page1[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 || page2[0].ID == page1[1].ID {
		t.Errorf("id cursor did not advance: page1=%v page2=%v", ids(page1), ids(page2))
	}
	page2b, err := st.List(ctx, store.ListQuery{Limit: 2, Before: page1[1].CreatedAt.Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	if ids(page2b)[0] != ids(page2)[0] {
		t.Errorf("timestamp cursor disagrees with the id cursor: %v vs %v", ids(page2b), ids(page2))
	}

	if _, err := st.List(ctx, store.ListQuery{Before: "ord_nope"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown cursor = %v, want ErrNotFound", err)
	}
}

func TestStatsCountsAndThroughput(t *testing.T) {
	st := newStore(t)
	ctx := testsupport.Context(t)

	for i := range 3 {
		if _, _, err := st.Create(ctx, newOrder(fmt.Sprintf("ord_%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	// Push one order all the way to delivered.
	for i, step := range []struct {
		subject string
		data    any
	}{
		{ledger.SubjectFabricationRequested, map[string]any{"orderId": "ord_0", "attempt": 1}},
		{ledger.SubjectFabricationCompleted, map[string]any{"orderId": "ord_0", "attempt": 1, "serials": []string{"a", "b"}}},
		{ledger.SubjectShipmentDispatched, map[string]any{"orderId": "ord_0", "carrier": "c", "trackingNumber": "NT1"}},
		{ledger.SubjectShipmentDelivered, map[string]any{"orderId": "ord_0", "carrier": "c", "trackingNumber": "NT1"}},
	} {
		if _, err := st.Apply(ctx, fmt.Sprintf("e%d", i), step.subject, time.Now().UTC(), raw(t, step.data)); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 3 {
		t.Errorf("total = %d", stats.Total)
	}
	if stats.Counts[ledger.StatusDelivered] != 1 || stats.Counts[ledger.StatusAccepted] != 2 {
		t.Errorf("counts = %v", stats.Counts)
	}
	if stats.Throughput.OrdersAccepted != 3 {
		t.Errorf("ordersAccepted = %d, want 3", stats.Throughput.OrdersAccepted)
	}
	// ord_0 carries quantity 2, so two units were delivered.
	if stats.Throughput.UnitsDelivered != 2 {
		t.Errorf("unitsDelivered = %d, want 2", stats.Throughput.UnitsDelivered)
	}
	if stats.WindowSeconds != 300 {
		t.Errorf("windowSeconds = %d", stats.WindowSeconds)
	}
}

func TestGetUnknownOrder(t *testing.T) {
	st := newStore(t)
	if _, err := st.Get(testsupport.Context(t), "ord_nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if _, err := st.GetWithTimeline(testsupport.Context(t), "ord_nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestPingIsTheReadinessCheck(t *testing.T) {
	st := newStore(t)
	if err := st.Ping(testsupport.Context(t)); err != nil {
		t.Fatalf("Ping: %v", err)
	}

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

func ids(orders []ledger.Order) []string {
	out := make([]string, 0, len(orders))
	for _, o := range orders {
		out = append(out, o.ID)
	}
	return out
}
