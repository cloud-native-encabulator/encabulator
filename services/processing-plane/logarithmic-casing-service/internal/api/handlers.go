package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/logarithmic-casing-service/internal/broker"
	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/logarithmic-casing-service/internal/models"
	"github.com/rs/zerolog"
)

type Handler struct {
	broker *broker.NATSBroker
	logger zerolog.Logger
}

func NewHandler(b *broker.NATSBroker, logger zerolog.Logger) *Handler {
	return &Handler{
		broker: b,
		logger: logger,
	}
}

func (h *Handler) PublishMessage(w http.ResponseWriter, r *http.Request) {
	var req models.PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Topic == "" {
		h.respondError(w, http.StatusBadRequest, "MISSING_TOPIC", "Topic is required")
		return
	}

	if req.Payload == nil {
		h.respondError(w, http.StatusBadRequest, "MISSING_PAYLOAD", "Payload is required")
		return
	}

	response, err := h.broker.Publish(req.Topic, req.Payload, req.Headers)
	if err != nil {
		h.logger.Error().Err(err).Str("topic", req.Topic).Msg("Failed to publish message")
		h.respondError(w, http.StatusInternalServerError, "PUBLISH_ERROR", "Failed to publish message")
		return
	}

	h.respondJSON(w, http.StatusCreated, response)
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats := h.broker.GetStats()
	h.respondJSON(w, http.StatusOK, stats)
}

func (h *Handler) GetTopicStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	topic := vars["topic"]

	stats := h.broker.GetStats()

	for _, topicStats := range stats.Topics {
		if topicStats.Topic == topic {
			h.respondJSON(w, http.StatusOK, topicStats)
			return
		}
	}

	h.respondError(w, http.StatusNotFound, "TOPIC_NOT_FOUND", "Topic not found")
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
