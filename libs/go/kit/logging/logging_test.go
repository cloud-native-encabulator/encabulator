package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/logging"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("no log line written")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, line)
	}
	return out
}

func TestRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewTo(&buf, "hydrocoptic-datastore", "info")
	log.Info("marzlevanes nominal")

	rec := decode(t, &buf)
	for _, key := range []string{"time", "level", "msg", "service"} {
		if _, ok := rec[key]; !ok {
			t.Errorf("missing required field %q in %v", key, rec)
		}
	}
	if rec["level"] != "info" {
		t.Errorf("level = %v, want info", rec["level"])
	}
	if rec["service"] != "hydrocoptic-datastore" {
		t.Errorf("service = %v", rec["service"])
	}
	// RFC 3339 with milliseconds: 23 characters plus the zone.
	ts, _ := rec["time"].(string)
	if len(ts) < 23 || ts[19] != '.' || ts[23] != 'Z' {
		t.Errorf("time %q is not RFC 3339 with milliseconds", ts)
	}
}

func TestRequestIDFromContext(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewTo(&buf, "svc", "debug")
	ctx := logging.WithRequestID(context.Background(), "req-abc")
	log.InfoContext(ctx, "hello")

	rec := decode(t, &buf)
	if rec["request_id"] != "req-abc" {
		t.Errorf("request_id = %v, want req-abc", rec["request_id"])
	}
	if got := logging.RequestID(ctx); got != "req-abc" {
		t.Errorf("RequestID = %q", got)
	}
}

func TestNoTraceFieldsWithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewTo(&buf, "svc", "info")
	log.InfoContext(context.Background(), "hello")

	rec := decode(t, &buf)
	if _, ok := rec["trace_id"]; ok {
		t.Error("trace_id present with no active span")
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewTo(&buf, "svc", "warn")
	log.Info("suppressed")
	if buf.Len() != 0 {
		t.Errorf("info logged at warn level: %q", buf.String())
	}
	log.Warn("kept")
	if buf.Len() == 0 {
		t.Error("warn suppressed at warn level")
	}
}

func TestWithAttrsKeepsContextDecoration(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewTo(&buf, "svc", "info").With("order_id", "ord_1")
	log.InfoContext(logging.WithRequestID(context.Background(), "r1"), "hello")

	rec := decode(t, &buf)
	if rec["order_id"] != "ord_1" || rec["request_id"] != "r1" {
		t.Errorf("attrs lost through With: %v", rec)
	}
}
