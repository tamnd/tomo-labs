package claude

import (
	"testing"

	"github.com/tamnd/tomo-labs/pkg/pricing"
)

func TestCostDisjointTiers(t *testing.T) {
	s := parseSample(t) // one opus turn: 100 fresh, 50 write, 900 read, 30 output
	sum := s.Summarize()
	rate, cost, ok := sum.Cost(pricing.Default())
	if !ok {
		t.Fatal("Cost ok = false, want a rate for claude-opus-4-8")
	}
	// The three input kinds are billed apart, no subtraction: fresh at input rate,
	// read at the cheap read rate, write at the premium write rate.
	wantInput := 100 * rate.InputCost
	wantRead := 900 * rate.CacheReadCost
	wantWrite := 50 * rate.CacheWriteCost
	wantOut := 30 * rate.OutputCost
	if !approx(cost.InputUSD, wantInput) {
		t.Errorf("input = %v, want %v", cost.InputUSD, wantInput)
	}
	if !approx(cost.CachedUSD, wantRead) {
		t.Errorf("cache read = %v, want %v", cost.CachedUSD, wantRead)
	}
	if !approx(cost.CacheWriteUSD, wantWrite) {
		t.Errorf("cache write = %v, want %v", cost.CacheWriteUSD, wantWrite)
	}
	if !approx(cost.TotalUSD, wantInput+wantRead+wantWrite+wantOut) {
		t.Errorf("total = %v, want the sum of the four tiers", cost.TotalUSD)
	}
	// The write tier is a real premium over fresh input for Anthropic, so a written
	// token must cost more than a fresh one.
	if rate.CacheWriteCost <= rate.InputCost {
		t.Errorf("opus write rate %v should exceed input rate %v", rate.CacheWriteCost, rate.InputCost)
	}
}

func TestLookupModelTrimsDateSuffix(t *testing.T) {
	table := pricing.Default()
	// A dated snapshot must resolve to the same rate as the bare alias.
	dated, okDated := lookupModel(table, "claude-haiku-4-5-20251001")
	bare, okBare := lookupModel(table, "claude-haiku-4-5")
	if !okDated || !okBare {
		t.Fatalf("lookup dated=%v bare=%v, want both found", okDated, okBare)
	}
	if dated.InputCost != bare.InputCost {
		t.Errorf("dated rate %v != bare rate %v", dated.InputCost, bare.InputCost)
	}
}

func TestTrimDateSuffix(t *testing.T) {
	cases := map[string]string{
		"claude-haiku-4-5-20251001": "claude-haiku-4-5",
		"claude-opus-4-8":           "claude-opus-4-8", // no date, unchanged
		"gpt-5.6-sol":               "gpt-5.6-sol",
	}
	for in, want := range cases {
		if got := trimDateSuffix(in); got != want {
			t.Errorf("trimDateSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToolNamesMostUsedFirst(t *testing.T) {
	sum := Summary{ByTool: map[string]int{"Bash": 5, "Read": 2, "Edit": 2}}
	got := sum.ToolNames()
	if len(got) != 3 || got[0] != "Bash" {
		t.Fatalf("names = %v, want Bash first", got)
	}
	// Ties break by name, so Edit precedes Read.
	if got[1] != "Edit" || got[2] != "Read" {
		t.Errorf("tie order = %v, want Edit before Read", got[1:])
	}
}

func approx(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-12
}
