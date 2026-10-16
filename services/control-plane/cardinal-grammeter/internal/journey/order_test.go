package journey

import (
	"math"
	"testing"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/rng"
)

func TestRandomSpecShares(t *testing.T) {
	source := rng.Seeded(42)
	const n = 200000

	models := map[string]int{}
	dingle := 0
	large := 0
	minQty, maxQty := 99, 0

	for range n {
		spec := RandomSpec(source)
		models[spec.Model]++
		if spec.Options.DingleArm {
			dingle++
		}
		if spec.Quantity > 3 {
			large++
		}
		minQty = min(minQty, spec.Quantity)
		maxQty = max(maxQty, spec.Quantity)
	}

	// 60% turbo, 25% retro, 15% hyper.
	for model, want := range map[string]float64{"turbo": 0.60, "retro": 0.25, "hyper": 0.15} {
		if got := float64(models[model]) / n; math.Abs(got-want) > 0.01 {
			t.Errorf("model %q share = %.3f, want %.3f", model, got, want)
		}
	}
	if len(models) != 3 {
		t.Errorf("saw models %v, want exactly turbo/retro/hyper", models)
	}
	if got := float64(dingle) / n; math.Abs(got-0.30) > 0.01 {
		t.Errorf("dingle arm share = %.3f, want 0.30", got)
	}
	if got := float64(large) / n; math.Abs(got-0.10) > 0.01 {
		t.Errorf("large-order share = %.3f, want 0.10", got)
	}
	if minQty != 1 {
		t.Errorf("minimum quantity = %d, want 1", minQty)
	}
	if maxQty != 8 {
		t.Errorf("maximum quantity = %d, want 8 (the occasional large order)", maxQty)
	}
}

func TestRandomSpecQuantityStaysInRange(t *testing.T) {
	source := rng.Seeded(3)
	for range 50000 {
		spec := RandomSpec(source)
		if spec.Quantity < 1 || spec.Quantity > 10 {
			t.Fatalf("quantity %d is outside the order limits of 1 to 10", spec.Quantity)
		}
	}
}
