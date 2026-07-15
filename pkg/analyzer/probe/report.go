package probe

import (
	"fmt"
	"io"
	"sort"
)

// WriteReport renders a Report as a concrete, human-first summary: the headline
// re-send ratio, the cost of the growing transcript, the biggest input jumps, and
// the round-by-round curve. summary is the optional sibling summary.json, used
// only for the task/engine/grade header.
func WriteReport(w io.Writer, rep Report, summary map[string]any) {
	if summary != nil {
		fmt.Fprintf(w, "sim trace: %v | %v | --engine %v\n",
			summary["task"], summary["model"], summary["engine"])
		grade := "ungraded"
		if g, _ := summary["graded"].(bool); g {
			if p, _ := summary["passed"].(bool); p {
				grade = "PASS"
			} else {
				grade = "FAIL"
			}
		}
		fmt.Fprintf(w, "verdict %s", grade)
		if r, ok := summary["check_reason"].(string); ok && r != "" {
			fmt.Fprintf(w, " (%s)", r)
		}
		fmt.Fprintln(w)
		// A run that edited nothing is the sharpest convergence tell: it investigated
		// forever and never committed a change.
		if edited, ok := summary["edited_files"]; ok && (edited == nil) {
			fmt.Fprintln(w, "NOTE: the run made zero edits; every token bought investigation, not a fix")
		}
	}

	fmt.Fprintf(w, "\nrounds %d  tool calls %d  wall %.0fs\n", len(rep.Rounds), rep.TotalTools, float64(rep.WallMs)/1000)

	fmt.Fprintln(w, "\nTokens")
	fmt.Fprintf(w, "  input total  %s\n", comma(rep.TotalInput))
	fmt.Fprintf(w, "  output total %s\n", comma(rep.TotalOut))
	fmt.Fprintf(w, "  re-send ratio %.0f:1  (input tokens paid per output token)\n", rep.ResendRatio())
	if rep.LastInput > 0 && rep.FirstInput > 0 {
		fmt.Fprintf(w, "  per-round prompt grew %s -> %s  (%.1fx over %d rounds)\n",
			comma(rep.FirstInput), comma(rep.LastInput), float64(rep.LastInput)/float64(rep.FirstInput), len(rep.Rounds))
	}
	if rep.TotalCache > 0 {
		fmt.Fprintf(w, "  cache hit rate %.0f%%  (%s of %s input served from cache)\n",
			rep.CacheHitRate()*100, comma(rep.TotalCache), comma(rep.TotalInput))
	} else {
		fmt.Fprintln(w, "  cache hit rate 0%  (provider reports no cached tokens: the whole re-send is billed full price)")
	}

	fmt.Fprintln(w, "\nBiggest input jumps (a fat tool result that then re-sends every later round)")
	for _, r := range rep.TopJumps(5) {
		if r.InputDelta <= 0 {
			continue
		}
		// A jump of D tokens at round k re-sends on every one of the rounds after it,
		// so its lifetime cost is roughly D times the rounds that follow.
		remaining := len(rep.Rounds) - r.N
		if remaining < 0 {
			remaining = 0
		}
		fmt.Fprintf(w, "  round %-3d +%-7s  carried for ~%d more rounds (~%s tokens of re-send)\n",
			r.N, comma(r.InputDelta), remaining, comma(r.InputDelta*remaining))
	}

	fmt.Fprintln(w, "\nRound-by-round (prompt tokens, new this round, output, tool calls)")
	for _, r := range rep.Rounds {
		cache := ""
		if r.CachedTok > 0 {
			cache = fmt.Sprintf(" cached=%s", comma(r.CachedTok))
		}
		fmt.Fprintf(w, "  r%-3d msgs=%-3d in=%-8s +%-7s out=%-5d tools=%d%s\n",
			r.N, r.Messages, comma(r.InputTok), comma(r.InputDelta), r.OutputTok, r.ToolCalls, cache)
	}
}

// WriteProjection prints the counterfactual: the run's actual input cost next to
// each modeled transcript-shaping strategy, so a change can be judged before a
// single token is spent. The strategies are grounded on the recorded per-round
// tokens, so the savings are an estimate of the same run under a different loop.
func WriteProjection(w io.Writer, actual Projection, strategies []Projection) {
	fmt.Fprintln(w, "\nProjection (recorded run re-costed under each transcript strategy)")
	fmt.Fprintf(w, "  %-34s %14s %10s %9s\n", "strategy", "input tokens", "ratio", "vs actual")
	fmt.Fprintf(w, "  %-34s %14s %9.0f:1 %9s\n", actual.Name, comma(actual.TotalInput), actual.ResendRatio(), "-")
	for _, s := range strategies {
		saved := 0.0
		if actual.TotalInput > 0 {
			saved = (1 - float64(s.TotalInput)/float64(actual.TotalInput)) * 100
		}
		fmt.Fprintf(w, "  %-34s %14s %9.0f:1 %8.0f%%\n", s.Name, comma(s.TotalInput), s.ResendRatio(), saved)
	}
	fmt.Fprintln(w, "  (input tokens only; a shaping change can also alter what the model does next)")
}

// WriteToolMix prints the tool call counts a run made, most-used first, from the
// summary's tool_calls_by map. It shows the shape of the run: heavy reads and
// greps with no writes is an investigation that never converged.
func WriteToolMix(w io.Writer, summary map[string]any) {
	by, ok := summary["tool_calls_by"].(map[string]any)
	if !ok || len(by) == 0 {
		return
	}
	type kv struct {
		name string
		n    int
	}
	var mix []kv
	for k, v := range by {
		if f, ok := v.(float64); ok {
			mix = append(mix, kv{k, int(f)})
		}
	}
	sort.Slice(mix, func(i, j int) bool {
		if mix[i].n != mix[j].n {
			return mix[i].n > mix[j].n
		}
		return mix[i].name < mix[j].name
	})
	fmt.Fprintln(w, "\nTool mix")
	for _, m := range mix {
		fmt.Fprintf(w, "  %-6s %d\n", m.name, m.n)
	}
}

// comma formats n with thousands separators, so a six or seven figure token count
// reads at a glance.
func comma(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
