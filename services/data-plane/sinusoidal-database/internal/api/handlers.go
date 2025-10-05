package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-database/internal/models"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-database/internal/tsdb"
	"github.com/rs/zerolog"
)

type Handler struct {
	db     *tsdb.InfluxDB
	logger zerolog.Logger
}

func NewHandler(database *tsdb.InfluxDB, logger zerolog.Logger) *Handler {
	return &Handler{
		db:     database,
		logger: logger,
	}
}

func (h *Handler) WriteMetrics(w http.ResponseWriter, r *http.Request) {
	var req models.MetricWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if len(req.Points) == 0 {
		h.respondError(w, http.StatusBadRequest, "EMPTY_POINTS", "No points provided")
		return
	}

	// Set timestamps if not provided
	for i := range req.Points {
		if req.Points[i].Timestamp.IsZero() {
			req.Points[i].Timestamp = time.Now()
		}
	}

	if err := h.db.WritePoints(req.Points); err != nil {
		h.logger.Error().Err(err).Msg("Failed to write metrics")
		h.respondError(w, http.StatusInternalServerError, "WRITE_ERROR", "Failed to write metrics")
		return
	}

	h.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"status": "created",
		"count":  len(req.Points),
	})
}

func (h *Handler) QueryMetrics(w http.ResponseWriter, r *http.Request) {
	var req models.MetricQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Measurement == "" {
		h.respondError(w, http.StatusBadRequest, "MISSING_MEASUREMENT", "Measurement is required")
		return
	}

	// Default time range if not provided
	if req.Start.IsZero() {
		req.Start = time.Now().Add(-1 * time.Hour)
	}
	if req.End.IsZero() {
		req.End = time.Now()
	}

	points, err := h.db.QueryRange(req.Measurement, req.Start, req.End, req.Tags)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to query metrics")
		h.respondError(w, http.StatusInternalServerError, "QUERY_ERROR", "Failed to query metrics")
		return
	}

	response := models.MetricQueryResponse{
		Points: points,
		Count:  len(points),
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *Handler) QueryMetricsRange(w http.ResponseWriter, r *http.Request) {
	measurement := r.URL.Query().Get("measurement")
	if measurement == "" {
		h.respondError(w, http.StatusBadRequest, "MISSING_MEASUREMENT", "Measurement parameter is required")
		return
	}

	// Parse time range
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	var start, end time.Time
	var err error

	if startStr != "" {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			h.respondError(w, http.StatusBadRequest, "INVALID_START_TIME", "Invalid start time format")
			return
		}
	} else {
		start = time.Now().Add(-1 * time.Hour)
	}

	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			h.respondError(w, http.StatusBadRequest, "INVALID_END_TIME", "Invalid end time format")
			return
		}
	} else {
		end = time.Now()
	}

	points, err := h.db.QueryRange(measurement, start, end, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to query metrics")
		h.respondError(w, http.StatusInternalServerError, "QUERY_ERROR", "Failed to query metrics")
		return
	}

	response := models.MetricQueryResponse{
		Points: points,
		Count:  len(points),
	}

	h.respondJSON(w, http.StatusOK, response)
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
