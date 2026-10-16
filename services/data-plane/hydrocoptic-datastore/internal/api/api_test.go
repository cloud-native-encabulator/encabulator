package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/identity"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/ledger"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/store"
)

// fakeStore is an in-memory ledger good enough to exercise the handlers.
type fakeStore struct {
	orders  map[string]ledger.Order
	order   []string // insertion order, oldest first
	lastQ   store.ListQuery
	failNow error
}

func newFakeStore(orders ...ledger.Order) *fakeStore {
	f := &fakeStore{orders: map[string]ledger.Order{}}
	for _, o := range orders {
		f.orders[o.ID] = o
		f.order = append(f.order, o.ID)
	}
	return f
}

func (f *fakeStore) Create(_ context.Context, in store.NewOrder) (ledger.Order, bool, error) {
	if f.failNow != nil {
		return ledger.Order{}, false, f.failNow
	}
	if existing, ok := f.orders[in.ID]; ok {
		return existing, false, nil
	}
	o := ledger.Order{
		ID: in.ID, Customer: in.Customer, Model: in.Model, Quantity: in.Quantity,
		Options: in.Options, Status: ledger.StatusAccepted, Attempt: 1, Quote: in.Quote,
		ReservationID: in.ReservationID, Serials: []string{},
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	f.orders[in.ID] = o
	f.order = append(f.order, in.ID)
	return o, true, nil
}

func (f *fakeStore) Get(_ context.Context, id string) (ledger.Order, error) {
	if f.failNow != nil {
		return ledger.Order{}, f.failNow
	}
	o, ok := f.orders[id]
	if !ok {
		return ledger.Order{}, store.ErrNotFound
	}
	return o, nil
}

func (f *fakeStore) GetWithTimeline(ctx context.Context, id string) (ledger.Order, error) {
	o, err := f.Get(ctx, id)
	if err != nil {
		return ledger.Order{}, err
	}
	o.Timeline = []ledger.TimelineEntry{{
		Time: o.CreatedAt, Type: ledger.SubjectOrderAccepted, Detail: "Order accepted",
	}}
	return o, nil
}

func (f *fakeStore) List(_ context.Context, q store.ListQuery) ([]ledger.Order, error) {
	f.lastQ = q
	if f.failNow != nil {
		return nil, f.failNow
	}
	ids := slices.Clone(f.order)
	slices.Reverse(ids) // newest first
	out := make([]ledger.Order, 0, len(ids))
	for _, id := range ids {
		o := f.orders[id]
		if q.Customer != "" && o.Customer != q.Customer {
			continue
		}
		if q.Status != "" && o.Status != q.Status {
			continue
		}
		out = append(out, o)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeStore) Stats(context.Context) (store.Stats, error) {
	if f.failNow != nil {
		return store.Stats{}, f.failNow
	}
	counts := map[string]int{}
	for _, o := range f.orders {
		counts[o.Status]++
	}
	return store.Stats{
		Counts: counts, Total: len(f.orders), WindowSeconds: 300,
		Throughput: store.Throughput{OrdersAccepted: len(f.orders), UnitsDelivered: 0},
	}, nil
}

func order(id, customer, model, status string) ledger.Order {
	return ledger.Order{
		ID: id, Customer: customer, Model: model, Quantity: 1, Status: status,
		Attempt: 1, Serials: []string{},
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

func newServer(t *testing.T, s api.Store) *httpx.Server {
	t.Helper()
	srv := httpx.NewServer(httpx.ServerConfig{
		Service: "hydrocoptic-datastore", Addr: "127.0.0.1:0",
		Logger: slog.New(slog.DiscardHandler), Registry: prometheus.NewRegistry(),
	})
	api.New(s, slog.New(slog.DiscardHandler), srv.Registry()).Routes(srv)
	return srv
}

func do(t *testing.T, srv *httpx.Server, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func as(req *http.Request, subject string, roles ...string) *http.Request {
	req.Header.Set(identity.HeaderSubject, subject)
	req.Header.Set(identity.HeaderRoles, strings.Join(roles, ","))
	return req
}

func post(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/v1/orders", strings.NewReader(body))
}

const validOrder = `{"id":"ord_1","customer":"wendy","model":"turbo","quantity":2,
	"options":{"dingleArm":true},"quote":{"id":"qte_1","total":4200,"currency":"USD"},
	"reservationId":"rsv_1"}`

func TestCreateOrder(t *testing.T) {
	srv := newServer(t, newFakeStore())

	rec := do(t, srv, post(validOrder))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var o ledger.Order
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	if o.ID != "ord_1" || o.Status != ledger.StatusAccepted || o.Attempt != 1 {
		t.Errorf("order = %+v", o)
	}
	if !o.Options.DingleArm {
		t.Error("dingleArm lost")
	}
	if o.Quote == nil || o.Quote.Total != 4200 {
		t.Errorf("quote = %+v", o.Quote)
	}
	if o.ReservationID != "rsv_1" {
		t.Errorf("reservationId = %q", o.ReservationID)
	}
}

func TestCreateOrderIsIdempotent(t *testing.T) {
	srv := newServer(t, newFakeStore())

	if rec := do(t, srv, post(validOrder)); rec.Code != http.StatusCreated {
		t.Fatalf("first status = %d", rec.Code)
	}
	rec := do(t, srv, post(validOrder))
	if rec.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", rec.Code)
	}
	var o ledger.Order
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	if o.ID != "ord_1" {
		t.Errorf("replay returned %+v", o)
	}
}

func TestCreateOrderValidation(t *testing.T) {
	srv := newServer(t, newFakeStore())
	cases := []struct {
		name, body string
	}{
		{"missing id", `{"customer":"wendy","model":"turbo","quantity":1}`},
		{"missing customer", `{"id":"ord_1","model":"turbo","quantity":1}`},
		{"unknown model", `{"id":"ord_1","customer":"wendy","model":"quantum","quantity":1}`},
		{"quantity zero", `{"id":"ord_1","customer":"wendy","model":"turbo","quantity":0}`},
		{"quantity eleven", `{"id":"ord_1","customer":"wendy","model":"turbo","quantity":11}`},
		{"negative quote", `{"id":"ord_1","customer":"wendy","model":"turbo","quantity":1,"quote":{"total":-1}}`},
		{"unknown field", `{"id":"ord_1","customer":"wendy","model":"turbo","quantity":1,"nonsense":true}`},
		{"not json", `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, srv, post(tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
			}
			var env httpx.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if env.Error.Code == "" {
				t.Error("error has no code")
			}
		})
	}
}

func TestListFiltersToOwnOrdersWithoutAStaffRole(t *testing.T) {
	fs := newFakeStore(
		order("ord_1", "wendy", "turbo", ledger.StatusAccepted),
		order("ord_2", "otto", "retro", ledger.StatusFabricating),
	)
	srv := newServer(t, fs)

	// A customer asking for someone else's orders still gets only its own.
	rec := do(t, srv, as(httptest.NewRequest(http.MethodGet, "/v1/orders?customer=otto", nil),
		"wendy", identity.RoleCustomer))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if fs.lastQ.Customer != "wendy" {
		t.Errorf("query customer = %q, want the caller's own subject", fs.lastQ.Customer)
	}
	var body api.ListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, o := range body.Orders {
		if o.Customer != "wendy" {
			t.Errorf("leaked another customer's order: %+v", o)
		}
	}
}

func TestListHonorsTheQueryForStaff(t *testing.T) {
	fs := newFakeStore(
		order("ord_1", "wendy", "turbo", ledger.StatusAccepted),
		order("ord_2", "otto", "retro", ledger.StatusFabricating),
	)
	srv := newServer(t, fs)

	for _, role := range []string{identity.RoleOperator, identity.RoleViewer, identity.RoleAdmin} {
		rec := do(t, srv, as(httptest.NewRequest(http.MethodGet, "/v1/orders", nil), "opal", role))
		var body api.ListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Orders) != 2 {
			t.Errorf("role %s saw %d orders, want 2", role, len(body.Orders))
		}
		if body.Orders[0].ID != "ord_2" {
			t.Errorf("orders are not newest first: %s first", body.Orders[0].ID)
		}
	}
}

func TestListLimitAndCursor(t *testing.T) {
	orders := make([]ledger.Order, 0, 5)
	for i := range 5 {
		orders = append(orders, order(fmt.Sprintf("ord_%d", i), "wendy", "turbo", ledger.StatusAccepted))
	}
	fs := newFakeStore(orders...)
	srv := newServer(t, fs)

	rec := do(t, srv, as(httptest.NewRequest(http.MethodGet, "/v1/orders?limit=2", nil), "opal", "operator"))
	var body api.ListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Orders) != 2 {
		t.Fatalf("got %d orders", len(body.Orders))
	}
	if body.NextBefore != body.Orders[1].ID {
		t.Errorf("nextBefore = %q, want %q", body.NextBefore, body.Orders[1].ID)
	}

	// The cursor is handed straight to the store.
	do(t, srv, as(httptest.NewRequest(http.MethodGet, "/v1/orders?limit=2&before="+body.NextBefore, nil),
		"opal", "operator"))
	if fs.lastQ.Before != body.NextBefore {
		t.Errorf("cursor = %q", fs.lastQ.Before)
	}
}

func TestListRejectsBadQueries(t *testing.T) {
	srv := newServer(t, newFakeStore())
	for _, target := range []string{"/v1/orders?limit=0", "/v1/orders?limit=500", "/v1/orders?limit=abc",
		"/v1/orders?status=invented"} {
		rec := do(t, srv, as(httptest.NewRequest(http.MethodGet, target, nil), "opal", "operator"))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", target, rec.Code)
		}
	}
}

func TestGetOrderVisibility(t *testing.T) {
	srv := newServer(t, newFakeStore(order("ord_1", "wendy", "turbo", ledger.StatusAccepted)))

	// The owner sees it, with a timeline.
	rec := do(t, srv, as(httptest.NewRequest(http.MethodGet, "/v1/orders/ord_1", nil), "wendy", "customer"))
	if rec.Code != http.StatusOK {
		t.Fatalf("owner status = %d", rec.Code)
	}
	var o ledger.Order
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	if len(o.Timeline) == 0 {
		t.Error("no timeline on the single-order view")
	}

	// Another customer gets a 404, not a 403: ids must not be probeable.
	rec = do(t, srv, as(httptest.NewRequest(http.MethodGet, "/v1/orders/ord_1", nil), "otto", "customer"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("other customer status = %d, want 404", rec.Code)
	}

	// Staff see it.
	rec = do(t, srv, as(httptest.NewRequest(http.MethodGet, "/v1/orders/ord_1", nil), "opal", "viewer"))
	if rec.Code != http.StatusOK {
		t.Errorf("viewer status = %d", rec.Code)
	}

	// A missing order is a 404 for everyone.
	rec = do(t, srv, as(httptest.NewRequest(http.MethodGet, "/v1/orders/ord_nope", nil), "opal", "admin"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing order status = %d", rec.Code)
	}
}

func TestStats(t *testing.T) {
	srv := newServer(t, newFakeStore(
		order("ord_1", "wendy", "turbo", ledger.StatusAccepted),
		order("ord_2", "otto", "retro", ledger.StatusDelivered),
	))

	rec := do(t, srv, httptest.NewRequest(http.MethodGet, "/v1/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var stats store.Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 {
		t.Errorf("total = %d", stats.Total)
	}
	if stats.Counts[ledger.StatusAccepted] != 1 || stats.Counts[ledger.StatusDelivered] != 1 {
		t.Errorf("counts = %v", stats.Counts)
	}
	if stats.WindowSeconds != 300 {
		t.Errorf("windowSeconds = %d, want 300", stats.WindowSeconds)
	}
}

func TestStoreFailuresBecome500NotAPanic(t *testing.T) {
	fs := newFakeStore()
	fs.failNow = context.DeadlineExceeded
	srv := newServer(t, fs)

	for _, req := range []*http.Request{
		post(validOrder),
		as(httptest.NewRequest(http.MethodGet, "/v1/orders", nil), "opal", "operator"),
		as(httptest.NewRequest(http.MethodGet, "/v1/orders/ord_1", nil), "opal", "operator"),
		httptest.NewRequest(http.MethodGet, "/v1/stats", nil),
	} {
		rec := do(t, srv, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s %s -> %d, want 500", req.Method, req.URL.Path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "context deadline") {
			t.Error("internal error detail leaked to the client")
		}
	}
}
