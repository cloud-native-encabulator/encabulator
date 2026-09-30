package catalog

import (
	"slices"
	"testing"
)

func TestServiceCatalogMatchesTheDesign(t *testing.T) {
	if len(Services) != 15 {
		t.Fatalf("catalog holds %d services, want the 15 of design section 2", len(Services))
	}
	seen := map[string]bool{}
	for _, name := range Services {
		if seen[name] {
			t.Errorf("service %q appears twice", name)
		}
		seen[name] = true
	}
	for _, want := range []string{
		"dingle-arm-gateway", "panametric-fam-processor", "spurving-bearing-quoter",
		"logarithmic-casing-line", "nofer-trunnion-dispatch", "hydrocoptic-datastore",
		"prefabulated-amulite-depot", "sinusoidal-recorder", "unilateral-phase-detractor",
		"differential-girdlespring", "side-fumbling-detector",
		"encabulation-control-console", "cardinal-grammeter",
		"malaise-injection-engine", "demo-scenario-orchestrator",
	} {
		if !seen[want] {
			t.Errorf("service %q is missing", want)
		}
	}
}

// Design section 7: the console, grammeter, engine and orchestrator do not apply
// malaise to themselves.
func TestAppLevelExcludesTheControlPlane(t *testing.T) {
	controlPlane := []string{
		"encabulation-control-console", "cardinal-grammeter",
		"malaise-injection-engine", "demo-scenario-orchestrator",
	}
	for _, name := range controlPlane {
		if AppLevel(name) {
			t.Errorf("%q must not apply malaise to itself", name)
		}
	}
	if len(AppLevelServices) != len(Services)-len(controlPlane) {
		t.Errorf("app-level services = %d, want %d", len(AppLevelServices), len(Services)-len(controlPlane))
	}
	for _, name := range AppLevelServices {
		if !Known(name) {
			t.Errorf("app-level service %q is not in the catalog", name)
		}
		if slices.Contains(controlPlane, name) {
			t.Errorf("%q is control plane and should not be app-level", name)
		}
	}
}

func TestKnown(t *testing.T) {
	if !Known("dingle-arm-gateway") {
		t.Error("the gateway is in the catalog")
	}
	// Backing stores are not services; Kubernetes-level malaise reaches them by
	// label, not by catalog membership.
	for _, name := range []string{"postgres", "valkey", "nats", "influxdb", "", "Dingle-Arm-Gateway"} {
		if Known(name) {
			t.Errorf("%q should not be in the service catalog", name)
		}
		if AppLevel(name) {
			t.Errorf("%q should not be app-level", name)
		}
	}
}
