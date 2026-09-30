// Package api serves the differential girdlespring's flag API (design 8.10).
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/flags"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/identity"

	"github.com/jlbutler/cloud-native-encabulator/services/support-plane/differential-girdlespring/internal/store"
)

// Store is the persistence the API needs.
type Store interface {
	List(ctx context.Context) ([]flags.Flag, error)
	Get(ctx context.Context, key string) (flags.Flag, error)
	Put(ctx context.Context, key string, up store.Update) (flags.Flag, error)
}

// API holds the handlers.
type API struct {
	store Store
	log   *slog.Logger

	updates *prometheus.CounterVec
	polls   *prometheus.CounterVec
}

// New builds the API and registers its domain metrics.
func New(s Store, log *slog.Logger, reg prometheus.Registerer) *API {
	return &API{
		store: s,
		log:   log,
		updates: httpx.RegisterCounterVec(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "encabulator_flag_updates_total",
			Help: "Flag edits accepted, by key.",
		}, []string{"key"})),
		polls: httpx.RegisterCounterVec(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "encabulator_flag_polls_total",
			Help: "Flag set polls, by outcome.",
		}, []string{"result"})),
	}
}

// Routes registers every route on the kit's HTTP server.
func (a *API) Routes(s *httpx.Server) {
	s.Handle("GET /v1/flags", http.HandlerFunc(a.ListFlags))
	s.Handle("GET /v1/flags/{key}", http.HandlerFunc(a.GetFlag))
	s.Handle("GET /v1/flags/{key}/evaluate", http.HandlerFunc(a.EvaluateFlag))
	s.Handle("PUT /v1/flags/{key}", http.HandlerFunc(a.PutFlag))
}

