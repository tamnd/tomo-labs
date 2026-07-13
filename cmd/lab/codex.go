package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/analyzer/codex"
	"github.com/tamnd/tomo-labs/pkg/pricing"
)

// cmdCodex reads the local Codex install: the models it can reach and the
// session rollouts it wrote. It is a tap on a tool running under its own real
// subscription, not a benchmark run on the shared free model, so it needs no
// container and no key. It dispatches before lab.New for that reason.
//
//	lab codex models [--json]                      list the models the subscription can pick
//	lab codex analyze [rollout] [--patch] [--json] summarize a rollout (latest if omitted)
func cmdCodex(args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "models":
		return codexModels(hasFlag(args, "--json"))
	case "analyze", "":
		return codexAnalyze(arg(args, 1), hasFlag(args, "--json"), hasFlag(args, "--patch"))
	default:
		return fmt.Errorf("usage: lab codex {models|analyze [rollout] [--patch]} [--json]")
	}
}

// codexModels prints the models the subscription can select, best rank first,
// with their reasoning efforts and context window, read from the models cache
// Codex keeps on disk. It is how a run picks a model and effort without hard
// coding the roster, which shifts as the subscription gains new gpt-5.x tiers.
func codexModels(asJSON bool) error {
	path := codex.CatalogPath(codex.Home())
	cat, err := codex.ParseCatalogFile(path)
	if err != nil {
		return err
	}
	sel := cat.Selectable()
	if asJSON {
		return writeJSON(sel)
	}
	if len(sel) == 0 {
		fmt.Printf("no selectable models in %s\n", path)
		return nil
	}
	fmt.Printf("codex models (%s, client %s):\n", path, cat.ClientVersion)
	for _, m := range sel {
		def := m.DefaultEffort
		if def == "" {
			def = "-"
		}
		fmt.Printf("  %-16s efforts=%-32s default=%-7s context=%d\n",
			m.Slug, strings.Join(m.Efforts, ",")+" ", def, m.ContextWindow)
	}
	return nil
}

// codexAnalyze summarizes one rollout: which model and effort ran it, how it
// converged, and what it spent broken down by kind. The token detail is the
// point: Codex counts cached input and reasoning output apart from plain input
// and output, so a cache-heavy or a reasoning-heavy run reads differently from a
// lean one, and that is exactly what a fair cost comparison against tomo needs.
func codexAnalyze(path string, asJSON, showPatch bool) error {
	if path == "" {
		p, ok, err := codex.LatestRollout(codex.Home())
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no rollouts under %s", codex.Home())
		}
		path = p
	}
	r, err := codex.ParseRolloutFile(path)
	if err != nil {
		return err
	}
	s := r.Summarize()
	if asJSON {
		return writeJSON(s)
	}

	fmt.Printf("rollout: %s\n", path)
	fmt.Printf("session: %s  cli %s\n", s.SessionID, s.CLIVersion)
	fmt.Printf("models:  %s\n", modelList(s.Models))
	fmt.Printf("cwd:     %s\n", s.Cwd)
	fmt.Printf("turns=%d tool_calls=%d writes=%d\n", s.Turns, s.ToolCalls, s.Writes)
	if len(s.ByTool) > 0 {
		fmt.Printf("by tool: %s\n", byToolLine(s.ByTool))
	}
	fmt.Printf("outcome: %s wall=%dms\n", outcome(s), s.WallMs)

	t := s.Tokens
	uncached := t.InputTokens - t.CachedInputTokens
	if uncached < 0 {
		uncached = 0
	}
	fmt.Println("tokens:")
	fmt.Printf("  input   %9d  (uncached %d + cached %d, %s cache hit)\n",
		t.InputTokens, uncached, t.CachedInputTokens, hitRate(t.CachedInputTokens, t.InputTokens))
	fmt.Printf("  output  %9d  (reasoning %d of it)\n", t.OutputTokens, t.ReasoningOutputTokens)
	fmt.Printf("  total   %9d\n", t.TotalTokens)

	// Turn the tokens into a dollar figure at the model's published rate, so a run
	// on a subscription (which is not metered per token) still reads as what the
	// same work would cost on the metered API, and cached input reads as cheap but
	// not free. The rate comes from the shared pricing table, the single source of
	// truth across every model and provider the lab compares.
	printCost(s, t)
	if s.Prompt != "" {
		fmt.Printf("prompt:  %s\n", firstLine(s.Prompt, 100))
	}

	// The patches are the part worth learning from: the exact source change the
	// run made, which read next to what tomo did on the same task is how a real
	// fix teaches a better one. Off by default so the summary stays short.
	if showPatch {
		patches := r.Patches()
		if len(patches) == 0 {
			fmt.Println("patches: (none)")
		}
		for i, p := range patches {
			fmt.Printf("--- patch %d: %s ---\n", i+1, patchFileList(p))
			fmt.Println(strings.TrimRight(p.Body, "\n"))
		}
	}
	return nil
}

