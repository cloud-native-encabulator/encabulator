// Package ledger holds the order domain: the order representation, the status
// lattice, and the rules for turning an event into a ledger update. It has no
// database or transport dependencies, so the rules are testable on their own.
package ledger

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Order statuses from design section 3.
const (
	StatusAccepted      = "accepted"
	StatusFabricating   = "fabricating"
	StatusInspecting    = "inspecting"
	StatusRefabricating = "refabricating"
	StatusDispatched    = "dispatched"
	StatusDelivered     = "delivered"
	StatusScrapped      = "scrapped"
)

// Models from design section 3.
const (
	ModelTurbo = "turbo"
	ModelRetro = "retro"
	ModelHyper = "hyper"
)

// Options are the order options.
type Options struct {
	DingleArm bool `json:"dingleArm"`
}

// Quote is the priced quote carried on an order.
type Quote struct {
	ID       string  `json:"id"`
	Total    float64 `json:"total"`
	Currency string  `json:"currency"`
}

// TimelineEntry is one line of an order's history.
type TimelineEntry struct {
	Time   time.Time `json:"time"`
	Type   string    `json:"type"`
	Detail string    `json:"detail,omitempty"`
}

// Order is the ledger's representation of an order, and the JSON every other
// service reads.
type Order struct {
	ID             string          `json:"id"`
	Customer       string          `json:"customer"`
	Model          string          `json:"model"`
	Quantity       int             `json:"quantity"`
	Options        Options         `json:"options"`
	Status         string          `json:"status"`
	Stage          string          `json:"stage,omitempty"`
	Attempt        int             `json:"attempt"`
	Quote          *Quote          `json:"quote,omitempty"`
	ReservationID  string          `json:"reservationId,omitempty"`
	Serials        []string        `json:"serials"`
	Carrier        string          `json:"carrier,omitempty"`
	TrackingNumber string          `json:"trackingNumber,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	Timeline       []TimelineEntry `json:"timeline,omitempty"`
}

// statusRank orders the lifecycle. Transitions are monotonic by rank, with the
// two documented exceptions for refabrication.
var statusRank = map[string]int{
	StatusAccepted:      1,
	StatusFabricating:   2,
	StatusRefabricating: 2,
	StatusInspecting:    3,
	StatusDispatched:    4,
	StatusDelivered:     5,
	StatusScrapped:      5,
}

// Rank returns a status's rank, or 0 for an unknown status.
func Rank(status string) int { return statusRank[status] }

// IsTerminal reports whether a status admits no further change.
func IsTerminal(status string) bool {
	return status == StatusDelivered || status == StatusScrapped
}

// AllowTransition reports whether the ledger should move from cur to next.
//
// Events arrive at least once and can arrive out of order, so the ledger only
// ever moves forward: monotonic by rank, plus the documented
// inspecting -> refabricating -> fabricating detour, and never out of a terminal
// status.
func AllowTransition(cur, next string) bool {
	if cur == "" {
		return true
	}
	if cur == next {
		return false
	}
	if _, ok := statusRank[next]; !ok {
		return false
	}
	if IsTerminal(cur) {
		return false
	}
	// The refabrication detour, the one place the ledger walks backwards.
	if cur == StatusInspecting && next == StatusRefabricating {
		return true
	}
	if cur == StatusRefabricating && next == StatusFabricating {
		return true
	}
	return statusRank[next] > statusRank[cur]
}

// Update is the change one event implies. A nil field means "leave it alone".
type Update struct {
	// Status is the status the event implies, or "" for no status change.
	Status string
	// Stage, Attempt, Serials and the shipment fields are set when the event
	// carries them.
	Stage          *string
	Attempt        *int
	Serials        []string
	Carrier        *string
	TrackingNumber *string
	// Detail is the timeline line for this event.
	Detail string
	// Create carries enough to insert a missing order, for the order.accepted
	// event. It is nil for every other event.
	Create *Order
}

// Event payloads, narrowed to the fields the ledger needs.
type (
	orderAccepted struct {
		OrderID  string  `json:"orderId"`
		Customer string  `json:"customer"`
		Model    string  `json:"model"`
		Quantity int     `json:"quantity"`
		Options  Options `json:"options"`
		Total    float64 `json:"total"`
		Currency string  `json:"currency"`
	}
	fabricationRequested struct {
		OrderID       string `json:"orderId"`
		Attempt       int    `json:"attempt"`
		Model         string `json:"model"`
		Quantity      int    `json:"quantity"`
		ReservationID string `json:"reservationId"`
	}
	fabricationStage struct {
		OrderID    string `json:"orderId"`
		Attempt    int    `json:"attempt"`
		Serial     string `json:"serial"`
		Stage      string `json:"stage"`
		StageIndex int    `json:"stageIndex"`
		DurationMs int    `json:"durationMs"`
	}
	fabricationCompleted struct {
		OrderID    string   `json:"orderId"`
		Attempt    int      `json:"attempt"`
		Model      string   `json:"model"`
		Serials    []string `json:"serials"`
		DurationMs int      `json:"durationMs"`
	}
	inspectionResult struct {
		OrderID  string   `json:"orderId"`
		Attempt  int      `json:"attempt"`
		Serials  []string `json:"serials"`
		Findings []struct {
			Serial string  `json:"serial"`
			Stage  string  `json:"stage"`
			Metric string  `json:"metric"`
			Score  float64 `json:"score"`
			Reason string  `json:"reason"`
		} `json:"findings"`
	}
	orderScrapped struct {
		OrderID string `json:"orderId"`
		Attempt int    `json:"attempt"`
		Reason  string `json:"reason"`
	}
	shipment struct {
		OrderID        string `json:"orderId"`
		Carrier        string `json:"carrier"`
		TrackingNumber string `json:"trackingNumber"`
		ETASeconds     int    `json:"etaSeconds"`
	}
)

// ErrNotForLedger means an event carries nothing the ledger records.
type ErrNotForLedger struct{ Subject string }

func (e ErrNotForLedger) Error() string {
	return fmt.Sprintf("ledger: %s carries no ledger update", e.Subject)
}

// Subjects the ledger understands.
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

// Interpret turns an event body into the order id it concerns and the update it
// implies. It returns ErrNotForLedger for subjects the ledger ignores.
func Interpret(subject string, eventTime time.Time, data json.RawMessage) (string, Update, error) {
	switch subject {
	case SubjectOrderAccepted:
		var d orderAccepted
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		// The processor also creates the order over HTTP; if the event wins the
		// race, or the POST was lost, the ledger can still build the row.
		created := Order{
			ID:        d.OrderID,
			Customer:  d.Customer,
			Model:     d.Model,
			Quantity:  d.Quantity,
			Options:   d.Options,
			Status:    StatusAccepted,
			Attempt:   1,
			CreatedAt: eventTime,
			UpdatedAt: eventTime,
		}
		if d.Total > 0 || d.Currency != "" {
			created.Quote = &Quote{Total: d.Total, Currency: d.Currency}
		}
		return d.OrderID, Update{
			Status: StatusAccepted,
			Detail: fmt.Sprintf("Order accepted: %d x %s", d.Quantity, d.Model),
			Create: &created,
		}, nil

	case SubjectFabricationRequested:
		var d fabricationRequested
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		up := Update{
			Status:  StatusFabricating,
			Attempt: &d.Attempt,
			Detail:  fmt.Sprintf("Fabrication requested, attempt %d", d.Attempt),
		}
		if d.ReservationID != "" {
			up.Detail += " (reservation " + d.ReservationID + ")"
		}
		return d.OrderID, up, nil

	case SubjectFabricationStage:
		var d fabricationStage
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		stage := d.Stage
		up := Update{
			Status: StatusFabricating,
			Stage:  &stage,
			Detail: fmt.Sprintf("Stage %d/7 %s on %s (%dms)", d.StageIndex, d.Stage, d.Serial, d.DurationMs),
		}
		if d.Serial != "" {
			up.Serials = []string{d.Serial}
		}
		return d.OrderID, up, nil

	case SubjectFabricationCompleted:
		var d fabricationCompleted
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		empty := ""
		return d.OrderID, Update{
			Status:  StatusInspecting,
			Stage:   &empty,
			Serials: d.Serials,
			Detail: fmt.Sprintf("Fabrication completed: %d unit(s) in %dms; inspecting for side fumbling",
				len(d.Serials), d.DurationMs),
		}, nil

	case SubjectInspectionPassed:
		var d inspectionResult
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		// Inspection passing does not itself advance the status; dispatch does.
		return d.OrderID, Update{
			Serials: d.Serials,
			Detail:  fmt.Sprintf("Inspection passed on attempt %d: no side fumbling detected", d.Attempt),
		}, nil

	case SubjectInspectionFailed:
		var d inspectionResult
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		return d.OrderID, Update{
			Status: StatusRefabricating,
			Detail: fmt.Sprintf("Inspection failed on attempt %d: %s", d.Attempt, describeFindings(d)),
		}, nil

	case SubjectOrderScrapped:
		var d orderScrapped
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		return d.OrderID, Update{
			Status: StatusScrapped,
			Detail: fmt.Sprintf("Scrapped after attempt %d: %s", d.Attempt, d.Reason),
		}, nil

	case SubjectShipmentDispatched:
		var d shipment
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		return d.OrderID, Update{
			Status:         StatusDispatched,
			Carrier:        &d.Carrier,
			TrackingNumber: &d.TrackingNumber,
			Detail: fmt.Sprintf("Dispatched by %s, tracking %s, eta %ds",
				d.Carrier, d.TrackingNumber, d.ETASeconds),
		}, nil

	case SubjectShipmentDelivered:
		var d shipment
		if err := json.Unmarshal(data, &d); err != nil {
			return "", Update{}, err
		}
		return d.OrderID, Update{
			Status:         StatusDelivered,
			Carrier:        &d.Carrier,
			TrackingNumber: &d.TrackingNumber,
			Detail:         fmt.Sprintf("Delivered by %s, tracking %s", d.Carrier, d.TrackingNumber),
		}, nil

	default:
		return "", Update{}, ErrNotForLedger{Subject: subject}
	}
}

func describeFindings(d inspectionResult) string {
	if len(d.Findings) == 0 {
		return "no findings recorded"
	}
	parts := make([]string, 0, len(d.Findings))
	for _, f := range d.Findings {
		parts = append(parts, fmt.Sprintf("%s %s/%s z=%.2f", f.Serial, f.Stage, f.Metric, f.Score))
	}
	if len(parts) > 4 {
		return strings.Join(parts[:4], "; ") + fmt.Sprintf("; and %d more", len(parts)-4)
	}
	return strings.Join(parts, "; ")
}

// MergeSerials adds new serials to existing ones, preserving order and dropping
// duplicates. The casing line reports serials one stage event at a time.
func MergeSerials(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	out := make([]string, 0, len(existing)+len(incoming))
	for _, list := range [][]string{existing, incoming} {
		for _, s := range list {
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
