package telemetry

import (
	"context"
	"testing"
)

// The no-op path (empty endpoint) is what every other test exercises, so it
// hid a resource-merge error that only surfaced in a cluster. Exercise the real
// path: building the exporter does not dial, so no collector is needed.
func TestSetupWithEndpoint(t *testing.T) {
	shutdown, err := Setup(context.Background(), Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.2.3",
		Endpoint:       "http://127.0.0.1:4318",
	})
	if err != nil {
		t.Fatalf("Setup with an endpoint failed: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Logf("shutdown (no collector listening, expected to be quiet): %v", err)
	}
}

func TestSetupWithoutEndpointIsNoop(t *testing.T) {
	shutdown, err := Setup(context.Background(), Config{ServiceName: "test-service"})
	if err != nil {
		t.Fatalf("no-op Setup failed: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown failed: %v", err)
	}
}