// printCost prices a run at its model's published rate and prints the dollar
// breakdown, input, cached, and output kept apart. The model is the last one the
// run used, since a run that switched settled there. When the table has no rate
// for it, we say so plainly rather than invent a number, and note that a
// subscription run is not itself metered per token, so this is the equivalent API
// list price, not a bill.
func printCost(s codex.Summary, t codex.TokenUsage) {
	model := ""
	if n := len(s.Models); n > 0 {
		model = s.Models[n-1].Model
	}
	rate, ok := pricing.Default().Lookup(model)
	if !ok {
		fmt.Printf("  cost      (no published rate for %q, tokens only)\n", model)
		return
	}
	// Codex reports input as a total with cached input a subset of it, so pass the
	// fresh remainder as the full-rate input: pricing.Usage kinds are disjoint.
	uncached := t.InputTokens - t.CachedInputTokens
	if uncached < 0 {
		uncached = 0
	}
	c := rate.Cost(pricing.Usage{
		InputTokens:       uncached,
		CachedInputTokens: t.CachedInputTokens,
		OutputTokens:      t.OutputTokens,
	})
	fmt.Printf("cost (%s API list price, a subscription run is not metered per token):\n", model)
	fmt.Printf("  input   %s  (uncached)\n", usd(c.InputUSD))
	fmt.Printf("  cached  %s  (cached input, billed at the discounted read rate)\n", usd(c.CachedUSD))
	fmt.Printf("  output  %s\n", usd(c.OutputUSD))
	fmt.Printf("  total   %s\n", usd(c.TotalUSD))
}

// usd renders a dollar amount at a fixed width, in cents when it is small enough
// that dollars would print as $0.00, so a lean run still shows a real number.
func usd(v float64) string {
	if v < 0.01 && v > 0 {
		return fmt.Sprintf("%.3f¢", v*100)
	}
	return fmt.Sprintf("$%.4f", v)
}

// patchFileList renders a patch's files with their op, e.g. "update xferfcn.py",
// so the header says what changed before the diff prints.
func patchFileList(p codex.Patch) string {
	if len(p.Files) == 0 {
		return "(no file markers)"
	}
	parts := make([]string, len(p.Files))
	for i, f := range p.Files {
		op := "edit"
		if i < len(p.Ops) {
			op = p.Ops[i]
		}
		parts[i] = op + " " + f
	}
	return strings.Join(parts, ", ")
}

func modelList(ms []codex.ModelUse) string {
	if len(ms) == 0 {
		return "(none)"
	}
	parts := make([]string, len(ms))
	for i, m := range ms {
		if m.Effort != "" {
			parts[i] = m.Model + "/" + m.Effort
		} else {
			parts[i] = m.Model
		}
	}
	return strings.Join(parts, ", ")
}

// byToolLine renders the per-tool call counts most-used first, so the shape of a
// run reads at a glance.
func byToolLine(by map[string]int) string {
	type kv struct {
		name string
		n    int
	}
	pairs := make([]kv, 0, len(by))
	for k, v := range by {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].name < pairs[j].name
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s=%d", p.name, p.n)
	}
	return strings.Join(parts, " ")
}

func outcome(s codex.Summary) string {
	switch {
	case s.Complete:
		return "complete"
	case s.Aborted:
		return "aborted"
	default:
		return "unknown"
	}
}

// firstLine trims a prompt to its first line and caps the length, so a long
// issue body prints as a single readable line.
func firstLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// hitRate renders the share of prompt tokens served from cache as a percent, the
// lever that decides how much of a large input a run actually paid full rate for.
func hitRate(cached, input int) string {
	if input <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(cached)/float64(input))
}

func writeJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(b, '\n'))
	return err
}
