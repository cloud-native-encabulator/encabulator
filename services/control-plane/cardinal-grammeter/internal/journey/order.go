package journey

import "github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/rng"

// Options mirrors the order and quote option block.
type Options struct {
	DingleArm bool `json:"dingleArm"`
}

// Spec is the body of a quote or order request.
type Spec struct {
	Model    string  `json:"model"`
	Quantity int     `json:"quantity"`
	Options  Options `json:"options"`
}

// Model selection weights: 60% turbo, 25% retro, 15% hyper.
const (
	turboShare = 0.60
	retroShare = 0.85 // cumulative
)

// largeOrderShare is how often a customer asks for more than three units.
const largeOrderShare = 0.10

// dingleArmShare is how often a customer pays for the drawn reciprocation dingle arm.
const dingleArmShare = 0.30

// RandomSpec shapes a plausible customer request.
func RandomSpec(source rng.Source) Spec {
	var model string
	switch roll := source.Float64(); {
	case roll < turboShare:
		model = "turbo"
	case roll < retroShare:
		model = "retro"
	default:
		model = "hyper"
	}

	// Quantity 1 to 3, occasionally up to 8.
	quantity := 1 + source.IntN(3)
	if source.Float64() < largeOrderShare {
		quantity = 4 + source.IntN(5)
	}

	return Spec{
		Model:    model,
		Quantity: quantity,
		Options:  Options{DingleArm: source.Float64() < dingleArmShare},
	}
}
