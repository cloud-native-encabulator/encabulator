package influx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-recorder/internal/influx"
)

// The recorder -> influxdb edge on the console exists only because the
// recorder emits a CLIENT span per InfluxDB call carrying db.system.name: the
// collector's service_graph connector names the server after that value.
// These tests need no container: the write path is plain HTTP, so a stub
// server stands in for InfluxDB.

func newTracedClient(t *testing.T, status int) (*influx.Client, *tracetest.SpanRecorder, *atomic.Int32) {
	t.Helper()
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v3/write_lp") {
			writes.Add(1)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	c, err := influx.New(influx.Config{URL: srv.URL, Database: "sinusoidal", TracerProvider: tp})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, recorder, &writes
}

func reading() influx.Reading {
	return influx.Reading{Serial: "ENC-1", OrderID: "ord_1", Model: "turbo", Stage: "base-plate",
		Metric: "repleneration", Value: 1.5, Time: time.Now().UTC()}
}

func attrsOf(span sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	out := map[attribute.Key]attribute.Value{}
	for _, kv := range span.Attributes() {
		out[kv.Key] = kv.Value
	}
	return out
}

func TestWriteRecordsAnInfluxDBClientSpan(t *testing.T) {
	c, recorder, writes := newTracedClient(t, http.StatusNoContent)
	if err := c.Write(context.Background(), []influx.Reading{reading(), reading()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if writes.Load() == 0 {
		t.Fatal("the stub never saw a write_lp request")
	}
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.SpanKind() != trace.SpanKindClient {
		t.Errorf("kind = %v, want client", span.SpanKind())
	}
	if span.Name() != "write readings" {
		t.Errorf("name = %q", span.Name())
	}
	a := attrsOf(span)
	for key, want := range map[attribute.Key]string{
		"db.system.name":     "influxdb",
		"db.namespace":       "sinusoidal",
		"db.collection.name": "readings",
		"db.operation.name":  "write",
		"server.address":     "127.0.0.1",
	} {
		if got := a[key].AsString(); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if a["server.port"].AsInt64() == 0 {
		t.Error("server.port missing")
	}
	if a["db.operation.batch.size"].AsInt64() != 2 {
		t.Errorf("batch size = %v", a["db.operation.batch.size"])
	}
	if span.Status().Code == codes.Error {
		t.Errorf("a successful write is marked failed: %v", span.Status())
	}
}

func TestAFailedWriteMarksItsSpanFailed(t *testing.T) {
	c, recorder, _ := newTracedClient(t, http.StatusInternalServerError)
	if err := c.Write(context.Background(), []influx.Reading{reading()}); err == nil {
		t.Fatal("want an error from a 500")
	}
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("want one failed span, got %d spans", len(spans))
	}
}

func TestAnEmptyBatchMakesNoCallAndNoSpan(t *testing.T) {
	c, recorder, writes := newTracedClient(t, http.StatusNoContent)
	if err := c.Write(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 0 || len(recorder.Ended()) != 0 {
		t.Fatalf("writes=%d spans=%d, want none", writes.Load(), len(recorder.Ended()))
	}
}

func TestHealthIsNotTraced(t *testing.T) {
	c, recorder, _ := newTracedClient(t, http.StatusOK)
	if err := c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(recorder.Ended()); n != 0 {
		t.Fatalf("health produced %d spans, want 0", n)
	}
}

func TestAQueryRecordsAFailedClientSpanWhenInfluxDBIsUnreachable(t *testing.T) {
	// The Flight SQL read path needs gRPC, which the stub does not speak, so the
	// query fails. It must still leave a CLIENT span, marked failed.
	c, recorder, _ := newTracedClient(t, http.StatusNotFound)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Summary(ctx, time.Minute); err == nil {
		t.Skip("the stub unexpectedly answered a Flight query")
	}
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	a := attrsOf(spans[0])
	if spans[0].SpanKind() != trace.SpanKindClient || a["db.operation.name"].AsString() != "SELECT" ||
		a["db.system.name"].AsString() != "influxdb" || spans[0].Status().Code != codes.Error {
		t.Fatalf("query span: kind=%v attrs=%v status=%v", spans[0].SpanKind(), a, spans[0].Status())
	}
}
