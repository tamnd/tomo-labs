package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/analyzer/codex"
)

// cmdCodex reads the local Codex install: the models it can reach and the
// session rollouts it wrote. It is a tap on a tool running under its own real
// subscription, not a benchmark run on the shared free model, so it needs no
// container and no key. It dispatches before lab.New for that reason.
//
//	lab codex models [--json]              list the models the subscription can pick
//	lab codex analyze [rollout] [--json]   summarize a rollout (latest if omitted)
func cmdCodex(args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "models":
		return codexModels(hasFlag(args, "--json"))
	case "analyze", "":
		return codexAnalyze(arg(args, 1), hasFlag(args, "--json"))
	default:
		return fmt.Errorf("usage: lab codex {models|analyze [rollout]} [--json]")
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
func codexAnalyze(path string, asJSON bool) error {
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
	fmt.Println("tokens:")
	fmt.Printf("  input      %8d\n", t.InputTokens)
	fmt.Printf("  cached     %8d\n", t.CachedInputTokens)
	fmt.Printf("  output     %8d\n", t.OutputTokens)
	fmt.Printf("  reasoning  %8d\n", t.ReasoningOutputTokens)
	fmt.Printf("  total      %8d\n", t.TotalTokens)
	if s.Prompt != "" {
		fmt.Printf("prompt:  %s\n", firstLine(s.Prompt, 100))
	}
	return nil
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

func writeJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(b, '\n'))
	return err
}
