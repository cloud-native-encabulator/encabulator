// Package api is the malaise engine's HTTP surface.
package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/catalog"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/engine"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/malaise"
)

// maxRequestBody bounds POST bodies.
const maxRequestBody = 64 << 10

// defaultListLimit is how much history GET /v1/malaises?state=all returns.
const defaultListLimit = 100

// Router is the subset of *httpx.Server (or an http.ServeMux) the API needs.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// API serves the malaise routes.
type API struct {
	engine *engine.Engine
	log    *slog.Logger
	now    func() time.Time
}

// New builds the API.
func New(e *engine.Engine, log *slog.Logger, now func() time.Time) *API {
	if log == nil {
		log = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	return &API{engine: e, log: log, now: now}
}

// Register attaches the routes to r.
func (a *API) Register(r Router) {
	r.Handle("POST /v1/malaises", http.HandlerFunc(a.create))
	r.Handle("GET /v1/malaises", http.HandlerFunc(a.list))
	r.Handle("DELETE /v1/malaises", http.HandlerFunc(a.clearAll))
	r.Handle("DELETE /v1/malaises/{id}", http.HandlerFunc(a.clear))
	r.Handle("GET /v1/malaises/{id}", http.HandlerFunc(a.get))
	r.Handle("GET /v1/directives", http.HandlerFunc(a.directives))
	r.Handle("GET /v1/targets", http.HandlerFunc(a.targets))
}

// view is a malaise plus the countdown the console draws.
type view struct {
	malaise.Malaise
	RemainingSeconds int `json:"remainingSeconds"`
}

func (a *API) view(m malaise.Malaise) view {
	return view{Malaise: m, RemainingSeconds: m.RemainingSeconds(a.now())}
}

type listResponse struct {
	Malaises []view `json:"malaises"`
	Count    int    `json:"count"`
}

type directivesResponse struct {
	Directives []malaise.Directive `json:"directives"`
}

type clearAllResponse struct {
	Cleared int `json:"cleared"`
}

type targetsResponse struct {
	Services         []string `json:"services"`
	AppLevelServices []string `json:"appLevelServices"`
	Kinds            []string `json:"kinds"`
	KubernetesReady  bool     `json:"kubernetesReady"`
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var req malaise.Request
	if err := httpx.DecodeJSON(r, &req, maxRequestBody); err != nil {
		httpx.WriteErr(w, err)
		return
	}

	m, err := a.engine.Create(r.Context(), req)
	if err != nil {
		a.writeEngineError(w, err)
		return
	}
	w.Header().Set("Location", "/v1/malaises/"+m.ID)
	httpx.WriteJSON(w, http.StatusCreated, a.view(m))
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	switch state {
	case "", "active", "all":
	default:
		httpx.WriteError(w, http.StatusBadRequest, "invalid_malaise",
			`state must be "active" or "all"`)
		return
	}

	limit := defaultListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_malaise", "limit must be a positive integer")
			return
		}
		limit = parsed
	}

	items, err := a.engine.List(r.Context(), state, limit)
	if err != nil {
		a.writeEngineError(w, err)
		return
	}
	views := make([]view, 0, len(items))
	for _, m := range items {
		views = append(views, a.view(m))
	}
	httpx.WriteJSON(w, http.StatusOK, listResponse{Malaises: views, Count: len(views)})
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	m, err := a.engine.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.writeEngineError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a.view(m))
}

func (a *API) clear(w http.ResponseWriter, r *http.Request) {
	m, err := a.engine.Clear(r.Context(), r.PathValue("id"))
	if err != nil {
		a.writeEngineError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a.view(m))
}

func (a *API) clearAll(w http.ResponseWriter, r *http.Request) {
	cleared, err := a.engine.ClearAll(r.Context())
	if err != nil {
		a.writeEngineError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, clearAllResponse{Cleared: cleared})
}

func (a *API) directives(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	if service == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "service is required")
		return
	}
	directives, err := a.engine.Directives(r.Context(), service)
	if err != nil {
		a.writeEngineError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, directivesResponse{Directives: directives})
}

// targets tells the console what it may ask for, so the UI does not have to
// hard-code the catalog.
func (a *API) targets(w http.ResponseWriter, _ *http.Request) {
	kinds := make([]string, 0, len(malaise.Kinds))
	for _, k := range malaise.Kinds {
		kinds = append(kinds, string(k))
	}
	httpx.WriteJSON(w, http.StatusOK, targetsResponse{
		Services:         catalog.Services,
		AppLevelServices: catalog.AppLevelServices,
		Kinds:            kinds,
		KubernetesReady:  a.engine.HasCluster(),
	})
}

func (a *API) writeEngineError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, malaise.ErrInvalid):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_malaise", err.Error())
	case errors.Is(err, engine.ErrUnknownTarget):
		httpx.WriteError(w, http.StatusBadRequest, "unknown_target", err.Error())
	case errors.Is(err, engine.ErrClusterUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "kubernetes_unavailable", err.Error())
	case errors.Is(err, engine.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "no malaise with that id")
	default:
		a.log.Error("malaise request failed", "error", err.Error())
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error",
			"the engine could not induce malaise")
	}
}
