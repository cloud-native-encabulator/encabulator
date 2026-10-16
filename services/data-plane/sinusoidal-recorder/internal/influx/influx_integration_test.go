package influx_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/wait"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/testsupport"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/sinusoidal-recorder/internal/influx"
)

// newClient brings up a real influxdb:3-core with auth disabled and an
// in-memory object store, then returns a client pointed at it.
func newClient(t *testing.T) *influx.Client {
	t.Helper()
	return newClientWith(t, nil)
}

// newClientWith is newClient with the client spans going to tp (nil: the global provider).
func newClientWith(t *testing.T, tp trace.TracerProvider) *influx.Client {
	t.Helper()
	c, _ := newClientWithServerArgs(t, tp)
	return c
}

// newClientWithServerArgs is newClientWith with extra `influxdb3 serve` arguments. It also
// returns the server's base URL.
func newClientWithServerArgs(t *testing.T, tp trace.TracerProvider, serverArgs ...string) (*influx.Client, string) {
	t.Helper()
	port := testsupport.StartContainer(t, testsupport.ContainerSpec{
		Image: "influxdb:3-core",
		Cmd: append([]string{
			"influxdb3", "serve",
			"--node-id=recorder-test",
			"--object-store=memory",
			"--without-auth",
			"--http-bind=0.0.0.0:8181",
		}, serverArgs...),
		Port: 8181,
		// The server logs its startup time on the line after it binds.
		Wait: wait.ForHTTP("/health").WithPort("8181/tcp").WithStartupTimeout(3 * time.Minute),
	})
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	// The token is deliberately empty: --without-auth accepts no credentials, and
	// the recorder must cope with INFLUXDB_TOKEN being unset.
	c, err := influx.New(influx.Config{
		URL:            base,
		Token:          "",
		Database:       "sinusoidal",
		TracerProvider: tp,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Logf("close: %v", err)
		}
	})

	ctx := testsupport.Context(t)
	if err := c.Health(ctx); err != nil {
		t.Fatalf("health: %v", err)
	}
	if err := c.EnsureDatabase(ctx); err != nil {
		t.Fatalf("ensure database: %v", err)
	}
	if err := c.EnsureTable(ctx); err != nil {
		t.Fatalf("ensure table: %v", err)
	}
	return c, base
}

func TestHealthAndEnsureDatabaseAreIdempotent(t *testing.T) {
	c := newClient(t)
	ctx := testsupport.Context(t)

	if err := c.Health(ctx); err != nil {
		t.Fatalf("health: %v", err)
	}
	// A second replica must not fail on an existing database or table.
	if err := c.EnsureDatabase(ctx); err != nil {
		t.Fatalf("second EnsureDatabase: %v", err)
	}
	if err := c.EnsureTable(ctx); err != nil {
		t.Fatalf("second EnsureTable: %v", err)
	}
	if c.Database() != "sinusoidal" {
		t.Errorf("database = %q", c.Database())
	}
}

