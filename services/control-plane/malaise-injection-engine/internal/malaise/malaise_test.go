package malaise

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var epoch = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func TestKindClassification(t *testing.T) {
	app := map[Kind]bool{KindLatency: true, KindError: true, KindCongestion: true, KindCPUBurn: true}
	k8s := map[Kind]bool{KindPodKill: true, KindPartition: true, KindScale: true}

	for _, k := range Kinds {
		if app[k] == k8s[k] {
			t.Fatalf("kind %q is classified as both or neither", k)
		}
		if k.AppLevel() != app[k] {
			t.Errorf("%q AppLevel = %v, want %v", k, k.AppLevel(), app[k])
		}
		if k.Kubernetes() != k8s[k] {
			t.Errorf("%q Kubernetes = %v, want %v", k, k.Kubernetes(), k8s[k])
		}
		if !k.Valid() {
			t.Errorf("%q should be valid", k)
		}
	}
	if Kind("ennui").Valid() {
		t.Error("an unknown kind should not be valid")
	}
	if !KindPodKill.Instant() {
		t.Error("pod-kill is instant")
	}
	if KindScale.Instant() {
		t.Error("scale is not instant")
	}
}

func TestStateTerminal(t *testing.T) {
	if StateActive.Terminal() {
		t.Error("active is not terminal")
	}
	for _, s := range []State{StateCompleted, StateExpired, StateCleared, StateFailed} {
		if !s.Terminal() {
			t.Errorf("%q should be terminal", s)
		}
	}
}

func TestStringListAcceptsBothShapes(t *testing.T) {
	cases := []struct {
		body string
		want []string
	}{
		{`"*"`, []string{"*"}},
		{`["a","b"]`, []string{"a", "b"}},
		{`[]`, []string{}},
	}
	for _, c := range cases {
		var got StringList
		if err := json.Unmarshal([]byte(c.body), &got); err != nil {
			t.Fatalf("%s: %v", c.body, err)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s parsed to %v, want %v", c.body, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s parsed to %v, want %v", c.body, got, c.want)
			}
		}
	}
	var bad StringList
	if err := json.Unmarshal([]byte(`{"from":1}`), &bad); err == nil {
		t.Error("an object should not parse as a string list")
	}
	if !(StringList{"*"}).Wildcard() {
		t.Error(`["*"] is the wildcard`)
	}
	if (StringList{"*", "a"}).Wildcard() {
		t.Error("a mixed list is not the wildcard")
	}
}

func TestNewIDIsPrefixedAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID: %v", err)
		}
		if !strings.HasPrefix(id, IDPrefix) {
			t.Fatalf("id %q lacks the %q prefix", id, IDPrefix)
		}
		if seen[id] {
			t.Fatalf("id %q repeated", id)
		}
		seen[id] = true
	}
}

func intPtr(v int) *int     { return &v }
func i32Ptr(v int32) *int32 { return &v }

func TestBuildDefaults(t *testing.T) {
	m, err := Build(Request{
		Target: "dingle-arm-gateway",
		Kind:   KindLatency,
		Params: Params{Ms: 400},
	}, epoch)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.DurationSeconds != DefaultDurationSeconds {
		t.Errorf("duration = %d, want the default %d", m.DurationSeconds, DefaultDurationSeconds)
	}
	if m.State != StateActive {
		t.Errorf("state = %q, want active", m.State)
	}
	if !m.CreatedAt.Equal(epoch) {
		t.Errorf("createdAt = %v, want %v", m.CreatedAt, epoch)
	}
	want := epoch.Add(DefaultDurationSeconds * time.Second)
	if !m.ExpiresAt.Equal(want) {
		t.Errorf("expiresAt = %v, want %v", m.ExpiresAt, want)
	}
	if m.Params.Percent == nil || *m.Params.Percent != 100 {
		t.Errorf("percent = %v, want 100", m.Params.Percent)
	}
	if m.EndedAt != nil {
		t.Error("a fresh malaise has not ended")
	}
}

func TestBuildTrimsTheTarget(t *testing.T) {
	m, err := Build(Request{Target: "  spurving-bearing-quoter  ", Kind: KindCPUBurn,
		Params: Params{Millicores: 500}}, epoch)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.Target != "spurving-bearing-quoter" {
		t.Errorf("target = %q", m.Target)
	}
}

// An instant kind has no duration to live out.
func TestBuildPodKillIsInstantlyComplete(t *testing.T) {
	m, err := Build(Request{Target: "logarithmic-casing-line", Kind: KindPodKill}, epoch)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.State != StateCompleted {
		t.Errorf("state = %q, want completed", m.State)
	}
	if !m.ExpiresAt.Equal(epoch) {
		t.Errorf("expiresAt = %v, want the creation time", m.ExpiresAt)
	}
	if m.EndedAt == nil || !m.EndedAt.Equal(epoch) {
		t.Errorf("endedAt = %v, want the creation time", m.EndedAt)
	}
	if m.Params.Count != 1 {
		t.Errorf("count = %d, want the default 1", m.Params.Count)
	}
	if m.RemainingSeconds(epoch) != 0 {
		t.Error("an instant malaise has no countdown")
	}
}

