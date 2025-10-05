package models

import "time"

// Encabulation represents a core encabulation record in the system
type Encabulation struct {
	ID                 string    `json:"id"`
	Grammeters         float64   `json:"grammeters"`
	Prefabulation      bool      `json:"prefabulation"`
	SinusoidalMode     string    `json:"sinusoidalMode"`
	EncabulationFactor float64   `json:"encabulationFactor"`
	DingleArmPosition  float64   `json:"dingleArmPosition"`
	PrefabulationRate  float64   `json:"prefabulationRate"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// CreateEncabulationRequest represents the request to create an encabulation
type CreateEncabulationRequest struct {
	Grammeters     float64 `json:"grammeters"`
	Prefabulation  bool    `json:"prefabulation"`
	SinusoidalMode string  `json:"sinusoidalMode"`
}

// UpdateEncabulationRequest represents the request to update an encabulation
type UpdateEncabulationRequest struct {
	Grammeters     *float64 `json:"grammeters,omitempty"`
	Prefabulation  *bool    `json:"prefabulation,omitempty"`
	SinusoidalMode *string  `json:"sinusoidalMode,omitempty"`
}

// ListEncabulationsResponse represents a paginated list of encabulations
type ListEncabulationsResponse struct {
	Encabulations []Encabulation `json:"encabulations"`
	Total         int            `json:"total"`
	Page          int            `json:"page"`
	PageSize      int            `json:"pageSize"`
}

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
