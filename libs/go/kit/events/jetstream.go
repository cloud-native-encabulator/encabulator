package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/jlbutler/cloud-native-encabulator/libs/go/kit/events"

// Connect dials NATS with the conventions the design expects: named connection,
// infinite reconnect, and a modest reconnect wait.
func Connect(url, name string) (*nats.Conn, jetstream.JetStream, error) {
	if url == "" {
		url = nats.DefaultURL
	}
	nc, err := nats.Connect(url,
		nats.Name(name),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("events: connect %s: %w", url, err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("events: jetstream: %w", err)
	}
	return nc, js, nil
}

// StreamConfig is the ENCABULATOR stream configuration. Every service that
// publishes or consumes creates the stream with exactly this config and never
// updates an existing one.
func StreamConfig() jetstream.StreamConfig {
	return jetstream.StreamConfig{
		Name:       StreamName,
		Subjects:   []string{StreamSubjects},
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		MaxAge:     StreamMaxAge,
		Replicas:   StreamReplicas,
		Duplicates: DuplicateWindow,
	}
}

// EnsureStream creates the ENCABULATOR stream if it is missing, and otherwise
// returns the existing one untouched.
func EnsureStream(ctx context.Context, js jetstream.JetStream) (jetstream.Stream, error) {
	stream, err := js.CreateStream(ctx, StreamConfig())
	if err == nil {
		return stream, nil
	}
	if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		existing, getErr := js.Stream(ctx, StreamName)
		if getErr != nil {
			return nil, fmt.Errorf("events: stream %s exists but is unreadable: %w", StreamName, getErr)
		}
		return existing, nil
	}
	return nil, fmt.Errorf("events: create stream %s: %w", StreamName, err)
}

// Publisher publishes envelopes on the stream.
type Publisher struct {
	js     jetstream.JetStream
	source string
	tracer trace.Tracer
}

// NewPublisher returns a Publisher that stamps source on every envelope.
func NewPublisher(js jetstream.JetStream, source string) *Publisher {
	return &Publisher{js: js, source: source, tracer: otel.Tracer(tracerName)}
}

// Publish marshals data into an envelope and publishes it on subject. It sets
// Nats-Msg-Id to the event id for deduplication, injects W3C trace context into
// the message headers, and records a PRODUCER span.
func (p *Publisher) Publish(ctx context.Context, subject string, data any) (Envelope, error) {
	env, err := NewEnvelope(p.source, subject, data)
	if err != nil {
		return Envelope{}, fmt.Errorf("events: encode %s: %w", subject, err)
	}
	return env, p.PublishEnvelope(ctx, subject, env)
}

// PublishEnvelope publishes a pre-built envelope, for callers that need to
// control the id or time.
func (p *Publisher) PublishEnvelope(ctx context.Context, subject string, env Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("events: encode envelope %s: %w", subject, err)
	}

	ctx, span := p.tracer.Start(ctx, "publish "+subject,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.String("messaging.operation", "publish"),
			attribute.String("messaging.message.id", env.ID),
			attribute.String("encabulator.event.type", env.Type),
		),
	)
	defer span.End()

	msg := &nats.Msg{Subject: subject, Data: body, Header: nats.Header{}}
	otel.GetTextMapPropagator().Inject(ctx, HeaderCarrier(msg.Header))

	if _, err := p.js.PublishMsg(ctx, msg, jetstream.WithMsgID(env.ID)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("events: publish %s: %w", subject, err)
	}
	return nil
}

// Handler processes one event. Returning nil acks the message; returning an
// error leaves the message for the caller-configured redelivery (Consume naks).
type Handler func(ctx context.Context, env Envelope, msg jetstream.Msg) error

// ConsumerConfig describes a durable pull consumer.
type ConsumerConfig struct {
	// Durable is the durable name, shared across a service's replicas.
	Durable string
	// Filter is the subject filter, for example encabulator.fabrication.requested.
	Filter string
	// MaxAckPending bounds in-flight messages. Zero uses the server default.
	MaxAckPending int
	// Logger is optional.
	Logger *slog.Logger
}

// Consume creates (or updates) the durable consumer and starts delivering to
// handler. Each message runs inside a CONSUMER span that continues the
// publisher's trace. The returned ConsumeContext must be stopped by the caller.
func Consume(ctx context.Context, stream jetstream.Stream, cfg ConsumerConfig, handler Handler) (jetstream.ConsumeContext, error) {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       cfg.Durable,
		Name:          cfg.Durable,
		FilterSubject: cfg.Filter,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       AckWait,
		MaxDeliver:    MaxDeliver,
		MaxAckPending: cfg.MaxAckPending,
	})
	if err != nil {
		return nil, fmt.Errorf("events: consumer %s: %w", cfg.Durable, err)
	}

	tracer := otel.Tracer(tracerName)
	return cons.Consume(func(msg jetstream.Msg) {
		var env Envelope
		if err := json.Unmarshal(msg.Data(), &env); err != nil {
			log.Error("undecodable event; terminating it rather than looping",
				slog.String("subject", msg.Subject()), slog.Any("error", err))
			_ = msg.Term()
			return
		}

		msgCtx := otel.GetTextMapPropagator().Extract(ctx, HeaderCarrier(msg.Headers()))
		msgCtx, span := tracer.Start(msgCtx, "process "+msg.Subject(),
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("messaging.system", "nats"),
				attribute.String("messaging.destination.name", msg.Subject()),
				attribute.String("messaging.operation", "process"),
				attribute.String("messaging.message.id", env.ID),
				attribute.String("encabulator.event.type", env.Type),
			),
		)
		defer span.End()

		if err := handler(msgCtx, env, msg); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			log.ErrorContext(msgCtx, "event handler failed; naking for redelivery",
				slog.String("subject", msg.Subject()),
				slog.String("event_id", env.ID),
				slog.Any("error", err))
			_ = msg.Nak()
			return
		}
		if err := msg.Ack(); err != nil {
			log.ErrorContext(msgCtx, "ack failed", slog.String("event_id", env.ID), slog.Any("error", err))
		}
	})
}

// HeaderCarrier adapts nats.Header to the OTel propagation carrier interface.
type HeaderCarrier nats.Header

var _ propagation.TextMapCarrier = HeaderCarrier{}

func (c HeaderCarrier) Get(key string) string {
	if c == nil {
		return ""
	}
	return nats.Header(c).Get(key)
}

func (c HeaderCarrier) Set(key, value string) {
	if c == nil {
		return
	}
	nats.Header(c).Set(key, value)
}

func (c HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