func TestBuildNormalisesEachKind(t *testing.T) {
	cases := []struct {
		name  string
		req   Request
		check func(*testing.T, Malaise)
	}{
		{
			name: "latency keeps its own fields only",
			req: Request{Target: "t", Kind: KindLatency, Params: Params{
				Ms: 400, JitterMs: 100, Percent: intPtr(25),
				// Fields from other kinds must be discarded.
				Status: 503, Millicores: 900, Count: 7, Replicas: i32Ptr(3),
			}},
			check: func(t *testing.T, m Malaise) {
				if m.Params.Ms != 400 || m.Params.JitterMs != 100 || *m.Params.Percent != 25 {
					t.Errorf("params = %+v", m.Params)
				}
				if m.Params.Status != 0 || m.Params.Millicores != 0 ||
					m.Params.Count != 0 || m.Params.Replicas != nil {
					t.Errorf("foreign parameters survived: %+v", m.Params)
				}
			},
		},
		{
			name: "error",
			req:  Request{Target: "t", Kind: KindError, Params: Params{Status: 503}},
			check: func(t *testing.T, m Malaise) {
				if m.Params.Status != 503 || *m.Params.Percent != 100 {
					t.Errorf("params = %+v", m.Params)
				}
			},
		},
		{
			name: "congestion defaults the queue timeout",
			req:  Request{Target: "t", Kind: KindCongestion, Params: Params{MaxConcurrent: 2}},
			check: func(t *testing.T, m Malaise) {
				if m.Params.MaxConcurrent != 2 || m.Params.QueueTimeoutMs != 2000 {
					t.Errorf("params = %+v", m.Params)
				}
			},
		},
		{
			name: "cpu-burn",
			req:  Request{Target: "t", Kind: KindCPUBurn, Params: Params{Millicores: 500}},
			check: func(t *testing.T, m Malaise) {
				if m.Params.Millicores != 500 {
					t.Errorf("params = %+v", m.Params)
				}
			},
		},
		{
			name: "partition defaults to the wildcard",
			req:  Request{Target: "t", Kind: KindPartition},
			check: func(t *testing.T, m Malaise) {
				if !m.Params.From.Wildcard() {
					t.Errorf("from = %v, want the wildcard", m.Params.From)
				}
			},
		},
		{
			name: "partition keeps named peers",
			req:  Request{Target: "t", Kind: KindPartition, Params: Params{From: StringList{"a", "b"}}},
			check: func(t *testing.T, m Malaise) {
				if len(m.Params.From) != 2 || m.Params.From[0] != "a" {
					t.Errorf("from = %v", m.Params.From)
				}
			},
		},
		{
			name: "scale to zero is allowed",
			req:  Request{Target: "t", Kind: KindScale, Params: Params{Replicas: i32Ptr(0)}},
			check: func(t *testing.T, m Malaise) {
				if m.Params.Replicas == nil || *m.Params.Replicas != 0 {
					t.Errorf("replicas = %v", m.Params.Replicas)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := Build(c.req, epoch)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			c.check(t, m)
		})
	}
}

func TestBuildRejectsBadRequests(t *testing.T) {
	cases := map[string]Request{
		"no target":              {Kind: KindLatency, Params: Params{Ms: 1}},
		"blank target":           {Target: "   ", Kind: KindLatency, Params: Params{Ms: 1}},
		"no kind":                {Target: "t"},
		"unknown kind":           {Target: "t", Kind: "ennui"},
		"duration too short":     {Target: "t", Kind: KindLatency, Params: Params{Ms: 1}, DurationSeconds: intPtr(0)},
		"duration too long":      {Target: "t", Kind: KindLatency, Params: Params{Ms: 1}, DurationSeconds: intPtr(901)},
		"latency without ms":     {Target: "t", Kind: KindLatency},
		"latency ms too large":   {Target: "t", Kind: KindLatency, Params: Params{Ms: MaxLatencyMs + 1}},
		"negative jitter":        {Target: "t", Kind: KindLatency, Params: Params{Ms: 100, JitterMs: -1}},
		"jitter exceeds ms":      {Target: "t", Kind: KindLatency, Params: Params{Ms: 100, JitterMs: 200}},
		"percent too high":       {Target: "t", Kind: KindLatency, Params: Params{Ms: 100, Percent: intPtr(101)}},
		"percent negative":       {Target: "t", Kind: KindLatency, Params: Params{Ms: 100, Percent: intPtr(-1)}},
		"error without status":   {Target: "t", Kind: KindError},
		"error status too low":   {Target: "t", Kind: KindError, Params: Params{Status: 399}},
		"error status too high":  {Target: "t", Kind: KindError, Params: Params{Status: 600}},
		"congestion zero":        {Target: "t", Kind: KindCongestion},
		"negative queue":         {Target: "t", Kind: KindCongestion, Params: Params{MaxConcurrent: 1, QueueTimeoutMs: -1}},
		"cpu-burn zero":          {Target: "t", Kind: KindCPUBurn},
		"cpu-burn too big":       {Target: "t", Kind: KindCPUBurn, Params: Params{Millicores: MaxMillicores + 1}},
		"pod-kill too many":      {Target: "t", Kind: KindPodKill, Params: Params{Count: MaxPodKillCount + 1}},
		"pod-kill negative":      {Target: "t", Kind: KindPodKill, Params: Params{Count: -1}},
		"partition blank peer":   {Target: "t", Kind: KindPartition, Params: Params{From: StringList{" "}}},
		"partition mixed":        {Target: "t", Kind: KindPartition, Params: Params{From: StringList{"*", "a"}}},
		"scale without replicas": {Target: "t", Kind: KindScale},
		"scale negative":         {Target: "t", Kind: KindScale, Params: Params{Replicas: i32Ptr(-1)}},
		"scale too many":         {Target: "t", Kind: KindScale, Params: Params{Replicas: i32Ptr(MaxReplicas + 1)}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Build(req, epoch)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestBuildHonoursAnExplicitDuration(t *testing.T) {
	m, err := Build(Request{Target: "t", Kind: KindError,
		Params: Params{Status: 503}, DurationSeconds: intPtr(900)}, epoch)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.DurationSeconds != 900 {
		t.Errorf("duration = %d, want 900", m.DurationSeconds)
	}
	if !m.ExpiresAt.Equal(epoch.Add(900 * time.Second)) {
		t.Errorf("expiresAt = %v", m.ExpiresAt)
	}
}

func TestExpiredAndRemaining(t *testing.T) {
	m, err := Build(Request{Target: "t", Kind: KindError,
		Params: Params{Status: 503}, DurationSeconds: intPtr(60)}, epoch)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.Expired(epoch) {
		t.Error("a fresh malaise is not expired")
	}
	if got := m.RemainingSeconds(epoch); got != 60 {
		t.Errorf("remaining = %d, want 60", got)
	}
	if got := m.RemainingSeconds(epoch.Add(45 * time.Second)); got != 15 {
		t.Errorf("remaining = %d, want 15", got)
	}
	if !m.Expired(epoch.Add(60 * time.Second)) {
		t.Error("a malaise is expired at its expiry")
	}
	if got := m.RemainingSeconds(epoch.Add(90 * time.Second)); got != 0 {
		t.Errorf("remaining = %d, want 0", got)
	}

	m.State = StateCleared
	if got := m.RemainingSeconds(epoch); got != 0 {
		t.Errorf("a cleared malaise has remaining = %d, want 0", got)
	}
}

// The app-level directive must be byte-compatible with the kit's middleware.
func TestDirectiveWireShape(t *testing.T) {
	m, err := Build(Request{Target: "dingle-arm-gateway", Kind: KindLatency,
		Params:          Params{Ms: 400, JitterMs: 100, Percent: intPtr(50)},
		DurationSeconds: intPtr(120)}, epoch)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	encoded, err := json.Marshal(m.Directive())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"id", "kind", "params", "expiresAt"} {
		if _, ok := got[key]; !ok {
			t.Errorf("directive is missing %q: %s", key, encoded)
		}
	}
	params, ok := got["params"].(map[string]any)
	if !ok {
		t.Fatalf("params is not an object: %s", encoded)
	}
	for key, want := range map[string]float64{"ms": 400, "jitterMs": 100, "percent": 50} {
		if params[key] != want {
			t.Errorf("params[%q] = %v, want %v", key, params[key], want)
		}
	}
	// Parameters belonging to other kinds must not appear at all.
	for _, key := range []string{"status", "maxConcurrent", "millicores", "count", "from", "replicas"} {
		if _, ok := params[key]; ok {
			t.Errorf("params leaked %q: %s", key, encoded)
		}
	}
}

func TestMalaiseRoundTripsThroughJSON(t *testing.T) {
	m, err := Build(Request{Target: "valkey", Kind: KindScale,
		Params: Params{Replicas: i32Ptr(0)}}, epoch)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	m.Detail = &Detail{OriginalReplicas: i32Ptr(3), AppliedReplicas: i32Ptr(0)}

	encoded, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Malaise
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID != m.ID || back.Target != m.Target || back.Kind != m.Kind ||
		!back.ExpiresAt.Equal(m.ExpiresAt) || back.State != m.State {
		t.Errorf("round trip lost information: %+v vs %+v", back, m)
	}
	if back.Detail == nil || *back.Detail.OriginalReplicas != 3 {
		t.Errorf("detail round trip failed: %+v", back.Detail)
	}
}
