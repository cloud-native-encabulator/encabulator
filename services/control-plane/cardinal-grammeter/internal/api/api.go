// Package api exposes the grammeter's control surface: read the traffic
// configuration and live statistics, or change the configuration.
package api

import (
	"log/slog"
	"net/http"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/engine"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/journey"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/traffic"
)

// maxRequestBody bounds PUT bodies; the patch is a handful of fields.
const maxRequestBody = 64 << 10

// Router is the subset of the kit's *httpx.Server (or an http.ServeMux) the API
// needs.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// API serves /v1/traffic.
type API struct {
	engine *engine.Engine
	log    *slog.Logger
}

// New builds the API.
func New(e *engine.Engine, log *slog.Logger) *API {
	if log == nil {
		log = slog.Default()
	}
	return &API{engine: e, log: log}
}

// Register attaches the routes to r.
func (a *API) Register(r Router) {
	r.Handle("GET /v1/traffic", http.HandlerFunc(a.getTraffic))
	r.Handle("PUT /v1/traffic", http.HandlerFunc(a.putTraffic))
	r.Handle("GET /v1/journeys", http.HandlerFunc(a.getJourneys))
}

func (a *API) getTraffic(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, a.engine.Status())
}

func (a *API) putTraffic(w http.ResponseWriter, r *http.Request) {
	var patch engine.Patch
	if err := httpx.DecodeJSON(r, &patch, maxRequestBody); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_traffic", trim(err))
		return
	}
	if _, err := a.engine.Update(patch); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_traffic", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a.engine.Status())
}

// journeysResponse tells the console which journeys and patterns exist, so the
// controls do not have to be hard-coded in the UI.
type journeysResponse struct {
	Journeys   []string       `json:"journeys"`
	DefaultMix map[string]int `json:"defaultMix"`
	Patterns   []string       `json:"patterns"`
	MinRPS     float64        `json:"minRps"`
	MaxRPS     float64        `json:"maxRps"`
}

func (a *API) getJourneys(w http.ResponseWriter, _ *http.Request) {
	patterns := make([]string, 0, len(traffic.Patterns))
	for _, p := range traffic.Patterns {
		patterns = append(patterns, string(p))
	}
	httpx.WriteJSON(w, http.StatusOK, journeysResponse{
		Journeys:   journey.Names,
		DefaultMix: journey.DefaultMix(),
		Patterns:   patterns,
		MinRPS:     engine.MinRPS,
		MaxRPS:     engine.MaxRPS,
	})
}

// trim renders a decode error without the kit's status-error prefix.
func trim(err error) string {
	if se, ok := err.(*httpx.StatusError); ok {
		return se.Message
	}
	return err.Error()
}
