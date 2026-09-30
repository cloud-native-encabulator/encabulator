package ledger_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/ledger"
)

func TestAllowTransitionFollowsTheLifecycle(t *testing.T) {
	cases := []struct {
		cur, next string
		want      bool
		why       string
	}{
		{"", ledger.StatusAccepted, true, "a new order starts anywhere"},
		{ledger.StatusAccepted, ledger.StatusFabricating, true, "forward"},
		{ledger.StatusFabricating, ledger.StatusInspecting, true, "forward"},
		{ledger.StatusInspecting, ledger.StatusDispatched, true, "forward"},
		{ledger.StatusDispatched, ledger.StatusDelivered, true, "forward"},
		{ledger.StatusInspecting, ledger.StatusRefabricating, true, "the documented detour"},
		{ledger.StatusRefabricating, ledger.StatusFabricating, true, "the documented detour"},
		{ledger.StatusInspecting, ledger.StatusScrapped, true, "third failed inspection"},

		{ledger.StatusFabricating, ledger.StatusAccepted, false, "backwards"},
		{ledger.StatusInspecting, ledger.StatusFabricating, false, "only via refabricating"},
		{ledger.StatusDispatched, ledger.StatusInspecting, false, "backwards"},
		{ledger.StatusDelivered, ledger.StatusDispatched, false, "delivered is terminal"},
		{ledger.StatusDelivered, ledger.StatusScrapped, false, "delivered is terminal"},
		{ledger.StatusScrapped, ledger.StatusFabricating, false, "scrapped is terminal"},
		{ledger.StatusFabricating, ledger.StatusFabricating, false, "no self transition"},
		{ledger.StatusFabricating, "invented", false, "unknown status"},
	}
	for _, tc := range cases {
		if got := ledger.AllowTransition(tc.cur, tc.next); got != tc.want {
			t.Errorf("AllowTransition(%q, %q) = %v, want %v (%s)", tc.cur, tc.next, got, tc.want, tc.why)
		}
	}
}

