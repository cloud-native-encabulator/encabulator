// Package events is the NATS JetStream side of the kit: the one ENCABULATOR
// stream, the event envelope, and publish/consume helpers that carry trace
// context and produce the PRODUCER and CONSUMER spans the console's service
// graph is built from. See design sections 5 and 7.
package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Stream configuration fixed by design section 5.
const (
	StreamName      = "ENCABULATOR"
	StreamSubjects  = "encabulator.>"
	StreamMaxAge    = 24 * time.Hour
	StreamReplicas  = 1
	DuplicateWindow = 2 * time.Minute
)

// Consumer defaults fixed by design section 5.
const (
	AckWait    = 30 * time.Second
	MaxDeliver = 5
)

// Subjects published on the stream.
const (
	SubjectOrderAccepted        = "encabulator.order.accepted"
	SubjectFabricationRequested = "encabulator.fabrication.requested"
	SubjectFabricationStage     = "encabulator.fabrication.stage"
	SubjectFabricationCompleted = "encabulator.fabrication.completed"
	SubjectInspectionPassed     = "encabulator.inspection.passed"
	SubjectInspectionFailed     = "encabulator.inspection.failed"
	SubjectOrderScrapped        = "encabulator.order.scrapped"
	SubjectShipmentDispatched   = "encabulator.shipment.dispatched"
	SubjectShipmentDelivered    = "encabulator.shipment.delivered"
)

// Envelope is the JSON body of every event.
type Envelope struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Time   time.Time       `json:"time"`
	Source string          `json:"source"`
	Data   json.RawMessage `json:"data"`
}

// NewEnvelope builds an envelope for data, assigning an evt_-prefixed UUIDv7 id
// and the current time. The event type is the subject.
func NewEnvelope(source, subject string, data any) (Envelope, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		ID:     NewEventID(),
		Type:   subject,
		Time:   time.Now().UTC().Truncate(time.Millisecond),
		Source: source,
		Data:   payload,
	}, nil
}

// NewEventID returns an evt_-prefixed, time-ordered event id.
func NewEventID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 only fails if the entropy source fails; a random v4 is a fine
		// fallback for an id whose only job is deduplication.
		id = uuid.New()
	}
	return "evt_" + id.String()
}

// Unmarshal decodes the envelope's data into v.
func (e Envelope) Unmarshal(v any) error { return json.Unmarshal(e.Data, v) }
