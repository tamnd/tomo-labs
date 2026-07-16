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
// A model served on a free tier is recorded at its actual billed rate. A free
// variant with a paid twin carries that twin's published rate as a list-price
// reference (deepseek-v4-flash-free mirrors deepseek-v4-flash); a free model
// with no paid twin is billed nothing, so its rates are zero and it reports an
// explicit $0.00 rather than an unpriced gap. Either way the token breakdown is
// what carries the leaner-run comparison across the free roster.
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
	// gpt-5 family, cheap but not free. CacheWriteCost is what writing a token into
	// the cache costs, which Anthropic bills as a premium over fresh input (a quarter
	// more for a 5-minute entry); a provider without a distinct write tier leaves it
	// zero and a written token falls back to the plain input rate. OutputCost covers
	// output tokens, reasoning tokens included, since a provider bills reasoning as
	// output.
	InputCost      float64 `json:"input_cost_per_token"`
	CacheReadCost  float64 `json:"cache_read_input_token_cost"`
	CacheWriteCost float64 `json:"cache_creation_input_token_cost"`
	OutputCost     float64 `json:"output_cost_per_token"`
}

// Usage is the token count a cost is computed over: the subset of a run's token
// usage that carries a price. The three input kinds are disjoint, not nested, so
// each is billed at its own rate without any subtraction. A provider that reports
// input as a total with cache as a subset (the gpt-5 family) splits it before
// filling this in; a provider that already reports the kinds apart (Anthropic,
// which names fresh input, cache read, and cache creation separately) maps them
// straight across.
type Usage struct {
	InputTokens       int // fresh, cache-miss input, billed at the full input rate
	CachedInputTokens int // input served from cache (a read), billed at the read rate
	CacheWriteTokens  int // input written into the cache (creation), billed at the write rate
	OutputTokens      int // output, reasoning included
}

// Cost is a run's dollar cost broken out by kind, so a cache-heavy run reads as
// cheap without reading as free and the input and output sides stay separate.
type Cost struct {
	InputUSD      float64 // fresh input, billed at the full rate
	CachedUSD     float64 // cached input read, billed at the discounted read rate
	CacheWriteUSD float64 // input written to cache, billed at the write rate
	OutputUSD     float64 // output, reasoning included
	TotalUSD      float64
}

// Cost prices a usage at this model's rate. The three input kinds are disjoint,
// so each is multiplied by its own rate and summed with output. A model with no
// distinct cache-write tier bills a written token at the plain input rate, which
// is how the gpt-5 family and deepseek work, so the fallback keeps their cost
// unchanged while Anthropic's write premium is honored where it is published.
func (m Model) Cost(u Usage) Cost {
	writeRate := m.CacheWriteCost
	if writeRate == 0 {
		writeRate = m.InputCost
	}
	c := Cost{
		InputUSD:      float64(u.InputTokens) * m.InputCost,
		CachedUSD:     float64(u.CachedInputTokens) * m.CacheReadCost,
		CacheWriteUSD: float64(u.CacheWriteTokens) * writeRate,
		OutputUSD:     float64(u.OutputTokens) * m.OutputCost,
	}
	c.TotalUSD = c.InputUSD + c.CachedUSD + c.CacheWriteUSD + c.OutputUSD
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