func TestRankAndTerminal(t *testing.T) {
	if ledger.Rank("invented") != 0 {
		t.Error("an unknown status should rank 0")
	}
	if ledger.Rank(ledger.StatusFabricating) != ledger.Rank(ledger.StatusRefabricating) {
		t.Error("refabricating should rank with fabricating")
	}
	for _, s := range []string{ledger.StatusDelivered, ledger.StatusScrapped} {
		if !ledger.IsTerminal(s) {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range []string{ledger.StatusAccepted, ledger.StatusInspecting, ledger.StatusDispatched} {
		if ledger.IsTerminal(s) {
			t.Errorf("%s should not be terminal", s)
		}
	}
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var eventTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func TestInterpretOrderAccepted(t *testing.T) {
	id, up, err := ledger.Interpret(ledger.SubjectOrderAccepted, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "customer": "wendy", "model": "retro", "quantity": 3,
		"options": map[string]bool{"dingleArm": true}, "total": 8247.00, "currency": "USD",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if id != "ord_1" {
		t.Errorf("order id = %q", id)
	}
	if up.Status != ledger.StatusAccepted {
		t.Errorf("status = %q", up.Status)
	}
	if up.Create == nil {
		t.Fatal("order.accepted must be able to create the row")
	}
	if up.Create.Customer != "wendy" || up.Create.Model != "retro" || up.Create.Quantity != 3 {
		t.Errorf("created order = %+v", up.Create)
	}
	if !up.Create.Options.DingleArm {
		t.Error("dingleArm option lost")
	}
	if up.Create.Quote == nil || up.Create.Quote.Total != 8247.00 || up.Create.Quote.Currency != "USD" {
		t.Errorf("quote = %+v", up.Create.Quote)
	}
	if up.Create.CreatedAt != eventTime {
		t.Errorf("createdAt = %v, want the event time", up.Create.CreatedAt)
	}
	if up.Detail == "" {
		t.Error("no timeline detail")
	}
}

func TestInterpretFabricationRequested(t *testing.T) {
	_, up, err := ledger.Interpret(ledger.SubjectFabricationRequested, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "attempt": 2, "model": "turbo", "quantity": 1, "reservationId": "rsv_9",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != ledger.StatusFabricating {
		t.Errorf("status = %q", up.Status)
	}
	if up.Attempt == nil || *up.Attempt != 2 {
		t.Errorf("attempt = %v", up.Attempt)
	}
	if up.Create != nil {
		t.Error("only order.accepted may create an order")
	}
}

func TestInterpretFabricationStage(t *testing.T) {
	_, up, err := ledger.Interpret(ledger.SubjectFabricationStage, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "attempt": 1, "serial": "ENC-turbo-abc-1234",
		"stage": "align-spurving-bearings", "stageIndex": 3, "durationMs": 512,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if up.Stage == nil || *up.Stage != "align-spurving-bearings" {
		t.Errorf("stage = %v", up.Stage)
	}
	if !reflect.DeepEqual(up.Serials, []string{"ENC-turbo-abc-1234"}) {
		t.Errorf("serials = %v", up.Serials)
	}
	if up.Status != ledger.StatusFabricating {
		t.Errorf("status = %q", up.Status)
	}
}

func TestInterpretFabricationCompletedClearsStage(t *testing.T) {
	_, up, err := ledger.Interpret(ledger.SubjectFabricationCompleted, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "attempt": 1, "model": "turbo",
		"serials": []string{"ENC-a", "ENC-b"}, "durationMs": 3200,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != ledger.StatusInspecting {
		t.Errorf("status = %q", up.Status)
	}
	if up.Stage == nil || *up.Stage != "" {
		t.Errorf("completion should clear the stage, got %v", up.Stage)
	}
	if len(up.Serials) != 2 {
		t.Errorf("serials = %v", up.Serials)
	}
}

func TestInterpretInspectionPassedDoesNotAdvanceStatus(t *testing.T) {
	_, up, err := ledger.Interpret(ledger.SubjectInspectionPassed, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "attempt": 1, "serials": []string{"ENC-a"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != "" {
		t.Errorf("inspection.passed set status %q; dispatch owns that transition", up.Status)
	}
	if up.Detail == "" {
		t.Error("inspection.passed should still write a timeline entry")
	}
}

func TestInterpretInspectionFailedSummarizesFindings(t *testing.T) {
	findings := []map[string]any{}
	for i := range 6 {
		findings = append(findings, map[string]any{
			"serial": "ENC-" + string(rune('a'+i)), "stage": "fit-marzlevanes",
			"metric": "repleneration", "score": 4.5, "reason": "spike",
		})
	}
	_, up, err := ledger.Interpret(ledger.SubjectInspectionFailed, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "attempt": 2, "serials": []string{"ENC-a"}, "findings": findings,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != ledger.StatusRefabricating {
		t.Errorf("status = %q", up.Status)
	}
	if !contains(up.Detail, "and 2 more") {
		t.Errorf("detail should truncate a long finding list: %q", up.Detail)
	}
}

func TestInterpretShipmentEvents(t *testing.T) {
	_, up, err := ledger.Interpret(ledger.SubjectShipmentDispatched, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "carrier": "Waneshaft Express", "trackingNumber": "NT0123456789", "etaSeconds": 42,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != ledger.StatusDispatched {
		t.Errorf("status = %q", up.Status)
	}
	if up.Carrier == nil || *up.Carrier != "Waneshaft Express" {
		t.Errorf("carrier = %v", up.Carrier)
	}
	if up.TrackingNumber == nil || *up.TrackingNumber != "NT0123456789" {
		t.Errorf("tracking = %v", up.TrackingNumber)
	}

	_, up, err = ledger.Interpret(ledger.SubjectShipmentDelivered, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "carrier": "Waneshaft Express", "trackingNumber": "NT0123456789",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != ledger.StatusDelivered {
		t.Errorf("status = %q", up.Status)
	}
}

func TestInterpretScrapped(t *testing.T) {
	_, up, err := ledger.Interpret(ledger.SubjectOrderScrapped, eventTime, raw(t, map[string]any{
		"orderId": "ord_1", "attempt": 3, "reason": "persistent side fumbling",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != ledger.StatusScrapped {
		t.Errorf("status = %q", up.Status)
	}
	if !contains(up.Detail, "persistent side fumbling") {
		t.Errorf("detail = %q", up.Detail)
	}
}

func TestInterpretUnknownSubject(t *testing.T) {
	_, _, err := ledger.Interpret("encabulator.something.else", eventTime, raw(t, map[string]any{}))
	var notForLedger ledger.ErrNotForLedger
	if !errors.As(err, &notForLedger) {
		t.Fatalf("err = %v, want ErrNotForLedger", err)
	}
}

func TestInterpretMalformedPayload(t *testing.T) {
	if _, _, err := ledger.Interpret(ledger.SubjectOrderAccepted, eventTime,
		json.RawMessage(`{"quantity": "three"}`)); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestMergeSerials(t *testing.T) {
	cases := []struct {
		existing, incoming, want []string
	}{
		{nil, []string{"a"}, []string{"a"}},
		{[]string{"a"}, []string{"a"}, []string{"a"}},
		{[]string{"a"}, []string{"b", "a", "c"}, []string{"a", "b", "c"}},
		{[]string{"a"}, []string{""}, []string{"a"}},
		{nil, nil, []string{}},
	}
	for _, tc := range cases {
		if got := ledger.MergeSerials(tc.existing, tc.incoming); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("MergeSerials(%v, %v) = %v, want %v", tc.existing, tc.incoming, got, tc.want)
		}
	}
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
