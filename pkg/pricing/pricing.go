// Package pricing is the single source of truth for what a model run costs. It
// holds the published per-token rate for every model and provider the lab
// benchmarks, so a run's token counts turn into a dollar figure the same way no
// matter which tool or model produced them.
//
// The rates come from each provider's own pricing page, the official number
// rather than a third party's copy of it:
//
//	OpenAI    https://platform.openai.com/docs/pricing
//	DeepSeek  https://api-docs.deepseek.com/quick_start/pricing
//
// The on-disk shape matches LiteLLM's model_prices_and_context_window.json field
// names (input_cost_per_token, cache_read_input_token_cost, output_cost_per_token,
// litellm_provider), so a fuller table can be dropped in or refreshed from that
// community file without changing any code that reads it. We keep the official
// numbers as the source of truth and use the shared schema only for portability.
package pricing

import (
	_ "embed"
	"encoding/json"
	"io"
	"strings"
)

//go:embed prices.json
var pricesJSON []byte

// Model is one model's published rate, per token in US dollars. The field names
// mirror LiteLLM's schema so the table is refreshable from that file, but only
// the three rates and the provider are load bearing here.
type Model struct {
	Provider        string `json:"litellm_provider"`
	Mode            string `json:"mode"`
	MaxInputTokens  int    `json:"max_input_tokens"`
	MaxOutputTokens int    `json:"max_output_tokens"`

	// InputCost is the fresh (cache-miss) input rate per token. CacheReadCost is
	// what a cached input token is billed at instead, a tenth of InputCost for the
	// gpt-5 family, cheap but not free. OutputCost covers output tokens, reasoning
	// tokens included, since a provider bills reasoning as output.
	InputCost     float64 `json:"input_cost_per_token"`
	CacheReadCost float64 `json:"cache_read_input_token_cost"`
	OutputCost    float64 `json:"output_cost_per_token"`
}

// Usage is the token count a cost is computed over. It is the subset of a
// rollout's token usage that carries a price: total input, how much of that was
// served from cache, and total output.
type Usage struct {
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
}

// Cost is a run's dollar cost broken out by kind, so a cache-heavy run reads as
// cheap without reading as free and the input and output sides stay separate.
type Cost struct {
	InputUSD  float64 // fresh input, billed at the full rate
	CachedUSD float64 // cached input, billed at the discounted read rate
	OutputUSD float64 // output, reasoning included
	TotalUSD  float64
}

// Cost prices a usage at this model's rate. Cached input is split out of the
// input total and billed at the cheaper read rate, so the full-rate input is
// only the uncached remainder.
func (m Model) Cost(u Usage) Cost {
	uncached := u.InputTokens - u.CachedInputTokens
	if uncached < 0 {
		uncached = 0
	}
	c := Cost{
		InputUSD:  float64(uncached) * m.InputCost,
		CachedUSD: float64(u.CachedInputTokens) * m.CacheReadCost,
		OutputUSD: float64(u.OutputTokens) * m.OutputCost,
	}
	c.TotalUSD = c.InputUSD + c.CachedUSD + c.OutputUSD
	return c
}

// Table maps a model name to its rate. It is keyed the way the model reports
// itself, e.g. "gpt-5.6-sol" or "deepseek-chat", with provider prefixes handled
// by Lookup rather than baked into the keys.
type Table map[string]Model

// Load parses a pricing table from a LiteLLM-shaped JSON document. A "sample_spec"
// key, which that file carries as documentation, is dropped so it never looks
// like a real model.
func Load(r io.Reader) (Table, error) {
	var t Table
	if err := json.NewDecoder(r).Decode(&t); err != nil {
		return nil, err
	}
	delete(t, "sample_spec")
	return t, nil
}

// Default is the vendored table, the official rates for the models the lab runs.
func Default() Table {
	t, err := Load(strings.NewReader(string(pricesJSON)))
	if err != nil {
		// The table is embedded and tested, so a parse failure is a build-time bug,
		// not a runtime condition a caller can do anything about.
		panic("pricing: embedded table is invalid: " + err.Error())
	}
	return t
}

// Lookup finds a model's rate by the name it reports. It tries the name as given,
// then the name with any "provider/" prefix stripped, so both "gpt-5.6-sol" and
// "openai/gpt-5.6-sol" resolve to the same entry.
func (t Table) Lookup(name string) (Model, bool) {
	if m, ok := t[name]; ok {
		return m, true
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		if m, ok := t[name[i+1:]]; ok {
			return m, true
		}
	}
	return Model{}, false
}
