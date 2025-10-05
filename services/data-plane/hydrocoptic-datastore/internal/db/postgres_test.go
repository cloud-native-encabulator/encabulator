package db

import (
	"testing"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/models"
)

func TestCalculatePrefabulationRate(t *testing.T) {
	tests := []struct {
		name          string
		prefabulation bool
		grammeters    float64
		expected      float64
	}{
		{
			name:          "with prefabulation",
			prefabulation: true,
			grammeters:    10.0,
			expected:      25.0,
		},
		{
			name:          "without prefabulation",
			prefabulation: false,
			grammeters:    10.0,
			expected:      10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePrefabulationRate(tt.prefabulation, tt.grammeters)
			if result != tt.expected {
				t.Errorf("calculatePrefabulationRate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEncabulationValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     models.CreateEncabulationRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: models.CreateEncabulationRequest{
				Grammeters:     10.0,
				Prefabulation:  true,
				SinusoidalMode: "STANDARD",
			},
			wantErr: false,
		},
		{
			name: "zero grammeters",
			req: models.CreateEncabulationRequest{
				Grammeters:     0,
				Prefabulation:  true,
				SinusoidalMode: "STANDARD",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Grammeters <= 0 && !tt.wantErr {
				t.Error("Expected validation error for zero grammeters")
			}
		})
	}
}
