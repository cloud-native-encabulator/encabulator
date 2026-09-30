package config_test

import (
	"testing"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/config"
)

func TestStringAndDefaults(t *testing.T) {
	t.Setenv("ENC_TEST_STR", "  value  ")
	if got := config.String("ENC_TEST_STR", "def"); got != "value" {
		t.Errorf("String = %q", got)
	}
	if got := config.String("ENC_TEST_MISSING", "def"); got != "def" {
		t.Errorf("String default = %q", got)
	}
	t.Setenv("ENC_TEST_EMPTY", "   ")
	if got := config.String("ENC_TEST_EMPTY", "def"); got != "def" {
		t.Errorf("blank value should fall back: %q", got)
	}
}

func TestNumbersAndBools(t *testing.T) {
	t.Setenv("ENC_TEST_INT", "42")
	t.Setenv("ENC_TEST_BAD_INT", "forty-two")
	t.Setenv("ENC_TEST_FLOAT", "1.5")
	t.Setenv("ENC_TEST_BOOL", "YES")
	t.Setenv("ENC_TEST_DUR", "250ms")

	if got := config.Int("ENC_TEST_INT", 1); got != 42 {
		t.Errorf("Int = %d", got)
	}
	if got := config.Int("ENC_TEST_BAD_INT", 7); got != 7 {
		t.Errorf("unparseable Int should fall back: %d", got)
	}
	if got := config.Float("ENC_TEST_FLOAT", 0); got != 1.5 {
		t.Errorf("Float = %v", got)
	}
	if !config.Bool("ENC_TEST_BOOL", false) {
		t.Error("Bool(YES) should be true")
	}
	if got := config.Duration("ENC_TEST_DUR", time.Second); got != 250*time.Millisecond {
		t.Errorf("Duration = %v", got)
	}
}

func TestRequire(t *testing.T) {
	if _, err := config.Require("ENC_TEST_ABSENT"); err == nil {
		t.Error("Require should fail for an unset variable")
	}
	t.Setenv("ENC_TEST_PRESENT", "x")
	if v, err := config.Require("ENC_TEST_PRESENT"); err != nil || v != "x" {
		t.Errorf("Require = %q, %v", v, err)
	}
}

func TestURLVarAndServiceURL(t *testing.T) {
	cases := map[string]string{
		"hydrocoptic-datastore":      "HYDROCOPTIC_DATASTORE_URL",
		"differential-girdlespring":  "DIFFERENTIAL_GIRDLESPRING_URL",
		"spurving-bearing-quoter":    "SPURVING_BEARING_QUOTER_URL",
		"unilateral-phase-detractor": "UNILATERAL_PHASE_DETRACTOR_URL",
	}
	for service, want := range cases {
		if got := config.URLVar(service); got != want {
			t.Errorf("URLVar(%q) = %q, want %q", service, got, want)
		}
	}

	t.Setenv("HYDROCOPTIC_DATASTORE_URL", "http://hydrocoptic-datastore:8080/")
	if got := config.ServiceURL("hydrocoptic-datastore"); got != "http://hydrocoptic-datastore:8080" {
		t.Errorf("ServiceURL = %q (trailing slash should be trimmed)", got)
	}
	if got := config.ServiceURL("nofer-trunnion-dispatch"); got != "" {
		t.Errorf("unset ServiceURL = %q", got)
	}
}
