package journey

import (
	"math"
	"testing"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/rng"
)

func TestDefaultMixMatchesTheDesign(t *testing.T) {
	want := map[string]int{Browse: 40, Quote: 25, Order: 20, Track: 15}
	got := DefaultMix()
	if len(got) != len(want) {
		t.Fatalf("mix has %d entries, want %d", len(got), len(want))
	}
	total := 0
	for k, v := range want {
		if got[k] != v {
			t.Errorf("mix[%q] = %d, want %d", k, got[k], v)
		}
		total += got[k]
	}
	if total != 100 {
		t.Errorf("default weights sum to %d, want 100", total)
	}
}

func TestValidateMix(t *testing.T) {
	cases := []struct {
		name string
		mix  map[string]int
		ok   bool
	}{
		{"default", DefaultMix(), true},
		{"single journey", map[string]int{Order: 1}, true},
		{"zeros allowed alongside a positive", map[string]int{Browse: 0, Order: 3}, true},
		{"empty", map[string]int{}, false},
		{"nil", nil, false},
		{"all zero", map[string]int{Browse: 0}, false},
		{"unknown journey", map[string]int{"sabotage": 1}, false},
		{"negative weight", map[string]int{Browse: -1, Order: 2}, false},
	}
	for _, c := range cases {
		err := ValidateMix(c.mix)
		if (err == nil) != c.ok {
			t.Errorf("%s: ValidateMix err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestPickerHonoursWeights(t *testing.T) {
	picker, err := NewPicker(DefaultMix(), rng.Seeded(7))
	if err != nil {
		t.Fatalf("NewPicker: %v", err)
	}

	const n = 200000
	counts := map[string]int{}
	for range n {
		counts[picker.Pick()]++
	}

	for name, weight := range DefaultMix() {
		want := float64(weight) / 100
		got := float64(counts[name]) / n
		if math.Abs(got-want) > 0.01 {
			t.Errorf("journey %q chosen %.3f of the time, want %.3f", name, got, want)
		}
	}
}

func TestPickerSkipsZeroWeights(t *testing.T) {
	picker, err := NewPicker(map[string]int{Browse: 0, Quote: 0, Order: 1, Track: 3}, rng.Seeded(11))
	if err != nil {
		t.Fatalf("NewPicker: %v", err)
	}
	counts := map[string]int{}
	for range 10000 {
		counts[picker.Pick()]++
	}
	if counts[Browse] != 0 || counts[Quote] != 0 {
		t.Errorf("zero-weight journeys were chosen: %v", counts)
	}
	ratio := float64(counts[Track]) / float64(counts[Order])
	if math.Abs(ratio-3) > 0.2 {
		t.Errorf("track:order ratio = %.2f, want ~3", ratio)
	}
}

func TestPickerIsDeterministicForASeed(t *testing.T) {
	first := sequence(t, 13)
	second := sequence(t, 13)
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("seeded pickers diverged at %d: %q vs %q", i, first[i], second[i])
		}
	}
}

func sequence(t *testing.T, seed uint64) []string {
	t.Helper()
	picker, err := NewPicker(DefaultMix(), rng.Seeded(seed))
	if err != nil {
		t.Fatalf("NewPicker: %v", err)
	}
	out := make([]string, 50)
	for i := range out {
		out[i] = picker.Pick()
	}
	return out
}

func TestNewPickerRejectsBadMix(t *testing.T) {
	if _, err := NewPicker(map[string]int{"nope": 1}, rng.Seeded(1)); err == nil {
		t.Fatal("expected an error for an unknown journey")
	}
}
