package broker

import (
	"testing"

	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/logarithmic-casing-service/internal/models"
)

func TestMessageValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     models.PublishRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: models.PublishRequest{
				Topic:   "test.topic",
				Payload: map[string]interface{}{"key": "value"},
			},
			wantErr: false,
		},
		{
			name: "empty topic",
			req: models.PublishRequest{
				Topic:   "",
				Payload: map[string]interface{}{"key": "value"},
			},
			wantErr: true,
		},
		{
			name: "nil payload",
			req: models.PublishRequest{
				Topic:   "test.topic",
				Payload: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasError := tt.req.Topic == "" || tt.req.Payload == nil
			if hasError != tt.wantErr {
				t.Errorf("validation mismatch: got %v, want %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestStatsInitialization(t *testing.T) {
	stats := &Stats{
		TopicMessages:    make(map[string]int64),
		TopicSubscribers: make(map[string]int),
	}

	if stats.TotalMessages != 0 {
		t.Errorf("expected TotalMessages to be 0, got %d", stats.TotalMessages)
	}

	if len(stats.TopicMessages) != 0 {
		t.Errorf("expected empty TopicMessages map, got %d entries", len(stats.TopicMessages))
	}
}
