package models

import "time"

// MetricPoint represents a single time-series data point
type MetricPoint struct {
	Measurement string                 `json:"measurement"`
	Tags        map[string]string      `json:"tags"`
	Fields      map[string]interface{} `json:"fields"`
	Timestamp   time.Time              `json:"timestamp"`
}

// MetricWriteRequest represents a request to write metrics
type MetricWriteRequest struct {
	Points []MetricPoint `json:"points"`
}

// MetricQueryRequest represents a query for time-series data
type MetricQueryRequest struct {
	Measurement string            `json:"measurement"`
	Start       time.Time         `json:"start"`
	End         time.Time         `json:"end"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// MetricQueryResponse represents the response from a query
type MetricQueryResponse struct {
	Points []MetricPoint `json:"points"`
	Count  int           `json:"count"`
}

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
