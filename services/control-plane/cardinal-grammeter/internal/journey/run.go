package journey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/customer"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/gateway"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/rng"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/stats"
)

// Metrics is the slice of Prometheus bookkeeping a journey performs.
type Metrics interface {
	// ObserveRequest counts one HTTP call to the gateway.
	ObserveRequest(journey, status string)
}

// Recorder is the rolling-window statistics sink.
type Recorder interface {
	RecordJourney(name string, duration time.Duration, failed bool)
	RecordRequest(status int)
}

// Runner performs one journey at a time on behalf of a randomly chosen customer.
// It is safe for concurrent use.
type Runner struct {
	gw      *gateway.Client
	pool    *customer.Pool
	rec     Recorder
	metrics Metrics
	source  rng.Source
	log     *slog.Logger
	now     func() time.Time
	timeout time.Duration
}

// RunnerOptions configures a Runner. Zero values get sensible defaults.
type RunnerOptions struct {
	Gateway *gateway.Client
	Pool    *customer.Pool
	Stats   Recorder
	Metrics Metrics
	Source  rng.Source
	Logger  *slog.Logger
	Now     func() time.Time
	// Timeout bounds a whole journey, including its retries.
	Timeout time.Duration
}

// NewRunner builds a Runner.
func NewRunner(o RunnerOptions) *Runner {
	if o.Source == nil {
		o.Source = rng.Global()
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Second
	}
	if o.Metrics == nil {
		o.Metrics = nopMetrics{}
	}
	return &Runner{
		gw:      o.Gateway,
		pool:    o.Pool,
		rec:     o.Stats,
		metrics: o.Metrics,
		source:  o.Source,
		log:     o.Logger,
		now:     o.Now,
		timeout: o.Timeout,
	}
}

type nopMetrics struct{}

func (nopMetrics) ObserveRequest(string, string) {}

// Run executes one journey and records the outcome. It never returns an error:
// a failed journey is data, not a program fault.
func (r *Runner) Run(ctx context.Context, name string) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	start := r.now()
	err := r.dispatch(ctx, name)
	if r.rec != nil {
		r.rec.RecordJourney(name, r.now().Sub(start), err != nil)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		r.log.Debug("journey did not complete", "journey", name, "error", err.Error())
	}
}

func (r *Runner) dispatch(ctx context.Context, name string) error {
	cust := r.pool.Pick(r.source)
	if cust == nil {
		return errors.New("customer pool is empty")
	}
	switch name {
	case Browse:
		return r.browse(ctx, name, cust)
	case Quote:
		return r.quote(ctx, name, cust)
	case Order:
		return r.order(ctx, name, cust)
	case Track:
		return r.track(ctx, name, cust)
	default:
		return fmt.Errorf("unknown journey %q", name)
	}
}

// failed decides whether a status counts against the journey. Business
// rejections (400, 404, 409) are normal factory life; server faults and rate
// limiting are the symptoms the demo exists to show.
func failed(status int) bool {
	return status == 0 || status >= 500 || status == http.StatusTooManyRequests
}

// call performs one gateway request, records it, and reports whether it counts
// as a journey failure.
func (r *Runner) call(ctx context.Context, journey string, req gateway.Request) (int, error) {
	status, err := r.gw.Do(ctx, req)
	if r.rec != nil {
		r.rec.RecordRequest(status)
	}
	r.metrics.ObserveRequest(journey, stats.StatusLabel(status))
	if err != nil {
		return status, err
	}
	if failed(status) {
		return status, fmt.Errorf("%s %s: status %d", req.Method, req.Path, status)
	}
	return status, nil
}

// authed performs a request with the customer's bearer token, refreshing the
// token exactly once if the gateway answers 401.
func (r *Runner) authed(ctx context.Context, journey string, cust *customer.Customer, req gateway.Request) (int, error) {
	token, err := cust.Token(ctx, r.now)
	if err != nil {
		// Report the failed login attempt itself so the console shows it.
		if r.rec != nil {
			r.rec.RecordRequest(0)
		}
		r.metrics.ObserveRequest(journey, stats.StatusLabel(0))
		return 0, err
	}

	req.Token = token
	status, err := r.call(ctx, journey, req)
	if status != http.StatusUnauthorized {
		return status, err
	}

	// Token refresh on 401: drop it, log in again, retry once.
	cust.Invalidate(token)
	fresh, ferr := cust.Token(ctx, r.now)
	if ferr != nil {
		return status, ferr
	}
	req.Token = fresh
	return r.call(ctx, journey, req)
}

func (r *Runner) browse(ctx context.Context, journey string, cust *customer.Customer) error {
	_, err := r.authed(ctx, journey, cust, gateway.Request{
		Method: http.MethodGet,
		Path:   "/api/v1/parts",
	})
	return err
}

func (r *Runner) quote(ctx context.Context, journey string, cust *customer.Customer) error {
	_, err := r.authed(ctx, journey, cust, gateway.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/quotes",
		Body:   RandomSpec(r.source),
	})
	return err
}

func (r *Runner) order(ctx context.Context, journey string, cust *customer.Customer) error {
	_, err := r.authed(ctx, journey, cust, gateway.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/orders",
		Body:   RandomSpec(r.source),
	})
	return err
}

// orderRef is the only field of an order the track journey needs.
type orderRef struct {
	ID string `json:"id"`
}

// orderList tolerates either a bare array or an envelope, because the ledger's
// list shape is the datastore's business, not ours.
type orderList struct {
	orders []orderRef
}

func (l *orderList) UnmarshalJSON(data []byte) error {
	var bare []orderRef
	if err := json.Unmarshal(data, &bare); err == nil {
		l.orders = bare
		return nil
	}
	var envelope struct {
		Orders []orderRef `json:"orders"`
		Items  []orderRef `json:"items"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	l.orders = envelope.Orders
	if l.orders == nil {
		l.orders = envelope.Items
	}
	return nil
}

func (r *Runner) track(ctx context.Context, journey string, cust *customer.Customer) error {
	var list orderList
	if _, err := r.authed(ctx, journey, cust, gateway.Request{
		Method: http.MethodGet,
		Path:   "/api/v1/orders",
		Query:  url.Values{"limit": []string{"5"}},
		Out:    &list,
	}); err != nil {
		return err
	}
	if len(list.orders) == 0 {
		// A customer with no orders yet has nothing to track. Not a failure.
		return nil
	}

	pick := list.orders[r.source.IntN(len(list.orders))]
	if pick.ID == "" {
		return nil
	}

	if _, err := r.authed(ctx, journey, cust, gateway.Request{
		Method: http.MethodGet,
		Path:   "/api/v1/orders/" + url.PathEscape(pick.ID),
	}); err != nil {
		return err
	}
	_, err := r.authed(ctx, journey, cust, gateway.Request{
		Method: http.MethodGet,
		Path:   "/api/v1/shipments/" + url.PathEscape(pick.ID),
	})
	return err
}
