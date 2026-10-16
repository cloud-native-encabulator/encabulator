// Package api serves the order ledger's HTTP API (design 8.6).
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/identity"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/ledger"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/store"
)

// Quantity bounds from design section 3.
const (
	MinQuantity = 1
	MaxQuantity = 10
)

// Store is the persistence the API needs.
type Store interface {
	Create(ctx context.Context, in store.NewOrder) (ledger.Order, bool, error)
	Get(ctx context.Context, id string) (ledger.Order, error)
	GetWithTimeline(ctx context.Context, id string) (ledger.Order, error)
	List(ctx context.Context, q store.ListQuery) ([]ledger.Order, error)
	Stats(ctx context.Context) (store.Stats, error)
}

// API holds the handlers.
type API struct {
	store Store
	log   *slog.Logger

	created *prometheus.CounterVec
}

// New builds the API and registers its domain metrics.
func New(s Store, log *slog.Logger, reg prometheus.Registerer) *API {
	return &API{
		store: s,
		log:   log,
		created: httpx.RegisterCounterVec(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "encabulator_ledger_orders_created_total",
			Help: "Orders written to the ledger, by model and whether the write was a replay.",
		}, []string{"model", "result"})),
	}
}

// Routes registers every route on the kit's HTTP server.
func (a *API) Routes(s *httpx.Server) {
	s.Handle("POST /v1/orders", http.HandlerFunc(a.CreateOrder))
	s.Handle("GET /v1/orders", http.HandlerFunc(a.ListOrders))
	s.Handle("GET /v1/orders/{id}", http.HandlerFunc(a.GetOrder))
	s.Handle("GET /v1/stats", http.HandlerFunc(a.Stats))
}

// CreateRequest is the body of POST /v1/orders.
type CreateRequest struct {
	ID            string         `json:"id"`
	Customer      string         `json:"customer"`
	Model         string         `json:"model"`
	Quantity      int            `json:"quantity"`
	Options       ledger.Options `json:"options"`
	Quote         *ledger.Quote  `json:"quote,omitempty"`
	ReservationID string         `json:"reservationId,omitempty"`
}

// Validate checks the body. The ledger is the system of record, so it refuses
// anything it could not explain later.
func (r CreateRequest) Validate() error {
	switch {
	case strings.TrimSpace(r.ID) == "":
		return httpx.Errorf(http.StatusBadRequest, "invalid_order", "id is required")
	case len(r.ID) > 64:
		return httpx.Errorf(http.StatusBadRequest, "invalid_order", "id must be 64 characters or fewer")
	case strings.TrimSpace(r.Customer) == "":
		return httpx.Errorf(http.StatusBadRequest, "invalid_order", "customer is required")
	case r.Model != ledger.ModelTurbo && r.Model != ledger.ModelRetro && r.Model != ledger.ModelHyper:
		return httpx.Errorf(http.StatusBadRequest, "invalid_order",
			"model must be one of turbo, retro, hyper")
	case r.Quantity < MinQuantity || r.Quantity > MaxQuantity:
		return httpx.Errorf(http.StatusBadRequest, "invalid_order",
			"quantity must be between %d and %d", MinQuantity, MaxQuantity)
	}
	if r.Quote != nil && r.Quote.Total < 0 {
		return httpx.Errorf(http.StatusBadRequest, "invalid_order", "quote total must not be negative")
	}
	return nil
}

// CreateOrder serves POST /v1/orders. It is idempotent on the order id: a replay
// returns 200 with the stored order, a new order returns 201.
func (a *API) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := httpx.DecodeJSON(r, &req, 64<<10); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := req.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}

	order, created, err := a.store.Create(r.Context(), store.NewOrder{
		ID:            req.ID,
		Customer:      req.Customer,
		Model:         req.Model,
		Quantity:      req.Quantity,
		Options:       req.Options,
		Quote:         req.Quote,
		ReservationID: req.ReservationID,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}

	if created {
		a.created.WithLabelValues(order.Model, "created").Inc()
		a.log.InfoContext(r.Context(), "order written to the ledger",
			slog.String("order_id", order.ID),
			slog.String("model", order.Model),
			slog.Int("quantity", order.Quantity))
		httpx.WriteJSON(w, http.StatusCreated, order)
		return
	}
	a.created.WithLabelValues(order.Model, "replayed").Inc()
	httpx.WriteJSON(w, http.StatusOK, order)
}

// ListResponse is the body of GET /v1/orders.
type ListResponse struct {
	Orders []ledger.Order `json:"orders"`
	// NextBefore is the cursor to pass as ?before= for the following page. It is
	// empty on the last page.
	NextBefore string `json:"nextBefore,omitempty"`
}

// ListOrders serves GET /v1/orders?customer=&status=&limit=&before=.
func (a *API) ListOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := parseLimit(q.Get("limit"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	status := q.Get("status")
	if status != "" && ledger.Rank(status) == 0 {
		a.fail(w, r, httpx.Errorf(http.StatusBadRequest, "invalid_status", "unknown status %q", status))
		return
	}

	query := store.ListQuery{
		Customer: q.Get("customer"),
		Status:   status,
		Limit:    limit,
		Before:   q.Get("before"),
	}
	// A caller without a staff role only ever sees its own orders, whatever the
	// query says.
	if id := identity.FromRequest(r); !id.IsStaff() {
		query.Customer = id.Subject
	}

	orders, err := a.store.List(r.Context(), query)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// An unknown cursor: treat it as the end of the list rather than a 404.
			httpx.WriteJSON(w, http.StatusOK, ListResponse{Orders: []ledger.Order{}})
			return
		}
		a.fail(w, r, err)
		return
	}

	resp := ListResponse{Orders: orders}
	if len(orders) == limit {
		resp.NextBefore = orders[len(orders)-1].ID
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// GetOrder serves GET /v1/orders/{id}, including the timeline.
func (a *API) GetOrder(w http.ResponseWriter, r *http.Request) {
	order, err := a.store.GetWithTimeline(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// Invisible is indistinguishable from absent, so a customer cannot probe for
	// other customers' order ids.
	if id := identity.FromRequest(r); !id.IsStaff() && order.Customer != id.Subject {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "No such order")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, order)
}

// Stats serves GET /v1/stats.
func (a *API) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.Stats(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, stats)
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 200 {
		return 0, httpx.Errorf(http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 200")
	}
	return n, nil
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "No such order")
		return
	}
	var se *httpx.StatusError
	if errors.As(err, &se) {
		httpx.WriteErr(w, se)
		return
	}
	a.log.ErrorContext(r.Context(), "ledger request failed", slog.Any("error", err))
	httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "Internal error")
}