// ListFlags serves GET /v1/flags with an ETag, honoring If-None-Match.
func (a *API) ListFlags(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.List(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	body := flags.ListResponse{Flags: list}
	payload, err := json.Marshal(body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag := ETag(payload)

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		a.polls.WithLabelValues("not_modified").Inc()
		w.WriteHeader(http.StatusNotModified)
		return
	}
	a.polls.WithLabelValues("modified").Inc()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// GetFlag serves GET /v1/flags/{key}.
func (a *API) GetFlag(w http.ResponseWriter, r *http.Request) {
	f, err := a.store.Get(r.Context(), r.PathValue("key"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, f)
}

// Evaluation is the response of the evaluate endpoint.
type Evaluation struct {
	Key     string          `json:"key"`
	Subject string          `json:"subject,omitempty"`
	Value   json.RawMessage `json:"value"`
	Reason  string          `json:"reason"`
}

// Evaluation reasons.
const (
	ReasonEveryone      = "applies_to_everyone"
	ReasonInRollout     = "in_rollout"
	ReasonOutOfRollout  = "out_of_rollout"
	ReasonUnknownFormat = "unreadable_value"
)

// EvaluateFlag serves GET /v1/flags/{key}/evaluate?subject=.
func (a *API) EvaluateFlag(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	subject := r.URL.Query().Get("subject")

	f, err := a.store.Get(r.Context(), key)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, Evaluate(f, subject))
}

// Evaluate applies the rollout rule from design section 6. A boolean flag with
// a rolloutPercent yields its value inside the rollout and the opposite value
// outside it; anything without a rolloutPercent applies to everyone.
func Evaluate(f flags.Flag, subject string) Evaluation {
	out := Evaluation{Key: f.Key, Subject: subject, Value: f.Value, Reason: ReasonEveryone}
	if f.RolloutPercent == nil || f.Type != flags.TypeBoolean {
		return out
	}
	value, ok := f.Bool()
	if !ok {
		out.Reason = ReasonUnknownFormat
		return out
	}
	if flags.InRollout(f.Key, subject, *f.RolloutPercent) {
		out.Reason = ReasonInRollout
		return out
	}
	out.Reason = ReasonOutOfRollout
	out.Value = boolJSON(!value)
	return out
}

// PutRequest is the body of PUT /v1/flags/{key}.
type PutRequest struct {
	Value          json.RawMessage `json:"value"`
	RolloutPercent *int            `json:"rolloutPercent,omitempty"`
	Description    *string         `json:"description,omitempty"`
}

// PutFlag serves PUT /v1/flags/{key}. Operator or admin only.
func (a *API) PutFlag(w http.ResponseWriter, r *http.Request) {
	id := identity.FromRequest(r)
	if !id.HasRole(identity.RoleOperator, identity.RoleAdmin) {
		httpx.WriteError(w, http.StatusForbidden, httpx.CodeForbidden,
			"Adjusting the girdlespring requires the operator or admin role")
		return
	}

	key := r.PathValue("key")
	var req PutRequest
	if err := httpx.DecodeJSON(r, &req, 16<<10); err != nil {
		a.fail(w, r, err)
		return
	}

	existing, err := a.store.Get(r.Context(), key)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := ValidateUpdate(existing.Type, req); err != nil {
		a.fail(w, r, err)
		return
	}

	updated, err := a.store.Put(r.Context(), key, store.Update{
		Value:          req.Value,
		RolloutPercent: req.RolloutPercent,
		Description:    req.Description,
		UpdatedBy:      updatedBy(id),
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}

	a.updates.WithLabelValues(key).Inc()
	a.log.InfoContext(r.Context(), "girdlespring adjusted",
		slog.String("flag", key),
		slog.String("value", string(updated.Value)),
		slog.String("updated_by", updated.UpdatedBy))
	httpx.WriteJSON(w, http.StatusOK, updated)
}

// ValidateUpdate checks that the new value matches the flag's declared type and
// that rolloutPercent is in range and only used on booleans.
func ValidateUpdate(flagType string, req PutRequest) error {
	if len(req.Value) == 0 {
		return httpx.Errorf(http.StatusBadRequest, "invalid_flag_value", "value is required")
	}
	switch flagType {
	case flags.TypeBoolean:
		var b bool
		if err := json.Unmarshal(req.Value, &b); err != nil {
			return httpx.Errorf(http.StatusBadRequest, "invalid_flag_value",
				"%s is a boolean flag; value must be true or false", flagType)
		}
	case flags.TypeNumber:
		var n float64
		if err := json.Unmarshal(req.Value, &n); err != nil {
			return httpx.Errorf(http.StatusBadRequest, "invalid_flag_value",
				"this is a number flag; value must be a number")
		}
		if req.RolloutPercent != nil {
			return httpx.Errorf(http.StatusBadRequest, "invalid_rollout",
				"rolloutPercent applies to boolean flags only")
		}
	default:
		return httpx.Errorf(http.StatusInternalServerError, httpx.CodeInternal,
			"unknown flag type %q", flagType)
	}
	if req.RolloutPercent != nil && (*req.RolloutPercent < 0 || *req.RolloutPercent > 100) {
		return httpx.Errorf(http.StatusBadRequest, "invalid_rollout",
			"rolloutPercent must be between 0 and 100")
	}
	if req.Description != nil && len(*req.Description) > 500 {
		return httpx.Errorf(http.StatusBadRequest, "invalid_description",
			"description must be 500 characters or fewer")
	}
	return nil
}

// ETag is a strong entity tag over the serialized flag set.
func ETag(payload []byte) string {
	sum := sha256.Sum256(payload)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// matchesETag implements the If-None-Match comparison, including "*" and weak tags.
func matchesETag(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}

func updatedBy(id identity.Identity) string {
	if id.Subject == "" {
		return "unknown"
	}
	return id.Subject
}

func boolJSON(v bool) json.RawMessage {
	if v {
		return json.RawMessage("true")
	}
	return json.RawMessage("false")
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "No such flag")
		return
	}
	var se *httpx.StatusError
	if errors.As(err, &se) {
		httpx.WriteErr(w, se)
		return
	}
	a.log.ErrorContext(r.Context(), "girdlespring request failed", slog.Any("error", err))
	httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "Internal error")
}

// Seeds are the built-in flags from design section 6, with their descriptions.
func Seeds() []store.Seed {
	return []store.Seed{
		{Key: flags.TurboEncabulation, Type: flags.TypeBoolean, Value: false,
			Description: "Halve stage durations; the quoter adds a 15% turbo surcharge."},
		{Key: flags.QuoteCache, Type: flags.TypeBoolean, Value: true,
			Description: "Cache quotes in Valkey for 60s."},
		{Key: flags.RetroEncabulatorSales, Type: flags.TypeBoolean, Value: true,
			Description: "When false, retro orders are rejected with model_unavailable."},
		{Key: flags.StrictSideFumbling, Type: flags.TypeBoolean, Value: false,
			Description: "Tighter anomaly thresholds in the side fumbling detector."},
		{Key: flags.AmuliteFoundryRate, Type: flags.TypeNumber, Value: 20,
			Description: "Units of each SKU restocked per 10s; 0 stops the foundry."},
		{Key: flags.GatewayRateLimitRPS, Type: flags.TypeNumber, Value: 50,
			Description: "Per-subject token bucket rate at the dingle arm gateway; burst is 2x."},
		{Key: flags.DingleArmDamping, Type: flags.TypeNumber, Value: 50,
			Description: "Percent reduction of the defect rate when a dingle arm is ordered."},
	}
}
