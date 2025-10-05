package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/prefabulated-cache/internal/cache"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/prefabulated-cache/internal/models"
	"github.com/rs/zerolog"
)

type Handler struct {
	cache  *cache.RedisCache
	logger zerolog.Logger
}

func NewHandler(c *cache.RedisCache, logger zerolog.Logger) *Handler {
	return &Handler{
		cache:  c,
		logger: logger,
	}
}

func (h *Handler) SetCache(w http.ResponseWriter, r *http.Request) {
	var req models.CacheEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Key == "" {
		h.respondError(w, http.StatusBadRequest, "INVALID_KEY", "Key cannot be empty")
		return
	}

	ttl := time.Duration(req.TTL) * time.Second
	if req.TTL == 0 {
		ttl = 0 // No expiration
	}

	if err := h.cache.Set(req.Key, req.Value, ttl); err != nil {
		h.logger.Error().Err(err).Str("key", req.Key).Msg("Failed to set cache")
		h.respondError(w, http.StatusInternalServerError, "CACHE_ERROR", "Failed to set cache entry")
		return
	}

	h.respondJSON(w, http.StatusCreated, map[string]string{"status": "created", "key": req.Key})
}

func (h *Handler) GetCache(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	value, found, err := h.cache.Get(key)
	if err != nil {
		h.logger.Error().Err(err).Str("key", key).Msg("Failed to get cache")
		h.respondError(w, http.StatusInternalServerError, "CACHE_ERROR", "Failed to get cache entry")
		return
	}

	response := models.CacheGetResponse{
		Key:   key,
		Value: value,
		Found: found,
	}

	if !found {
		w.WriteHeader(http.StatusNotFound)
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *Handler) DeleteCache(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if err := h.cache.Delete(key); err != nil {
		h.logger.Error().Err(err).Str("key", key).Msg("Failed to delete cache")
		h.respondError(w, http.StatusInternalServerError, "CACHE_ERROR", "Failed to delete cache entry")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.cache.GetStats()
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get cache stats")
		h.respondError(w, http.StatusInternalServerError, "CACHE_ERROR", "Failed to get cache stats")
		return
	}

	h.respondJSON(w, http.StatusOK, stats)
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
