package pricing

import (
	"math"
	"strings"
	"testing"
)

// The embedded table parses and carries the models the lab runs, at their
// official rates. gpt-5.6-sol is the flagship the codex subscription can pick.
func TestDefaultTable(t *testing.T) {
	tbl := Default()
	sol, ok := tbl.Lookup("gpt-5.6-sol")
	if !ok {
		t.Fatal("gpt-5.6-sol missing from the default table")
	}
	if sol.InputCost != 5e-06 || sol.CacheReadCost != 5e-07 || sol.OutputCost != 3e-05 {
		t.Errorf("gpt-5.6-sol rate = %+v, want official 5/0.5/30 per 1M", sol)
	}
	if sol.Provider != "openai" {
		t.Errorf("gpt-5.6-sol provider = %q, want openai", sol.Provider)
	}
	// The cached read rate is a tenth of fresh input for the gpt-5 family: cheap,
	// not free.
	if !close(sol.CacheReadCost*10, sol.InputCost) {
		t.Errorf("cache read %v is not a tenth of input %v", sol.CacheReadCost, sol.InputCost)
	}
}

// Lookup resolves a name with or without its provider prefix, so both the bare
// name a model reports and a "provider/name" form hit the same entry.
func TestLookupStripsProviderPrefix(t *testing.T) {
	tbl := Default()
	bare, ok1 := tbl.Lookup("deepseek-chat")
	pref, ok2 := tbl.Lookup("deepseek/deepseek-chat")
	if !ok1 || !ok2 {
		t.Fatalf("lookups failed: bare=%v prefixed=%v", ok1, ok2)
	}
	if bare != pref {
		t.Errorf("prefixed lookup gave a different rate: %+v vs %+v", pref, bare)
	}
	if _, ok := tbl.Lookup("no-such-model"); ok {
		t.Error("unknown model should not resolve")
	}
}

// Cost bills each disjoint input kind at its own rate and keeps output its own
// line. The caller passes fresh input already split from cached, so there is no
// subtraction inside.
func TestCost(t *testing.T) {
	m := Model{InputCost: 5e-06, CacheReadCost: 5e-07, OutputCost: 3e-05}
	c := m.Cost(Usage{InputTokens: 100, CachedInputTokens: 900, OutputTokens: 100})
	// 100 fresh * 5e-6 = 5e-4, 900 cached * 5e-7 = 4.5e-4, 100 out * 3e-5 = 3e-3.
	wantIn, wantCache, wantOut := 5e-04, 4.5e-04, 3e-03
	if !close(c.InputUSD, wantIn) || !close(c.CachedUSD, wantCache) || !close(c.OutputUSD, wantOut) {
		t.Errorf("cost = %+v, want in %v cache %v out %v", c, wantIn, wantCache, wantOut)
	}
	if !close(c.TotalUSD, wantIn+wantCache+wantOut) {
		t.Errorf("total %v != sum of parts", c.TotalUSD)
	}
}

// A model with a distinct cache-write tier (Anthropic) bills written tokens at
// that rate; a model without one (the gpt-5 family) falls back to the plain input
// rate, so a written token is never billed as free.
func TestCostCacheWrite(t *testing.T) {
	// Anthropic-shaped: write tier is a quarter more than fresh input.
	claude := Model{InputCost: 1.5e-05, CacheReadCost: 1.5e-06, CacheWriteCost: 1.875e-05, OutputCost: 7.5e-05}
	c := claude.Cost(Usage{InputTokens: 100, CachedInputTokens: 1000, CacheWriteTokens: 200, OutputTokens: 50})
	wantIn := 100 * 1.5e-05
	wantCache := 1000 * 1.5e-06
	wantWrite := 200 * 1.875e-05
	wantOut := 50 * 7.5e-05
	if !close(c.InputUSD, wantIn) || !close(c.CachedUSD, wantCache) || !close(c.CacheWriteUSD, wantWrite) || !close(c.OutputUSD, wantOut) {
		t.Errorf("claude cost = %+v", c)
	}
	if !close(c.TotalUSD, wantIn+wantCache+wantWrite+wantOut) {
		t.Errorf("total %v != sum of parts", c.TotalUSD)
	}
	// No write tier: a written token bills at the input rate, not zero.
	gpt := Model{InputCost: 5e-06, CacheReadCost: 5e-07, OutputCost: 3e-05}
	g := gpt.Cost(Usage{CacheWriteTokens: 100})
	if !close(g.CacheWriteUSD, 100*5e-06) {
		t.Errorf("fallback write cost = %v, want input rate", g.CacheWriteUSD)
	}
}

// Load drops the sample_spec documentation key so it never reads as a model.
func TestLoadDropsSampleSpec(t *testing.T) {
	tbl, err := Load(strings.NewReader(`{"sample_spec":{"litellm_provider":"x"},"gpt-5.4":{"input_cost_per_token":2.5e-06}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tbl["sample_spec"]; ok {
		t.Error("sample_spec should be dropped")
	}
	if _, ok := tbl["gpt-5.4"]; !ok {
		t.Error("real model should survive")
	}
}

func close(a, b float64) bool { return math.Abs(a-b) < 1e-12 }
