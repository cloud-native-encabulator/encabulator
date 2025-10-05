package models

import "time"

// CacheEntry represents a cache entry request
type CacheEntry struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
	TTL   int         `json:"ttl"` // TTL in seconds
}

// CacheGetResponse represents a cache get response
type CacheGetResponse struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
	Found bool        `json:"found"`
}

// CacheStats represents cache statistics
type CacheStats struct {
	TotalKeys   int64     `json:"totalKeys"`
	UsedMemory  string    `json:"usedMemory"`
	HitRate     float64   `json:"hitRate"`
	ConnectedAt time.Time `json:"connectedAt"`
}

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
