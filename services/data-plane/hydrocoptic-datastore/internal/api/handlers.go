package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/db"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/models"
	"github.com/rs/zerolog"
)

type Handler struct {
	db     *db.PostgresDB
	logger zerolog.Logger
}

func NewHandler(database *db.PostgresDB, logger zerolog.Logger) *Handler {
	return &Handler{
		db:     database,
		logger: logger,
	}
}

func (h *Handler) CreateEncabulation(w http.ResponseWriter, r *http.Request) {
	var req models.CreateEncabulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Validate request
	if req.Grammeters <= 0 {
		h.respondError(w, http.StatusBadRequest, "INVALID_GRAMMETERS", "Grammeters must be positive")
		return
	}
	if req.SinusoidalMode != "STANDARD" && req.SinusoidalMode != "INVERSE" && req.SinusoidalMode != "LOGARITHMIC" {
		h.respondError(w, http.StatusBadRequest, "INVALID_MODE", "Invalid sinusoidal mode")
		return
	}

	enc, err := h.db.CreateEncabulation(req)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to create encabulation")
		h.respondError(w, http.StatusInternalServerError, "DATABASE_ERROR", "Failed to create encabulation")
		return
	}

	h.respondJSON(w, http.StatusCreated, enc)
}

func (h *Handler) GetEncabulation(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	enc, err := h.db.GetEncabulation(id)
	if err != nil {
		h.logger.Error().Err(err).Str("id", id).Msg("Failed to get encabulation")
		h.respondError(w, http.StatusInternalServerError, "DATABASE_ERROR", "Failed to get encabulation")
		return
	}

	if enc == nil {
		h.respondError(w, http.StatusNotFound, "NOT_FOUND", "Encabulation not found")
		return
	}

	h.respondJSON(w, http.StatusOK, enc)
}

func (h *Handler) ListEncabulations(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	encabulations, total, err := h.db.ListEncabulations(page, pageSize)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to list encabulations")
		h.respondError(w, http.StatusInternalServerError, "DATABASE_ERROR", "Failed to list encabulations")
		return
	}

	response := models.ListEncabulationsResponse{
		Encabulations: encabulations,
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *Handler) UpdateEncabulation(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req models.UpdateEncabulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	enc, err := h.db.UpdateEncabulation(id, req)
	if err != nil {
		h.logger.Error().Err(err).Str("id", id).Msg("Failed to update encabulation")
		h.respondError(w, http.StatusInternalServerError, "DATABASE_ERROR", "Failed to update encabulation")
		return
	}

	if enc == nil {
		h.respondError(w, http.StatusNotFound, "NOT_FOUND", "Encabulation not found")
		return
	}

	h.respondJSON(w, http.StatusOK, enc)
}

func (h *Handler) DeleteEncabulation(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	err := h.db.DeleteEncabulation(id)
	if err != nil {
		h.logger.Error().Err(err).Str("id", id).Msg("Failed to delete encabulation")
		h.respondError(w, http.StatusInternalServerError, "DATABASE_ERROR", "Failed to delete encabulation")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) respondError(w http.ResponseWriter, status int, code, message string) {
	h.respondJSON(w, status, models.ErrorResponse{
		Error:   http.StatusText(status),
		Code:    code,
		Message: message,
	})
}
