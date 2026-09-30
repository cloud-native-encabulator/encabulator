// Package store is the order ledger's Postgres persistence.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/ledger"
)

// ErrNotFound is returned when an order does not exist.
var ErrNotFound = errors.New("order not found")

// ThroughputWindow is the window /v1/stats reports over.
const ThroughputWindow = 5 * time.Minute

// Store is the order ledger.
type Store struct {
	pool *pgxpool.Pool
}

// New connects to Postgres with OTel instrumentation on the pool.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse DATABASE_URL: %w", err)
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName())
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Pool exposes the connection pool for migrations.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping is the readiness check.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// NewOrder is the body of POST /v1/orders as the store sees it.
type NewOrder struct {
	ID            string
	Customer      string
	Model         string
	Quantity      int
	Options       ledger.Options
	Quote         *ledger.Quote
	ReservationID string
}

// Create inserts an order with status accepted. It is idempotent on the id: a
// replay returns the stored order and created=false.
func (s *Store) Create(ctx context.Context, in NewOrder) (ledger.Order, bool, error) {
	options, err := json.Marshal(in.Options)
	if err != nil {
		return ledger.Order{}, false, fmt.Errorf("store: encode options: %w", err)
	}
	var quote []byte
	if in.Quote != nil {
		if quote, err = json.Marshal(in.Quote); err != nil {
			return ledger.Order{}, false, fmt.Errorf("store: encode quote: %w", err)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ledger.Order{}, false, fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var inserted bool
	err = tx.QueryRow(ctx, `
		INSERT INTO orders (id, customer, model, quantity, options, status, attempt, quote, reservation_id)
		VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $8)
		ON CONFLICT (id) DO NOTHING
		RETURNING true`,
		in.ID, in.Customer, in.Model, in.Quantity, options, ledger.StatusAccepted, quote, in.ReservationID,
	).Scan(&inserted)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		inserted = false
	case err != nil:
		return ledger.Order{}, false, fmt.Errorf("store: insert order: %w", err)
	}

	if inserted {
		if err := appendTimeline(ctx, tx, in.ID, time.Now().UTC(), ledger.SubjectOrderAccepted,
			fmt.Sprintf("Order accepted: %d x %s", in.Quantity, in.Model)); err != nil {
			return ledger.Order{}, false, err
		}
	}

	order, err := getOrderTx(ctx, tx, in.ID)
	if err != nil {
		return ledger.Order{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ledger.Order{}, false, fmt.Errorf("store: commit: %w", err)
	}
	return order, inserted, nil
}

// Get returns one order without its timeline.
func (s *Store) Get(ctx context.Context, id string) (ledger.Order, error) {
	rows, err := s.pool.Query(ctx, selectOrders+` WHERE id = $1`, id)
	if err != nil {
		return ledger.Order{}, fmt.Errorf("store: get order: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return ledger.Order{}, fmt.Errorf("store: get order: %w", err)
		}
		return ledger.Order{}, ErrNotFound
	}
	return scanOrder(rows)
}

// GetWithTimeline returns one order and its timeline, oldest entry first.
func (s *Store) GetWithTimeline(ctx context.Context, id string) (ledger.Order, error) {
	order, err := s.Get(ctx, id)
	if err != nil {
		return ledger.Order{}, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT time, type, detail FROM order_timeline WHERE order_id = $1 ORDER BY time, id`, id)
	if err != nil {
		return ledger.Order{}, fmt.Errorf("store: get timeline: %w", err)
	}
	defer rows.Close()

	order.Timeline = make([]ledger.TimelineEntry, 0, 16)
	for rows.Next() {
		var e ledger.TimelineEntry
		if err := rows.Scan(&e.Time, &e.Type, &e.Detail); err != nil {
			return ledger.Order{}, fmt.Errorf("store: scan timeline: %w", err)
		}
		e.Time = e.Time.UTC()
		order.Timeline = append(order.Timeline, e)
	}
	if err := rows.Err(); err != nil {
		return ledger.Order{}, fmt.Errorf("store: get timeline: %w", err)
	}
	return order, nil
}

// ListQuery narrows GET /v1/orders.
type ListQuery struct {
	Customer string
	Status   string
	Limit    int
	// Before is an exclusive cursor: either an order id or an RFC 3339 time.
	Before string
}

// List returns orders newest first.
func (s *Store) List(ctx context.Context, q ListQuery) ([]ledger.Order, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	add := func(cond string, arg any) {
		args = append(args, arg)
		conditions = append(conditions, fmt.Sprintf(cond, len(args)))
	}
	if q.Customer != "" {
		add("customer = $%d", q.Customer)
	}
	if q.Status != "" {
		add("status = $%d", q.Status)
	}
	if q.Before != "" {
		cursor, err := s.resolveCursor(ctx, q.Before)
		if err != nil {
			return nil, err
		}
		add("created_at < $%d", cursor)
	}

	sql := selectOrders
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	args = append(args, limit)
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list orders: %w", err)
	}
	defer rows.Close()

	out := make([]ledger.Order, 0, limit)
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list orders: %w", err)
	}
	return out, nil
}

// resolveCursor accepts either an RFC 3339 timestamp or an order id.
func (s *Store) resolveCursor(ctx context.Context, before string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339Nano, before); err == nil {
		return ts.UTC(), nil
	}
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `SELECT created_at FROM orders WHERE id = $1`, before).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("store: resolve cursor: %w", err)
	}
	return createdAt.UTC(), nil
}

// Stats is the body of GET /v1/stats.
type Stats struct {
	Counts        map[string]int `json:"counts"`
	Total         int            `json:"total"`
	WindowSeconds int            `json:"windowSeconds"`
	Throughput    Throughput     `json:"throughput"`
}

// Throughput reports the recent flow through the works.
type Throughput struct {
	OrdersAccepted int `json:"ordersAccepted"`
	UnitsDelivered int `json:"unitsDelivered"`
}

// Stats returns counts by status plus throughput over the last five minutes.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	out := Stats{Counts: map[string]int{}, WindowSeconds: int(ThroughputWindow.Seconds())}

	rows, err := s.pool.Query(ctx, `SELECT status, count(*) FROM orders GROUP BY status`)
	if err != nil {
		return Stats{}, fmt.Errorf("store: stats by status: %w", err)
	}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return Stats{}, fmt.Errorf("store: scan stats: %w", err)
		}
		out.Counts[status] = n
		out.Total += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Stats{}, fmt.Errorf("store: stats by status: %w", err)
	}

	since := time.Now().Add(-ThroughputWindow)
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM orders WHERE created_at >= $1`, since,
	).Scan(&out.Throughput.OrdersAccepted); err != nil {
		return Stats{}, fmt.Errorf("store: stats orders accepted: %w", err)
	}

	// Units delivered: the quantity of every order whose delivery landed in the
	// window. Reads the timeline so a redelivered event cannot double-count.
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(o.quantity), 0)
		  FROM orders o
		  JOIN (
		       SELECT DISTINCT order_id
		         FROM order_timeline
		        WHERE type = $1 AND time >= $2
		  ) t ON t.order_id = o.id`,
		ledger.SubjectShipmentDelivered, since,
	).Scan(&out.Throughput.UnitsDelivered); err != nil {
		return Stats{}, fmt.Errorf("store: stats units delivered: %w", err)
	}

	return out, nil
}

// ApplyResult reports what Apply did with an event.
type ApplyResult string

// Apply outcomes.
const (
	// Applied means the event changed the ledger.
	Applied ApplyResult = "applied"
	// Duplicate means the event id had already been recorded.
	Duplicate ApplyResult = "duplicate"
	// Stale means the event was new but implied no forward change.
	Stale ApplyResult = "stale"
	// Orphan means no such order exists and the event cannot create one.
	Orphan ApplyResult = "orphan"
)

// Apply folds one event into the ledger, exactly once. Deduplication and the
// update share a transaction, so a crash between them cannot lose or repeat an
// event.
func (s *Store) Apply(ctx context.Context, eventID, subject string, eventTime time.Time, data json.RawMessage) (ApplyResult, error) {
	orderID, up, err := ledger.Interpret(subject, eventTime, data)
	if err != nil {
		return "", err
	}
	if orderID == "" {
		return Stale, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("store: begin apply: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var claimed bool
	err = tx.QueryRow(ctx, `
		INSERT INTO processed_events (event_id, subject, order_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (event_id) DO NOTHING
		RETURNING true`,
		eventID, subject, orderID,
	).Scan(&claimed)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Duplicate, nil
	case err != nil:
		return "", fmt.Errorf("store: claim event: %w", err)
	}

	current, err := getOrderTx(ctx, tx, orderID)
	switch {
	case errors.Is(err, ErrNotFound):
		if up.Create == nil {
			// Out-of-order delivery: the order will arrive; let the event be
			// redelivered rather than inventing a row from partial data.
			return Orphan, nil
		}
		if err := insertFromEvent(ctx, tx, *up.Create); err != nil {
			return "", err
		}
		current, err = getOrderTx(ctx, tx, orderID)
		if err != nil {
			return "", err
		}
	case err != nil:
		return "", err
	}

	changed, err := applyUpdate(ctx, tx, current, up, eventTime)
	if err != nil {
		return "", err
	}
	if err := appendTimeline(ctx, tx, orderID, eventTime, subject, up.Detail); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("store: commit apply: %w", err)
	}
	if changed {
		return Applied, nil
	}
	return Stale, nil
}

func applyUpdate(ctx context.Context, tx pgx.Tx, current ledger.Order, up ledger.Update, eventTime time.Time) (bool, error) {
	sets := make([]string, 0, 8)
	args := make([]any, 0, 8)
	set := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)+1))
	}

	if up.Status != "" && ledger.AllowTransition(current.Status, up.Status) {
		set("status", up.Status)
	}
	if up.Stage != nil && *up.Stage != current.Stage {
		set("stage", *up.Stage)
	}
	if up.Attempt != nil && *up.Attempt > current.Attempt {
		set("attempt", *up.Attempt)
	}
	if merged := ledger.MergeSerials(current.Serials, up.Serials); len(merged) != len(current.Serials) {
		set("serials", merged)
	}
	if up.Carrier != nil && *up.Carrier != "" && *up.Carrier != current.Carrier {
		set("carrier", *up.Carrier)
	}
	if up.TrackingNumber != nil && *up.TrackingNumber != "" && *up.TrackingNumber != current.TrackingNumber {
		set("tracking_number", *up.TrackingNumber)
	}
	if len(sets) == 0 {
		return false, nil
	}

	sets = append(sets, "updated_at = GREATEST(updated_at, $1)")
	sql := fmt.Sprintf("UPDATE orders SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args)+2)
	full := append([]any{eventTime}, args...)
	full = append(full, current.ID)
	if _, err := tx.Exec(ctx, sql, full...); err != nil {
		return false, fmt.Errorf("store: update order: %w", err)
	}
	return true, nil
}

func insertFromEvent(ctx context.Context, tx pgx.Tx, order ledger.Order) error {
	options, err := json.Marshal(order.Options)
	if err != nil {
		return fmt.Errorf("store: encode options: %w", err)
	}
	var quote []byte
	if order.Quote != nil {
		if quote, err = json.Marshal(order.Quote); err != nil {
			return fmt.Errorf("store: encode quote: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO orders (id, customer, model, quantity, options, status, attempt, quote, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		ON CONFLICT (id) DO NOTHING`,
		order.ID, order.Customer, order.Model, max(order.Quantity, 1), options,
		order.Status, max(order.Attempt, 1), quote, order.CreatedAt,
	); err != nil {
		return fmt.Errorf("store: insert order from event: %w", err)
	}
	return nil
}

func appendTimeline(ctx context.Context, tx pgx.Tx, orderID string, at time.Time, eventType, detail string) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO order_timeline (order_id, time, type, detail) VALUES ($1, $2, $3, $4)`,
		orderID, at, eventType, detail,
	); err != nil {
		return fmt.Errorf("store: append timeline: %w", err)
	}
	return nil
}

const selectOrders = `
	SELECT id, customer, model, quantity, options, status, stage, attempt, quote,
	       reservation_id, serials, carrier, tracking_number, created_at, updated_at
	  FROM orders`

func getOrderTx(ctx context.Context, tx pgx.Tx, id string) (ledger.Order, error) {
	rows, err := tx.Query(ctx, selectOrders+` WHERE id = $1`, id)
	if err != nil {
		return ledger.Order{}, fmt.Errorf("store: get order: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return ledger.Order{}, fmt.Errorf("store: get order: %w", err)
		}
		return ledger.Order{}, ErrNotFound
	}
	return scanOrder(rows)
}

func scanOrder(rows pgx.Rows) (ledger.Order, error) {
	var (
		o       ledger.Order
		options []byte
		quote   []byte
	)
	if err := rows.Scan(
		&o.ID, &o.Customer, &o.Model, &o.Quantity, &options, &o.Status, &o.Stage, &o.Attempt,
		&quote, &o.ReservationID, &o.Serials, &o.Carrier, &o.TrackingNumber, &o.CreatedAt, &o.UpdatedAt,
	); err != nil {
		return ledger.Order{}, fmt.Errorf("store: scan order: %w", err)
	}
	if len(options) > 0 {
		if err := json.Unmarshal(options, &o.Options); err != nil {
			return ledger.Order{}, fmt.Errorf("store: decode options: %w", err)
		}
	}
	if len(quote) > 0 {
		var q ledger.Quote
		if err := json.Unmarshal(quote, &q); err != nil {
			return ledger.Order{}, fmt.Errorf("store: decode quote: %w", err)
		}
		o.Quote = &q
	}
	if o.Serials == nil {
		o.Serials = []string{}
	}
	o.CreatedAt = o.CreatedAt.UTC()
	o.UpdatedAt = o.UpdatedAt.UTC()
	return o, nil
}
