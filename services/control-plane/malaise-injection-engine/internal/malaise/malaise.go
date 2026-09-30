// Package malaise defines what a malaise is, what its parameters mean, and
// which of them the caller is allowed to ask for.
//
// The wire shape of the app-level directive parameters matches the Go kit's
// malaise middleware exactly; the Kubernetes-level parameters are additions the
// middleware ignores.
package malaise

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// IDPrefix is the prefix every malaise identifier carries.
const IDPrefix = "mal_"

// Duration bounds from the design.
const (
	MinDurationSeconds     = 1
	MaxDurationSeconds     = 900
	DefaultDurationSeconds = 120
)

// Guard rails on the more energetic parameters. These are not in the design;
// they exist so that a typo in the console cannot take the cluster down in a
// way the reconcile loop cannot undo.
const (
	MaxPodKillCount = 20
	MaxReplicas     = 20
	MaxMillicores   = 8000
	MaxLatencyMs    = 60000
)

// Kind is what sort of suffering to induce.
type Kind string

// The app-level kinds, applied by the middleware inside each service.
const (
	KindLatency    Kind = "latency"
	KindError      Kind = "error"
	KindCongestion Kind = "congestion"
	KindCPUBurn    Kind = "cpu-burn"
)

// The Kubernetes-level kinds, applied by this service against the API server.
const (
	KindPodKill   Kind = "pod-kill"
	KindPartition Kind = "partition"
	KindScale     Kind = "scale"
)

// Kinds lists every kind in a stable order.
var Kinds = []Kind{
	KindLatency, KindError, KindCongestion, KindCPUBurn,
	KindPodKill, KindPartition, KindScale,
}

// AppLevel reports whether the kind is honoured by the in-process middleware.
func (k Kind) AppLevel() bool {
	switch k {
	case KindLatency, KindError, KindCongestion, KindCPUBurn:
		return true
	default:
		return false
	}
}

// Kubernetes reports whether the kind acts on the Kubernetes API.
func (k Kind) Kubernetes() bool {
	switch k {
	case KindPodKill, KindPartition, KindScale:
		return true
	default:
		return false
	}
}

// Instant reports whether the kind takes effect once rather than for a duration.
func (k Kind) Instant() bool { return k == KindPodKill }

// Valid reports whether the kind is known.
func (k Kind) Valid() bool { return k.AppLevel() || k.Kubernetes() }

// State is where a malaise is in its short life.
type State string

const (
	// StateActive means the fault is in force.
	StateActive State = "active"
	// StateCompleted means an instant fault has been applied and is over.
	StateCompleted State = "completed"
	// StateExpired means the duration elapsed and any change was reverted.
	StateExpired State = "expired"
	// StateCleared means an operator ended it early.
	StateCleared State = "cleared"
	// StateFailed means the fault could not be applied.
	StateFailed State = "failed"
)

// Terminal reports whether no further reconciliation is needed.
func (s State) Terminal() bool { return s != StateActive }

// StringList is a JSON value that may be a single string or an array of them,
// because "from": "*" reads better than "from": ["*"].
type StringList []string

// UnmarshalJSON accepts either shape.
func (l *StringList) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*l = StringList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("expected a string or an array of strings")
	}
	*l = many
	return nil
}

// Wildcard reports whether the list is the single entry "*".
func (l StringList) Wildcard() bool { return len(l) == 1 && l[0] == "*" }

// Params is the union of every kind's parameters. The app-level field names and
// their JSON tags match the kit's middleware.
type Params struct {
	// latency
	Ms       int `json:"ms,omitempty"`
	JitterMs int `json:"jitterMs,omitempty"`
	// latency and error
	Percent *int `json:"percent,omitempty"`
	// error
	Status int `json:"status,omitempty"`
	// congestion
	MaxConcurrent  int `json:"maxConcurrent,omitempty"`
	QueueTimeoutMs int `json:"queueTimeoutMs,omitempty"`
	// cpu-burn
	Millicores int `json:"millicores,omitempty"`
	// pod-kill
	Count int `json:"count,omitempty"`
	// partition
	From StringList `json:"from,omitempty"`
	// scale
	Replicas *int32 `json:"replicas,omitempty"`
}

// Detail records what a malaise actually did, so the console can show it and
// the reconcile loop can undo it.
type Detail struct {
	// KilledPods lists the pods a pod-kill deleted.
	KilledPods []string `json:"killedPods,omitempty"`
	// NetworkPolicy is the name of the policy a partition created.
	NetworkPolicy string `json:"networkPolicy,omitempty"`
	// OriginalReplicas is the replica count a scale must restore.
	OriginalReplicas *int32 `json:"originalReplicas,omitempty"`
	// AppliedReplicas is the replica count a scale set.
	AppliedReplicas *int32 `json:"appliedReplicas,omitempty"`
}

