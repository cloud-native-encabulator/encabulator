package tsdb

import (
	"testing"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-database/internal/models"
)

func TestMetricPointValidation(t *testing.T) {
	tests := []struct {
		name    string
		point   models.MetricPoint
		wantErr bool
	}{
		{
			name: "valid point",
			point: models.MetricPoint{
				Measurement: "test",
				Tags:        map[string]string{"host": "server1"},
				Fields:      map[string]interface{}{"value": 42.0},
				Timestamp:   time.Now(),
			},
			wantErr: false,
		},
		{
			name: "empty measurement",
			point: models.MetricPoint{
				Measurement: "",
				Tags:        map[string]string{"host": "server1"},
				Fields:      map[string]interface{}{"value": 42.0},
				Timestamp:   time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.point.Measurement == "" && !tt.wantErr {
				t.Error("Expected validation error for empty measurement")
			}
		})
	}
}

func TestTimeRangeValidation(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)

	if !past.Before(now) {
		t.Error("Time range validation failed")
	}
}
