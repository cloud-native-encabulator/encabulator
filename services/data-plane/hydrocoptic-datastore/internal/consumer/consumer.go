// Package consumer folds every event on the ENCABULATOR stream into the ledger.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/events"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/malaise"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/ledger"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/store"
)

// Durable is the consumer name shared by every replica.
const Durable = "hydrocoptic-datastore"

// Applier is the ledger write the consumer needs.
type Applier interface {
	Apply(ctx context.Context, eventID, subject string, eventTime time.Time, data json.RawMessage) (store.ApplyResult, error)
}

// Consumer folds events into the ledger.
type Consumer struct {
	store   Applier
	log     *slog.Logger
	malaise *malaise.Injector
	applied *prometheus.CounterVec
}

// New builds a consumer and registers its metric.
func New(s Applier, log *slog.Logger, inj *malaise.Injector, reg prometheus.Registerer) *Consumer {
	return &Consumer{
		store:   s,
		log:     log,
		malaise: inj,
		applied: httpx.RegisterCounterVec(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "encabulator_ledger_events_applied_total",
			Help: "Stream events folded into the ledger, by event type and outcome.",
		}, []string{"type", "result"})),
	}
}

// Start creates the durable consumer on encabulator.> and begins delivering.
// The returned ConsumeContext must be stopped by the caller.
func (c *Consumer) Start(ctx context.Context, stream jetstream.Stream) (jetstream.ConsumeContext, error) {
	return events.Consume(ctx, stream, events.ConsumerConfig{
		Durable: Durable,
		Filter:  events.StreamSubjects,
		Logger:  c.log,
	}, func(ctx context.Context, env events.Envelope, _ jetstream.Msg) error {
		return c.Handle(ctx, env)
	})
}

// Handle folds one event into the ledger under any injected malaise. It is
// exported so tests can drive it without a broker.
func (c *Consumer) Handle(ctx context.Context, env events.Envelope) error {
	return c.malaise.Apply(ctx, func(ctx context.Context) error {
		return c.apply(ctx, env)
	})
}

func (c *Consumer) apply(ctx context.Context, env events.Envelope) error {
	subject := env.Type
	result, err := c.store.Apply(ctx, env.ID, subject, env.Time, env.Data)
	if err != nil {
		var notForLedger ledger.ErrNotForLedger
		if errors.As(err, &notForLedger) {
			// An event the ledger has no opinion about. Ack it; redelivering would
			// not change our mind.
			c.applied.WithLabelValues(subject, "ignored").Inc()
			return nil
		}
		c.applied.WithLabelValues(subject, "error").Inc()
		return fmt.Errorf("consumer: apply %s: %w", subject, err)
	}

	c.applied.WithLabelValues(subject, string(result)).Inc()
	switch result {
	case store.Orphan:
		// The order has not been written yet. Nak so JetStream brings the event
		// back once the processor's POST has landed.
		return fmt.Errorf("consumer: %s arrived before its order existed", subject)
	case store.Applied:
		c.log.DebugContext(ctx, "ledger updated",
			slog.String("event_id", env.ID), slog.String("type", subject))
	}
	return nil
}