// Malaise is one injected fault.
type Malaise struct {
	ID              string     `json:"id"`
	Target          string     `json:"target"`
	Kind            Kind       `json:"kind"`
	Params          Params     `json:"params"`
	DurationSeconds int        `json:"durationSeconds"`
	State           State      `json:"state"`
	CreatedAt       time.Time  `json:"createdAt"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	EndedAt         *time.Time `json:"endedAt,omitempty"`
	Detail          *Detail    `json:"detail,omitempty"`
	// Error explains a failed malaise.
	Error string `json:"error,omitempty"`
}

// Expired reports whether the malaise has outlived its duration.
func (m Malaise) Expired(now time.Time) bool { return !now.Before(m.ExpiresAt) }

// RemainingSeconds is the countdown the console shows.
func (m Malaise) RemainingSeconds(now time.Time) int {
	if m.State.Terminal() || m.Expired(now) {
		return 0
	}
	return int(m.ExpiresAt.Sub(now).Round(time.Second).Seconds())
}

// Request is the body of POST /v1/malaises.
type Request struct {
	Target          string `json:"target"`
	Kind            Kind   `json:"kind"`
	Params          Params `json:"params"`
	DurationSeconds *int   `json:"durationSeconds"`
}

// ErrInvalid marks a request the caller must fix.
var ErrInvalid = errors.New("invalid malaise")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// NewID returns a fresh malaise identifier.
func NewID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return IDPrefix + id.String(), nil
}

// Build validates and normalises a request into a malaise record. It does not
// touch the cluster or the store.
func Build(req Request, now time.Time) (Malaise, error) {
	target := strings.TrimSpace(req.Target)
	if target == "" {
		return Malaise{}, invalid("target is required")
	}
	if !req.Kind.Valid() {
		return Malaise{}, invalid("unknown kind %q", req.Kind)
	}

	duration := DefaultDurationSeconds
	if req.DurationSeconds != nil {
		duration = *req.DurationSeconds
	}
	if duration < MinDurationSeconds || duration > MaxDurationSeconds {
		return Malaise{}, invalid("durationSeconds must be between %d and %d",
			MinDurationSeconds, MaxDurationSeconds)
	}

	params, err := normaliseParams(req.Kind, req.Params)
	if err != nil {
		return Malaise{}, err
	}

	id, err := NewID()
	if err != nil {
		return Malaise{}, err
	}

	now = now.UTC().Truncate(time.Millisecond)
	m := Malaise{
		ID:              id,
		Target:          target,
		Kind:            req.Kind,
		Params:          params,
		DurationSeconds: duration,
		State:           StateActive,
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Duration(duration) * time.Second),
	}
	// An instant fault has no duration to live out: it happens, and it is over.
	if req.Kind.Instant() {
		m.State = StateCompleted
		m.ExpiresAt = now
		m.EndedAt = &now
	}
	return m, nil
}

func normaliseParams(kind Kind, p Params) (Params, error) {
	// Discard anything that does not belong to this kind, so a record never
	// claims a parameter it does not honour.
	out := Params{}

	switch kind {
	case KindLatency:
		if p.Ms <= 0 {
			return out, invalid("latency requires ms > 0")
		}
		if p.Ms > MaxLatencyMs {
			return out, invalid("latency ms must be at most %d", MaxLatencyMs)
		}
		if p.JitterMs < 0 {
			return out, invalid("jitterMs must not be negative")
		}
		if p.JitterMs > p.Ms {
			return out, invalid("jitterMs must not exceed ms")
		}
		percent, err := percentOrDefault(p.Percent)
		if err != nil {
			return out, err
		}
		out.Ms, out.JitterMs, out.Percent = p.Ms, p.JitterMs, &percent

	case KindError:
		if p.Status < 400 || p.Status > 599 {
			return out, invalid("error requires a status between 400 and 599")
		}
		percent, err := percentOrDefault(p.Percent)
		if err != nil {
			return out, err
		}
		out.Status, out.Percent = p.Status, &percent

	case KindCongestion:
		if p.MaxConcurrent < 1 {
			return out, invalid("congestion requires maxConcurrent >= 1")
		}
		timeout := p.QueueTimeoutMs
		if timeout == 0 {
			timeout = 2000
		}
		if timeout < 0 {
			return out, invalid("queueTimeoutMs must not be negative")
		}
		out.MaxConcurrent, out.QueueTimeoutMs = p.MaxConcurrent, timeout

	case KindCPUBurn:
		if p.Millicores < 1 || p.Millicores > MaxMillicores {
			return out, invalid("cpu-burn requires millicores between 1 and %d", MaxMillicores)
		}
		out.Millicores = p.Millicores

	case KindPodKill:
		count := p.Count
		if count == 0 {
			count = 1
		}
		if count < 1 || count > MaxPodKillCount {
			return out, invalid("pod-kill count must be between 1 and %d", MaxPodKillCount)
		}
		out.Count = count

	case KindPartition:
		from := p.From
		if len(from) == 0 {
			from = StringList{"*"}
		}
		for _, name := range from {
			if strings.TrimSpace(name) == "" {
				return out, invalid("partition from must not contain empty names")
			}
		}
		if !from.Wildcard() {
			for _, name := range from {
				if name == "*" {
					return out, invalid(`partition from may not mix "*" with names`)
				}
			}
		}
		out.From = from

	case KindScale:
		if p.Replicas == nil {
			return out, invalid("scale requires replicas")
		}
		if *p.Replicas < 0 || *p.Replicas > MaxReplicas {
			return out, invalid("scale replicas must be between 0 and %d", MaxReplicas)
		}
		replicas := *p.Replicas
		out.Replicas = &replicas

	default:
		return out, invalid("unknown kind %q", kind)
	}
	return out, nil
}

func percentOrDefault(p *int) (int, error) {
	if p == nil {
		return 100, nil
	}
	if *p < 0 || *p > 100 {
		return 0, invalid("percent must be between 0 and 100")
	}
	return *p, nil
}

// Directive is the app-level view of a malaise, as the middleware wants it.
type Directive struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	Params    Params    `json:"params"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Directive renders the malaise for the middleware.
func (m Malaise) Directive() Directive {
	return Directive{ID: m.ID, Kind: m.Kind, Params: m.Params, ExpiresAt: m.ExpiresAt}
}
