package events_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/events"
)

func TestNewEnvelope(t *testing.T) {
	type payload struct {
		OrderID string `json:"orderId"`
		Attempt int    `json:"attempt"`
	}
	env, err := events.NewEnvelope("panametric-fam-processor", events.SubjectFabricationRequested,
		payload{OrderID: "ord_1", Attempt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(env.ID, "evt_") {
		t.Errorf("id = %q, want an evt_ prefix", env.ID)
	}
	if env.Type != events.SubjectFabricationRequested {
		t.Errorf("type = %q", env.Type)
	}
	if env.Source != "panametric-fam-processor" {
		t.Errorf("source = %q", env.Source)
	}
	if time.Since(env.Time) > time.Minute || env.Time.IsZero() {
		t.Errorf("time = %v", env.Time)
	}

	var back payload
	if err := env.Unmarshal(&back); err != nil {
		t.Fatal(err)
	}
	if back.OrderID != "ord_1" || back.Attempt != 1 {
		t.Errorf("round trip = %+v", back)
	}
}

func TestEnvelopeWireShape(t *testing.T) {
	env, err := events.NewEnvelope("svc", events.SubjectOrderAccepted, map[string]string{"orderId": "ord_1"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "type", "time", "source", "data"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("envelope is missing %q: %s", key, raw)
		}
	}
	if len(generic) != 5 {
		t.Errorf("envelope has unexpected fields: %s", raw)
	}
}

func TestEventIDsAreUniqueAndOrdered(t *testing.T) {
	prev := ""
	for range 50 {
		id := events.NewEventID()
		if id == prev {
			t.Fatal("duplicate event id")
		}
		prev = id
	}
}

func TestStreamConfigMatchesTheDesign(t *testing.T) {
	cfg := events.StreamConfig()
	if cfg.Name != "ENCABULATOR" {
		t.Errorf("name = %q", cfg.Name)
	}
	if len(cfg.Subjects) != 1 || cfg.Subjects[0] != "encabulator.>" {
		t.Errorf("subjects = %v", cfg.Subjects)
	}
	if cfg.MaxAge != 24*time.Hour {
		t.Errorf("max age = %v", cfg.MaxAge)
	}
	if cfg.Duplicates != 2*time.Minute {
		t.Errorf("duplicate window = %v", cfg.Duplicates)
	}
	if cfg.Replicas != 1 {
		t.Errorf("replicas = %d", cfg.Replicas)
	}
}

func TestHeaderCarrier(t *testing.T) {
	h := nats.Header{}
	c := events.HeaderCarrier(h)
	c.Set("traceparent", "00-aaaa-bbbb-01")
	if got := c.Get("traceparent"); got != "00-aaaa-bbbb-01" {
		t.Errorf("Get = %q", got)
	}
	if keys := c.Keys(); len(keys) != 1 || keys[0] != "traceparent" {
		t.Errorf("Keys = %v", keys)
	}

	// A nil carrier (a message with no headers) must not panic.
	var nilCarrier events.HeaderCarrier
	if got := nilCarrier.Get("traceparent"); got != "" {
		t.Errorf("nil carrier Get = %q", got)
	}
	nilCarrier.Set("a", "b")
	if len(nilCarrier.Keys()) != 0 {
		t.Error("nil carrier grew keys")
	}
}