func TestWriteAndReadBySerial(t *testing.T) {
	c := newClient(t)
	ctx := testsupport.Context(t)

	base := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	readings := make([]influx.Reading, 0, 60)
	for i := range 20 {
		at := base.Add(time.Duration(i) * 10 * time.Millisecond)
		readings = append(readings,
			influx.Reading{Serial: "ENC-turbo-a", OrderID: "ord_1", Model: "turbo",
				Stage: "fit-marzlevanes", Metric: "repleneration",
				Value: 1.0 + 0.01*float64(i), Time: at},
			influx.Reading{Serial: "ENC-turbo-a", OrderID: "ord_1", Model: "turbo",
				Stage: "fit-marzlevanes", Metric: "marzlevane_alignment",
				Value: 0.2 * float64(i%3), Time: at},
			influx.Reading{Serial: "ENC-turbo-b", OrderID: "ord_1", Model: "turbo",
				Stage: "fit-marzlevanes", Metric: "grammeter_drift",
				Value: float64(i) * 0.5, Time: at},
		)
	}
	if err := c.Write(ctx, readings); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := waitForReadings(t, c, "ENC-turbo-a", 40)
	for i := 1; i < len(got); i++ {
		if got[i].Time.Before(got[i-1].Time) {
			t.Fatal("readings are not ordered by time")
		}
	}
	for _, r := range got {
		if r.Serial != "ENC-turbo-a" {
			t.Fatalf("serial filter leaked %q", r.Serial)
		}
		if r.OrderID != "ord_1" || r.Model != "turbo" || r.Stage != "fit-marzlevanes" {
			t.Errorf("tags lost: %+v", r)
		}
		if r.Metric != "repleneration" && r.Metric != "marzlevane_alignment" {
			t.Errorf("unexpected metric %q on this serial", r.Metric)
		}
		if r.Time.IsZero() {
			t.Error("time column did not decode to a time.Time")
		}
	}

	// The other serial is separate, and an unknown serial is empty, not an error.
	other := waitForReadings(t, c, "ENC-turbo-b", 20)
	if len(other) != 20 {
		t.Errorf("second serial has %d readings", len(other))
	}
	empty, err := c.ReadingsBySerial(ctx, "ENC-does-not-exist", 0)
	if err != nil {
		t.Fatalf("unknown serial: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("unknown serial returned %d readings", len(empty))
	}
}

func TestReadLimitIsClamped(t *testing.T) {
	c := newClient(t)
	ctx := testsupport.Context(t)

	base := time.Now().UTC().Add(-time.Minute)
	readings := make([]influx.Reading, 0, 30)
	for i := range 30 {
		readings = append(readings, influx.Reading{
			Serial: "ENC-limit", OrderID: "ord_1", Model: "turbo", Stage: "wind-lotus-o-delta",
			Metric: "repleneration", Value: 1, Time: base.Add(time.Duration(i) * time.Millisecond),
		})
	}
	if err := c.Write(ctx, readings); err != nil {
		t.Fatal(err)
	}
	waitForReadings(t, c, "ENC-limit", 30)

	got, err := c.ReadingsBySerial(ctx, "ENC-limit", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Errorf("limit 5 returned %d readings", len(got))
	}
	// A limit above the ceiling is clamped, not rejected, and still works.
	got, err = c.ReadingsBySerial(ctx, "ENC-limit", influx.MaxReadLimit*10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 30 {
		t.Errorf("clamped limit returned %d readings", len(got))
	}
}

// TestSerialIsBoundNotInterpolated pushes SQL metacharacters through the serial
// parameter. A parameterized query returns nothing; an interpolated one would
// either error or return the whole table.
func TestSerialIsBoundNotInterpolated(t *testing.T) {
	c := newClient(t)
	ctx := testsupport.Context(t)

	if err := c.Write(ctx, []influx.Reading{{
		Serial: "ENC-safe", OrderID: "ord_1", Model: "turbo", Stage: "connect-tremie-pipes",
		Metric: "repleneration", Value: 1.0, Time: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
	waitForReadings(t, c, "ENC-safe", 1)

	for _, hostile := range []string{
		`' OR 1=1 --`,
		`ENC-safe' OR '1'='1`,
		`"; DROP TABLE readings; --`,
		`ENC-safe%`,
	} {
		got, err := c.ReadingsBySerial(ctx, hostile, 0)
		if err != nil {
			t.Fatalf("hostile serial %q errored instead of matching nothing: %v", hostile, err)
		}
		if len(got) != 0 {
			t.Errorf("hostile serial %q returned %d rows", hostile, len(got))
		}
	}

	// The table survived.
	if got := waitForReadings(t, c, "ENC-safe", 1); len(got) != 1 {
		t.Errorf("table state changed: %d rows", len(got))
	}
}

func TestSummaryAggregates(t *testing.T) {
	c := newClient(t)
	ctx := testsupport.Context(t)

	now := time.Now().UTC()
	readings := make([]influx.Reading, 0, 200)
	// 100 samples of repleneration at 1.0 exactly, so mean and p95 are knowable.
	for i := range 100 {
		readings = append(readings, influx.Reading{
			Serial: fmt.Sprintf("ENC-%d", i%4), OrderID: "ord_1", Model: "turbo",
			Stage: "align-spurving-bearings", Metric: "repleneration",
			Value: 1.0, Time: now.Add(-time.Duration(i) * time.Second),
		})
	}
	// 100 samples of grammeter_drift ramping 0..99.
	for i := range 100 {
		readings = append(readings, influx.Reading{
			Serial: fmt.Sprintf("ENC-%d", i%4), OrderID: "ord_1", Model: "turbo",
			Stage: "align-spurving-bearings", Metric: "grammeter_drift",
			Value: float64(i), Time: now.Add(-time.Duration(i) * time.Second),
		})
	}
	// One ancient sample that a 5 minute window must exclude.
	readings = append(readings, influx.Reading{
		Serial: "ENC-old", OrderID: "ord_0", Model: "turbo", Stage: "prelaminate-base-plate",
		Metric: "ancient_metric", Value: 999, Time: now.Add(-2 * time.Hour),
	})
	if err := c.Write(ctx, readings); err != nil {
		t.Fatal(err)
	}

	var summary []influx.MetricSummary
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		summary, err = c.Summary(ctx, 5*time.Minute)
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		if len(summary) == 2 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if len(summary) != 2 {
		t.Fatalf("summary = %+v, want two metrics inside the window", summary)
	}

	byMetric := map[string]influx.MetricSummary{}
	for _, s := range summary {
		byMetric[s.Metric] = s
	}
	if _, ok := byMetric["ancient_metric"]; ok {
		t.Error("the window did not exclude the two-hour-old sample")
	}

	rep := byMetric["repleneration"]
	if rep.Count != 100 {
		t.Errorf("repleneration count = %d, want 100", rep.Count)
	}
	if math.Abs(rep.Mean-1.0) > 1e-9 {
		t.Errorf("repleneration mean = %v, want 1.0", rep.Mean)
	}
	if math.Abs(rep.P95-1.0) > 1e-6 {
		t.Errorf("repleneration p95 = %v, want 1.0", rep.P95)
	}

	drift := byMetric["grammeter_drift"]
	if drift.Count != 100 {
		t.Errorf("grammeter_drift count = %d, want 100", drift.Count)
	}
	if math.Abs(drift.Mean-49.5) > 0.5 {
		t.Errorf("grammeter_drift mean = %v, want about 49.5", drift.Mean)
	}
	// p95 of 0..99 is about 94; approx_percentile_cont has slack.
	if drift.P95 < 90 || drift.P95 > 99.5 {
		t.Errorf("grammeter_drift p95 = %v, want about 94", drift.P95)
	}

	// A short window excludes most of the ramp.
	short, err := c.Summary(ctx, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range short {
		if s.Count >= 100 {
			t.Errorf("a 10s window returned %d samples of %s", s.Count, s.Metric)
		}
	}
}

func TestSummaryOnAnEmptyTable(t *testing.T) {
	c := newClient(t)
	// No writes at all: an empty summary, not an error, so /v1/summary works from
	// the moment the works opens.
	got, err := c.Summary(testsupport.Context(t), 5*time.Minute)
	if err != nil {
		t.Fatalf("summary on an empty database: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("summary = %+v", got)
	}
}

func TestWriteEmptyBatchIsANoOp(t *testing.T) {
	c := newClient(t)
	if err := c.Write(testsupport.Context(t), nil); err != nil {
		t.Fatalf("empty write: %v", err)
	}
}

// Against a real InfluxDB 3 Core: a write and both reads each leave one CLIENT
// span with db.system.name=influxdb and no error, which is what draws the
// recorder -> influxdb line on the console.
func TestRealCallsLeaveInfluxDBClientSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	c := newClientWith(t, tp)
	ctx := testsupport.Context(t)

	before := len(recorder.Ended())
	r := influx.Reading{Serial: "ENC-TRACE", OrderID: "ord_t", Model: "turbo", Stage: "base-plate",
		Metric: "repleneration", Value: 2, Time: time.Now().UTC()}
	if err := c.Write(ctx, []influx.Reading{r}); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitForReadings(t, c, "ENC-TRACE", 1)
	if _, err := c.Summary(ctx, time.Hour); err != nil {
		t.Fatalf("summary: %v", err)
	}

	var write, read int
	for _, span := range recorder.Ended()[before:] {
		system := ""
		op := ""
		for _, kv := range span.Attributes() {
			switch kv.Key {
			case "db.system.name":
				system = kv.Value.AsString()
			case "db.operation.name":
				op = kv.Value.AsString()
			}
		}
		if span.SpanKind() != trace.SpanKindClient || system != "influxdb" {
			t.Errorf("span %q: kind=%v db.system.name=%q", span.Name(), span.SpanKind(), system)
		}
		if span.Status().Code.String() == "Error" {
			t.Errorf("span %q failed against a real server: %v", span.Name(), span.Status())
		}
		switch op {
		case "write":
			write++
		case "SELECT":
			read++
		}
	}
	if write != 1 || read < 2 {
		t.Fatalf("want 1 write span and at least 2 query spans, got %d and %d", write, read)
	}
}

func TestHealthFailsAgainstADeadServer(t *testing.T) {
	c, err := influx.New(influx.Config{URL: "http://127.0.0.1:1", Database: "sinusoidal"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Health(ctx); err == nil {
		t.Error("Health against a dead server returned nil")
	}
}

// TestReadOfAUnitTouchesOnlyItsOwnFiles holds InfluxDB to a query file limit of 4 and leaves
// more Parquet files than that in the table, one per minute of older data. A read with no time
// bound plans every file and fails ("exceeding the file limit"); the read of a unit is bounded by
// the time in its serial, so it touches only the newest files and succeeds.
func TestReadOfAUnitTouchesOnlyItsOwnFiles(t *testing.T) {
	c, base := newClientWithServerArgs(t, nil,
		"--gen1-duration=1m",
		"--query-file-limit=4",
		"--wal-flush-interval=10ms",
		"--wal-files-per-snapshot=1",
	)
	ctx := testsupport.Context(t)

	// Eight older units, each in its own minute 20 to 40 minutes ago: one gen1 file each.
	now := time.Now().UTC()
	for i := range 8 {
		at := now.Add(-time.Duration(20+2*i) * time.Minute)
		if err := c.Write(ctx, []influx.Reading{{
			Serial: casingLineSerial(at, i), OrderID: "ord_old", Model: "turbo", Stage: "prelaminate-base-plate",
			Metric: "repleneration", Value: 1, Time: at,
		}}); err != nil {
			t.Fatalf("write old unit %d: %v", i, err)
		}
	}
	files := waitForParquetFiles(t, base, 6)
	t.Logf("%d parquet files before the read", files)

	// A unit minted now. One reading is stamped a minute before the mint time, as after a clock
	// adjustment on the casing line; the slack keeps it.
	minted := time.Now().UTC().Truncate(time.Millisecond)
	serial := casingLineSerial(minted, 99)
	readings := []influx.Reading{{Serial: serial, OrderID: "ord_new", Model: "turbo", Stage: "fit-marzlevanes",
		Metric: "repleneration", Value: 0.9, Time: minted.Add(-time.Minute)}}
	for i := range 19 {
		readings = append(readings, influx.Reading{Serial: serial, OrderID: "ord_new", Model: "turbo",
			Stage: "fit-marzlevanes", Metric: "repleneration", Value: 1, Time: minted.Add(time.Duration(i) * 10 * time.Millisecond)})
	}
	if err := c.Write(ctx, readings); err != nil {
		t.Fatalf("write new unit: %v", err)
	}
	got := waitForReadings(t, c, serial, 20)
	if len(got) != 20 {
		t.Fatalf("read %d readings of the new unit, want 20", len(got))
	}
	if !got[0].Time.Equal(minted.Add(-time.Minute)) {
		t.Errorf("first reading at %v, want the one a minute before the mint time", got[0].Time)
	}
}

// casingLineSerial builds a serial the way the casing line mints one.
func casingLineSerial(at time.Time, n int) string {
	return fmt.Sprintf("ENC-turbo-%s-%04X", strings.ToUpper(strconv.FormatInt(at.UnixMilli(), 36)), n)
}

// waitForParquetFiles polls the server's own catalog until the readings table has at least want
// Parquet files, and returns the count.
func waitForParquetFiles(t *testing.T, base string, want int) int {
	t.Helper()
	q := url.Values{
		"db":     {"sinusoidal"},
		"format": {"json"},
		"q":      {"SELECT count(*) AS files FROM system.parquet_files WHERE table_name = 'readings'"},
	}
	deadline := time.Now().Add(90 * time.Second)
	files := 0
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/api/v3/query_sql?" + q.Encode())
		if err == nil {
			var rows []struct {
				Files int `json:"files"`
			}
			if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&rows) == nil && len(rows) == 1 {
				files = rows[0].Files
			}
			resp.Body.Close()
		}
		if files >= want {
			return files
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("only %d of %d parquet files were persisted within 90s", files, want)
	return 0
}

// waitForReadings polls until the expected number of rows is queryable. Writes
// land in the write buffer first, so a read straight after a write can lag.
func waitForReadings(t *testing.T, c *influx.Client, serial string, want int) []influx.Reading {
	t.Helper()
	ctx := testsupport.Context(t)
	deadline := time.Now().Add(30 * time.Second)
	var got []influx.Reading
	for time.Now().Before(deadline) {
		var err error
		got, err = c.ReadingsBySerial(ctx, serial, 0)
		if err != nil {
			t.Fatalf("read %s: %v", serial, err)
		}
		if len(got) >= want {
			return got
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("only %d of %d readings for %s became queryable within 30s", len(got), want, serial)
	return nil
}
