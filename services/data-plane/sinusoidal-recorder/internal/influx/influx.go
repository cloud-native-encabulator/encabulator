// Package influx is the recorder's InfluxDB 3 Core client: batch writes of unit
// telemetry and parameterized SQL reads back out of it.
package influx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Table is the InfluxDB table (measurement) every reading lands in.
const Table = "readings"

// tracerName names the instrumentation scope of the InfluxDB client spans.
const tracerName = "github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-recorder/internal/influx"

// DBSystem is the db.system.name on every InfluxDB client span. The collector's
// service_graph connector names the server of a database edge after it, which is
// the console's `influxdb` node.
const DBSystem = "influxdb"

// Limits on the read path.
const (
	// DefaultReadLimit bounds GET /v1/readings when the caller does not say.
	DefaultReadLimit = 5000
	// MaxReadLimit is the ceiling for a single read.
	MaxReadLimit = 50000
	// MaxBatch is the largest batch POST /v1/readings accepts.
	MaxBatch = 1000
)

// placeholderToken stands in when INFLUXDB_TOKEN is empty. InfluxDB 3 Core
// started with --without-auth accepts any token (or none), but the Go client
// refuses to construct with an empty one, so we hand it something harmless.
const placeholderToken = "no-auth"

// Reading is one telemetry sample: one metric, on one unit, during one stage.
type Reading struct {
	Serial  string    `json:"serial"`
	OrderID string    `json:"orderId"`
	Model   string    `json:"model"`
	Stage   string    `json:"stage"`
	Metric  string    `json:"metric"`
	Value   float64   `json:"value"`
	Time    time.Time `json:"time"`
}

// Validate checks one reading. The recorder is the only writer of this table, so
// it refuses anything that would make the data unreadable later.
func (r Reading) Validate() error {
	switch {
	case strings.TrimSpace(r.Serial) == "":
		return errors.New("serial is required")
	case strings.TrimSpace(r.Metric) == "":
		return errors.New("metric is required")
	case strings.TrimSpace(r.Stage) == "":
		return errors.New("stage is required")
	case math.IsNaN(r.Value) || math.IsInf(r.Value, 0):
		return errors.New("value must be a finite number")
	case len(r.Serial) > 128 || len(r.Metric) > 64 || len(r.Stage) > 64:
		return errors.New("serial, metric and stage must be short identifiers")
	}
	return nil
}

// MetricSummary is one row of GET /v1/summary.
type MetricSummary struct {
	Metric string  `json:"metric"`
	Count  int64   `json:"count"`
	Mean   float64 `json:"mean"`
	P95    float64 `json:"p95"`
}

// Config configures the client.
type Config struct {
	// URL is INFLUXDB_URL, for example http://influxdb:8181.
	URL string
	// Token is INFLUXDB_TOKEN. It may be empty when the server runs without auth.
	Token string
	// Database is INFLUXDB_DATABASE, default "sinusoidal".
	Database string
	// Logger is optional.
	Logger *slog.Logger
	// HTTPClient overrides the client used for the health and admin endpoints.
	HTTPClient *http.Client
	// TracerProvider records the InfluxDB client spans. Defaults to the global
	// provider, which kit.Bootstrap configures.
	TracerProvider trace.TracerProvider
}

// Client talks to InfluxDB 3 Core. Writes use the HTTP line-protocol API and
// reads use Flight SQL; both are served on the same port.
type Client struct {
	influx   *influxdb3.Client
	http     *http.Client
	baseURL  string
	token    string
	database string
	log      *slog.Logger
	tracer   trace.Tracer
	// server.address and server.port of the InfluxDB endpoint, for the spans.
	host string
	port int
}

