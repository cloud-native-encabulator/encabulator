// Package catalog knows the names of the things that can be made to suffer.
package catalog

import "slices"

// Services is the service catalog of design section 2, in topology order. A
// malaise of any kind may target one of these by name.
var Services = []string{
	"encabulation-control-console",
	"cardinal-grammeter",
	"malaise-injection-engine",
	"demo-scenario-orchestrator",
	"dingle-arm-gateway",
	"panametric-fam-processor",
	"spurving-bearing-quoter",
	"logarithmic-casing-line",
	"nofer-trunnion-dispatch",
	"hydrocoptic-datastore",
	"prefabulated-amulite-depot",
	"sinusoidal-recorder",
	"unilateral-phase-detractor",
	"differential-girdlespring",
	"side-fumbling-detector",
}

// AppLevelServices are the services that run the malaise middleware and so can
// honour app-level directives. The control plane does not inject malaise into
// itself (design section 7).
var AppLevelServices = []string{
	"dingle-arm-gateway",
	"panametric-fam-processor",
	"spurving-bearing-quoter",
	"logarithmic-casing-line",
	"nofer-trunnion-dispatch",
	"hydrocoptic-datastore",
	"prefabulated-amulite-depot",
	"sinusoidal-recorder",
	"unilateral-phase-detractor",
	"differential-girdlespring",
	"side-fumbling-detector",
}

// Known reports whether name is a service in the catalog.
func Known(name string) bool { return slices.Contains(Services, name) }

// AppLevel reports whether name is a service that applies malaise to itself.
func AppLevel(name string) bool { return slices.Contains(AppLevelServices, name) }
