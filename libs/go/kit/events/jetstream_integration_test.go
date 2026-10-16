package events_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/events"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/testsupport"
)

// startNATS brings up a real nats:2.12 server with JetStream enabled and
// returns its client URL.
func startNATS(t *testing.T) string {
	t.Helper()
	port := testsupport.StartContainer(t, testsupport.ContainerSpec{
		Image: "nats:2.12",
		Cmd:   []string{"--jetstream", "--store_dir=/tmp/jetstream"},
		Port:  4222,
		Wait:  wait.ForLog("Server is ready").WithStartupTimeout(3 * time.Minute),
	})
	return fmt.Sprintf("nats://127.0.0.1:%d", port)
}

func TestEnsureStreamIsIdempotent(t *testing.T) {
	url := startNATS(t)
	ctx := testsupport.Context(t)

	nc, js, err := events.Connect(url, "kit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	first, err := events.EnsureStream(ctx, js)
	if err != nil {
		t.Fatalf("first EnsureStream: %v", err)
	}
	info, err := first.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Config.MaxAge != events.StreamMaxAge || info.Config.Duplicates != events.DuplicateWindow {
		t.Errorf("stream config = %+v", info.Config)
	}
	if info.Config.Storage != jetstream.FileStorage {
		t.Errorf("storage = %v, want file", info.Config.Storage)
	}

	// A second call from another replica must not fail or change anything.
	if _, err := events.EnsureStream(ctx, js); err != nil {
		t.Fatalf("second EnsureStream: %v", err)
	}
}

func TestPublishConsumeRoundTripWithTraceContext(t *testing.T) {
	url := startNATS(t)
	ctx := testsupport.Context(t)

	// Record spans so we can assert the producer/consumer linkage the console's
	// service graph depends on.
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	nc, js, err := events.Connect(url, "kit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	stream, err := events.EnsureStream(ctx, js)
	if err != nil {
		t.Fatal(err)
	}

	type stagePayload struct {
		OrderID string `json:"orderId"`
		Stage   string `json:"stage"`
	}

	received := make(chan events.Envelope, 4)
	consumeCtx, err := events.Consume(ctx, stream, events.ConsumerConfig{
		Durable:       "kit-test-consumer",
		Filter:        events.SubjectFabricationStage,
		MaxAckPending: 8,
	}, func(ctx context.Context, env events.Envelope, _ jetstream.Msg) error {
		if sc := trace.SpanContextFromContext(ctx); !sc.IsValid() {
			t.Error("consumer handler ran without a span context")
		}
		received <- env
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer consumeCtx.Stop()

	pub := events.NewPublisher(js, "logarithmic-casing-line")
	rootCtx, rootSpan := tp.Tracer("test").Start(ctx, "fabricate")
	sent, err := pub.Publish(rootCtx, events.SubjectFabricationStage,
		stagePayload{OrderID: "ord_1", Stage: "prelaminate-base-plate"})
	rootSpan.End()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		if got.ID != sent.ID {
			t.Errorf("event id = %q, want %q", got.ID, sent.ID)
		}
		var payload stagePayload
		if err := got.Unmarshal(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Stage != "prelaminate-base-plate" {
			t.Errorf("payload = %+v", payload)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("no event delivered")
	}

	// Give the consumer span time to end, then check kinds and linkage.
	time.Sleep(200 * time.Millisecond)
	var producer, consumer *tracetest.SpanStub
	for _, s := range exporter.GetSpans() {
		switch s.SpanKind {
		case trace.SpanKindProducer:
			producer = &s
		case trace.SpanKindConsumer:
			consumer = &s
		}
	}
	if producer == nil {
		t.Fatal("no PRODUCER span recorded")
	}
	if consumer == nil {
		t.Fatal("no CONSUMER span recorded")
	}
	if consumer.Parent.SpanID() != producer.SpanContext.SpanID() {
		t.Errorf("consumer span is not a child of the producer span (%s vs %s)",
			consumer.Parent.SpanID(), producer.SpanContext.SpanID())
	}
	if consumer.SpanContext.TraceID() != producer.SpanContext.TraceID() {
		t.Error("trace context did not propagate over NATS headers")
	}
	assertAttr(t, producer, "messaging.system", "nats")
	assertAttr(t, producer, "messaging.destination.name", events.SubjectFabricationStage)
	assertAttr(t, consumer, "messaging.system", "nats")
}

func TestPublishDeduplicatesOnMsgID(t *testing.T) {
	url := startNATS(t)
	ctx := testsupport.Context(t)

	nc, js, err := events.Connect(url, "kit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	stream, err := events.EnsureStream(ctx, js)
	if err != nil {
		t.Fatal(err)
	}

	pub := events.NewPublisher(js, "kit-test")
	env, err := events.NewEnvelope("kit-test", events.SubjectOrderAccepted, map[string]string{"orderId": "ord_dup"})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := pub.PublishEnvelope(ctx, events.SubjectOrderAccepted, env); err != nil {
			t.Fatal(err)
		}
	}

	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Errorf("stream holds %d messages; Nats-Msg-Id should have deduplicated to 1", info.State.Msgs)
	}
}

func assertAttr(t *testing.T, span *tracetest.SpanStub, key, want string) {
	t.Helper()
	for _, a := range span.Attributes {
		if string(a.Key) == key {
			if a.Value.AsString() != want {
				t.Errorf("span %s attribute %s = %q, want %q", span.Name, key, a.Value.AsString(), want)
			}
			return
		}
	}
	t.Errorf("span %s is missing attribute %s", span.Name, key)
}
