// Package journey describes what a synthetic customer does with the factory,
// and picks one behaviour at a time according to a weighted mix.
package journey

import (
	"fmt"
	"sort"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/rng"
)

// Journey names. These are the labels that appear in metrics and stats.
const (
	Browse = "browse"
	Quote  = "quote"
	Order  = "order"
	Track  = "track"
)

// Names lists every journey, in a stable order.
var Names = []string{Browse, Quote, Order, Track}

// DefaultMix is the weighting from the design document.
func DefaultMix() map[string]int {
	return map[string]int{
		Browse: 40,
		Quote:  25,
		Order:  20,
		Track:  15,
	}
}

// Known reports whether name is a journey the grammeter can run.
func Known(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}

// ValidateMix checks that every key is a known journey, that no weight is
// negative, and that at least one weight is positive.
func ValidateMix(mix map[string]int) error {
	if len(mix) == 0 {
		return fmt.Errorf("mix is empty")
	}
	total := 0
	for name, weight := range mix {
		if !Known(name) {
			return fmt.Errorf("unknown journey %q", name)
		}
		if weight < 0 {
			return fmt.Errorf("journey %q has negative weight %d", name, weight)
		}
		total += weight
	}
	if total == 0 {
		return fmt.Errorf("mix weights sum to zero")
	}
	return nil
}

// Picker selects journeys at random with probability proportional to weight.
type Picker struct {
	names      []string
	cumulative []int
	total      int
	source     rng.Source
}

// NewPicker builds a picker from a mix. Zero-weight journeys are never chosen.
func NewPicker(mix map[string]int, source rng.Source) (*Picker, error) {
	if err := ValidateMix(mix); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(mix))
	for name := range mix {
		if mix[name] > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	p := &Picker{names: names, source: source}
	for _, name := range names {
		p.total += mix[name]
		p.cumulative = append(p.cumulative, p.total)
	}
	return p, nil
}

// Pick returns the next journey name.
func (p *Picker) Pick() string {
	if p.total == 0 {
		return Browse
	}
	roll := p.source.IntN(p.total)
	idx := sort.SearchInts(p.cumulative, roll+1)
	if idx >= len(p.names) {
		idx = len(p.names) - 1
	}
	return p.names[idx]
}