// New builds a client. It does not touch the network.
func New(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, errors.New("influx: INFLUXDB_URL is required")
	}
	database := cfg.Database
	if database == "" {
		database = "sinusoidal"
	}
	token := cfg.Token
	if token == "" {
		token = placeholderToken
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	base := strings.TrimRight(cfg.URL, "/")
	client, err := influxdb3.New(influxdb3.ClientConfig{
		Host:     base,
		Token:    token,
		Database: database,
		// An http:// host means plaintext gRPC for the Flight query path.
		WriteOptions: &influxdb3.WriteOptions{
			Precision:     influxdb3.Nanosecond,
			GzipThreshold: 1000,
			AcceptPartial: true,
			// The v3 endpoint is the native one for InfluxDB 3 Core.
			UseV2Api: false,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("influx: client: %w", err)
	}

	tp := cfg.TracerProvider
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	host, port := hostPort(base)

	return &Client{
		influx:   client,
		http:     httpClient,
		baseURL:  base,
		token:    cfg.Token,
		database: database,
		log:      log,
		tracer:   tp.Tracer(tracerName),
		host:     host,
		port:     port,
	}, nil
}

// hostPort splits the InfluxDB URL into server.address and server.port.
func hostPort(base string) (string, int) {
	u, err := url.Parse(base)
	if err != nil {
		return "", 0
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		switch u.Scheme {
		case "https":
			port = 443
		case "http":
			port = 80
		}
	}
	return u.Hostname(), port
}

// startSpan opens the CLIENT span for one InfluxDB call. Nothing instruments
// InfluxDB itself, so this span is the only evidence of the recorder -> influxdb
// edge: the collector's service_graph connector turns a client span carrying
// db.system.name into a database edge named after it. The health check is not
// traced (design section 7: no spans for readiness).
func (c *Client) startSpan(ctx context.Context, operation string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("db.system.name", DBSystem),
		attribute.String("db.namespace", c.database),
		attribute.String("db.collection.name", Table),
		attribute.String("db.operation.name", operation),
	}
	if c.host != "" {
		attrs = append(attrs, attribute.String("server.address", c.host))
	}
	if c.port > 0 {
		attrs = append(attrs, attribute.Int("server.port", c.port))
	}
	return c.tracer.Start(ctx, operation+" "+Table,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// endSpan records err, if any, as the span's outcome and ends it.
func endSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// Database returns the configured database name.
func (c *Client) Database() string { return c.database }

// Close releases the HTTP pool and the Flight connection.
func (c *Client) Close() error { return c.influx.Close() }

// Health is the readiness check: GET /health must answer 200. The request
// carries the token when there is one, because InfluxDB 3 Core authenticates
// /health unless it was started with --without-auth.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("influx: health: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("influx: health returned %d", resp.StatusCode)
	}
	return nil
}

// EnsureDatabase creates the database if it is missing. Writes would create it
// implicitly, but creating it up front means a read before the first write
// answers with an empty result instead of an error.
func (c *Client) EnsureDatabase(ctx context.Context) error {
	body := strings.NewReader(`{"db":` + strconv.Quote(c.database) + `}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v3/configure/database", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("influx: create database: %w", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))

	switch {
	case resp.StatusCode < 300:
		c.log.InfoContext(ctx, "telemetry database ready", slog.String("database", c.database))
		return nil
	case resp.StatusCode == http.StatusConflict,
		resp.StatusCode == http.StatusBadRequest && strings.Contains(strings.ToLower(string(payload)), "already exists"):
		// Another replica got there first.
		return nil
	default:
		return fmt.Errorf("influx: create database %q returned %d: %s",
			c.database, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

// EnsureTable creates the readings table with its tag and field columns. Writes
// would create it implicitly, but a query against a database whose table does
// not exist yet is an error rather than an empty result, and the console asks for
// a summary before the first unit is ever fabricated.
func (c *Client) EnsureTable(ctx context.Context) error {
	body := fmt.Sprintf(`{"db":%s,"table":%s,`+
		`"tags":["serial","order_id","model","stage","metric"],`+
		`"fields":[{"name":"value","type":"float64"}]}`,
		strconv.Quote(c.database), strconv.Quote(Table))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v3/configure/table", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("influx: create table: %w", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))

	switch {
	case resp.StatusCode < 300:
		c.log.InfoContext(ctx, "telemetry table ready",
			slog.String("database", c.database), slog.String("table", Table))
		return nil
	case resp.StatusCode == http.StatusConflict,
		strings.Contains(strings.ToLower(string(payload)), "already exists"):
		return nil
	default:
		return fmt.Errorf("influx: create table %q returned %d: %s",
			Table, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

// isTableMissing reports whether err is InfluxDB saying the readings table does
// not exist yet. A fresh database is an empty result, not a server error.
func isTableMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") && strings.Contains(msg, Table)
}

func (c *Client) authorize(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// Write stores a batch of readings. Tags are serial, order_id, model, stage and
// metric; the single field is value.
func (c *Client) Write(ctx context.Context, readings []Reading) (err error) {
	if len(readings) == 0 {
		return nil
	}
	ctx, span := c.startSpan(ctx, "write")
	span.SetAttributes(attribute.Int("db.operation.batch.size", len(readings)))
	defer func() { endSpan(span, err) }()

	points := make([]*influxdb3.Point, 0, len(readings))
	for _, r := range readings {
		at := r.Time
		if at.IsZero() {
			at = time.Now().UTC()
		}
		points = append(points, influxdb3.NewPointWithMeasurement(Table).
			SetTag("serial", r.Serial).
			SetTag("order_id", r.OrderID).
			SetTag("model", r.Model).
			SetTag("stage", r.Stage).
			SetTag("metric", r.Metric).
			SetDoubleField("value", r.Value).
			SetTimestamp(at))
	}
	if err := c.influx.WritePoints(ctx, points); err != nil {
		return fmt.Errorf("influx: write %d readings: %w", len(readings), err)
	}
	return nil
}

// readingsSQL is a fixed query; only the serial is bound, as a parameter.
const readingsSQL = `
SELECT time, serial, order_id, model, stage, metric, value
  FROM ` + Table + `
 WHERE serial = $serial
 ORDER BY time
 LIMIT `

// ReadingsBySerial returns every reading for one unit, oldest first.
//
// The serial is bound as a query parameter and never interpolated. The limit is
// interpolated, but only after being clamped to an integer in a fixed range, so
// no caller-supplied text reaches the query.
func (c *Client) ReadingsBySerial(ctx context.Context, serial string, limit int) (_ []Reading, err error) {
	if limit <= 0 {
		limit = DefaultReadLimit
	}
	if limit > MaxReadLimit {
		limit = MaxReadLimit
	}
	ctx, span := c.startSpan(ctx, "SELECT")
	defer func() { endSpan(span, err) }()

	it, err := c.influx.QueryWithParameters(ctx,
		readingsSQL+strconv.Itoa(limit),
		influxdb3.QueryParameters{"serial": serial},
	)
	if isTableMissing(err) {
		return []Reading{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("influx: query readings: %w", err)
	}
	defer it.Raw().Release()

	out := make([]Reading, 0, 128)
	for it.Next() {
		row := it.Value()
		out = append(out, Reading{
			Serial:  stringOf(row["serial"]),
			OrderID: stringOf(row["order_id"]),
			Model:   stringOf(row["model"]),
			Stage:   stringOf(row["stage"]),
			Metric:  stringOf(row["metric"]),
			Value:   floatOf(row["value"]),
			Time:    timeOf(row["time"]),
		})
	}
	if err := it.Err(); err != nil {
		return nil, fmt.Errorf("influx: read readings: %w", err)
	}
	return out, nil
}

// summarySQL aggregates every metric over a window. The window lower bound is a
// bound parameter, cast to a timestamp so DataFusion compares it to the time
// column rather than to a string.
const summarySQL = `
SELECT metric,
       count(value)                            AS sample_count,
       avg(value)                              AS mean_value,
       approx_percentile_cont(value, 0.95)     AS p95_value
  FROM ` + Table + `
 WHERE time >= CAST($since AS TIMESTAMP)
 GROUP BY metric
 ORDER BY metric`

// Summary returns per-metric aggregates across the whole factory over window.
func (c *Client) Summary(ctx context.Context, window time.Duration) (_ []MetricSummary, err error) {
	since := time.Now().UTC().Add(-window)
	ctx, span := c.startSpan(ctx, "SELECT")
	defer func() { endSpan(span, err) }()

	it, err := c.influx.QueryWithParameters(ctx, summarySQL,
		influxdb3.QueryParameters{"since": since.Format("2006-01-02 15:04:05.000000")},
	)
	if isTableMissing(err) {
		return []MetricSummary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("influx: query summary: %w", err)
	}
	defer it.Raw().Release()

	out := make([]MetricSummary, 0, 4)
	for it.Next() {
		row := it.Value()
		out = append(out, MetricSummary{
			Metric: stringOf(row["metric"]),
			Count:  intOf(row["sample_count"]),
			Mean:   floatOf(row["mean_value"]),
			P95:    floatOf(row["p95_value"]),
		})
	}
	if err := it.Err(); err != nil {
		return nil, fmt.Errorf("influx: read summary: %w", err)
	}
	return out, nil
}

// ParseWindow reads a ?window= value. It accepts Go durations from one second to
// 24 hours; anything else is an error rather than a silent default.
func ParseWindow(raw string, def time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("window must be a duration such as 5m: %w", err)
	}
	if d < time.Second || d > 24*time.Hour {
		return 0, errors.New("window must be between 1s and 24h")
	}
	return d, nil
}

// The Flight result columns arrive as Arrow values; these narrow them without
// panicking on an unexpected type.

func stringOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func floatOf(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int64:
		return float64(t)
	case uint64:
		return float64(t)
	default:
		return 0
	}
}

func intOf(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case uint64:
		return int64(t)
	case float64:
		return int64(t)
	default:
		return 0
	}
}

func timeOf(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t.UTC()
	default:
		return time.Time{}
	}
}
