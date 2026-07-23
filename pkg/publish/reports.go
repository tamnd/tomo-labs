package publish

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/trace"
)

// The reports are the analysis the campaign would otherwise write by hand,
// generated instead from the result set so no run goes un-analyzed. Every report
// is a pure function of the aggregate, regenerated in full on each publish, so a
// backfill and a live run over the same results produce byte-identical files.

// GenerateReports returns every report file keyed by its repo path. The board
// and cost views are always present; the per-eval and per-model breakdowns are
// one file each.
func GenerateReports(ag Aggregate, generatedAt string) map[string][]byte {
	out := map[string][]byte{}
	out["reports/board.md"] = []byte(reportBoard(ag, generatedAt))
	out["reports/cost.md"] = []byte(reportCost(ag, generatedAt))
	for _, ev := range ag.Evals {
		out["reports/by-eval/"+trace.Slug(ev)+".md"] = []byte(reportByEval(ag, ev, generatedAt))
	}
	for _, m := range ag.Models {
		out["reports/by-model/"+trace.Slug(m)+".md"] = []byte(reportByModel(ag, m, generatedAt))
	}
	return out
}

func reportBoard(ag Aggregate, generatedAt string) string {
	var b strings.Builder
	b.WriteString("# Board\n\n")
	b.WriteString("Solve rate and cost per tool per eval, generated from every result on disk.\n\n")
	b.WriteString(renderBoard(ag))
	b.WriteString("\n\n## Cheapest solver per eval\n\n")
	b.WriteString(cheapestNote(ag))
	b.WriteString("\n\n## Tasks no tool solved\n\n")
	b.WriteString(unsolvedList(ag))
	b.WriteString("\n\n_Generated " + generatedAt + "._\n")
	return b.String()
}

// unsolvedList names the scenarios that no tool passed, the board's other lead:
// the tasks still open.
func unsolvedList(ag Aggregate) string {
	graded := map[string]bool{}
	solved := map[string]bool{}
	for _, r := range ag.Runs {
		if r.Result.Ungraded {
			continue
		}
		key := r.Eval + " / " + r.Result.Scenario
		graded[key] = true
		if r.Result.Passed {
			solved[key] = true
		}
	}
	var open []string
	for k := range graded {
		if !solved[k] {
			open = append(open, k)
		}
	}
	if len(open) == 0 {
		if len(graded) == 0 {
			return "_No graded scenarios yet._"
		}
		return "_Every graded scenario has a solver._"
	}
	sort.Strings(open)
	var b strings.Builder
	for _, k := range open {
		b.WriteString("- " + k + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func reportCost(ag Aggregate, generatedAt string) string {
	var b strings.Builder
	b.WriteString("# Cost\n\n")
	b.WriteString("Tokens and dollars per tool per eval, with cache-hit and reasoning shares.\n")
	b.WriteString("A cost the provider did not report is shown as unknown with token volume, never as free.\n\n")
	b.WriteString("| Eval | Tool | Model | Runs | Prompt | Completion | Cache read | Reasoning | Cost |\n")
	b.WriteString("|------|------|-------|-----:|-------:|-----------:|-----------:|----------:|-----:|\n")

	type acc struct {
		eval, tool, model                       string
		runs, prompt, completion, cache, reason int
		cost                                    float64
		known                                   bool
	}
	byKey := map[string]*acc{}
	var order []string
	for _, r := range ag.Runs {
		res := r.Result
		key := r.Eval + "\x00" + res.Tool + "\x00" + res.Model
		a := byKey[key]
		if a == nil {
			a = &acc{eval: r.Eval, tool: res.Tool, model: res.Model}
			byKey[key] = a
			order = append(order, key)
		}
		a.runs++
		a.prompt += res.Tokens.Prompt
		a.completion += res.Tokens.Completion
		a.cache += res.Tokens.Cached
		a.reason += res.Tokens.Reasoning
		if res.CostUSD > 0 {
			a.cost += res.CostUSD
			a.known = true
		}
	}
	sort.Strings(order)
	for _, key := range order {
		a := byKey[key]
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %s | %s | %s | %s |\n",
			a.eval, a.tool, orDash(a.model), a.runs,
			fmtTokens(a.prompt), fmtTokens(a.completion),
			fmtTokens(a.cache), fmtTokens(a.reason),
			fmtCost(a.cost, a.known))
	}
	b.WriteString("\n_Generated " + generatedAt + "._\n")
	return b.String()
}

func reportByEval(ag Aggregate, eval, generatedAt string) string {
	var b strings.Builder
	b.WriteString("# " + eval + "\n\n")
	b.WriteString("Per-scenario outcome for every tool on this eval.\n\n")
	b.WriteString("| Scenario | Tool | Model | Result | Tokens | Cost | Model calls | Tool calls | Wall |\n")
	b.WriteString("|----------|------|-------|--------|-------:|-----:|------------:|-----------:|-----:|\n")

	var rows []Run
	for _, r := range ag.Runs {
		if r.Eval == eval {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Result.Scenario != rows[j].Result.Scenario {
			return rows[i].Result.Scenario < rows[j].Result.Scenario
		}
		return rows[i].Result.Tool < rows[j].Result.Tool
	})
	for _, r := range rows {
		res := r.Result
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %d | %d | %s |\n",
			res.Scenario, res.Tool, orDash(res.Model), outcome(res),
			fmtTokens(res.Tokens.Total), fmtCost(res.CostUSD, res.CostUSD > 0),
			res.Orchestration.ModelCalls, res.Orchestration.ToolCalls, fmtWall(res.WallSeconds))
	}
	b.WriteString("\n_Generated " + generatedAt + "._\n")
	return b.String()
}

func reportByModel(ag Aggregate, model, generatedAt string) string {
	var b strings.Builder
	b.WriteString("# " + model + "\n\n")
	b.WriteString("How each tool did on this model, isolating the model's ceiling from a harness's.\n\n")
	b.WriteString("| Eval | Scenario | Tool | Result | Tokens | Cost | Wall |\n")
	b.WriteString("|------|----------|------|--------|-------:|-----:|-----:|\n")

	var rows []Run
	for _, r := range ag.Runs {
		if r.Result.Model == model {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Eval != rows[j].Eval {
			return rows[i].Eval < rows[j].Eval
		}
		if rows[i].Result.Scenario != rows[j].Result.Scenario {
			return rows[i].Result.Scenario < rows[j].Result.Scenario
		}
		return rows[i].Result.Tool < rows[j].Result.Tool
	})
	for _, r := range rows {
		res := r.Result
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			r.Eval, res.Scenario, res.Tool, outcome(res),
			fmtTokens(res.Tokens.Total), fmtCost(res.CostUSD, res.CostUSD > 0), fmtWall(res.WallSeconds))
	}
	b.WriteString("\n_Generated " + generatedAt + "._\n")
	return b.String()
}

// --- formatting ---

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// solvedCell renders a board cell's solve fraction, or "ungraded" when the cell
// held only ungraded prompt runs.
func solvedCell(c BoardCell) string {
	if c.Graded == 0 {
		return "ungraded"
	}
	return fmt.Sprintf("%d/%d", c.Passed, c.Graded)
}

// outcome renders one run's result, keeping a capped fail distinct from a graded
// fail per the measurement law: a timeout or turn-cap is not the same as the
// agent producing a wrong answer.
func outcome(res Result) string {
	if res.Ungraded {
		return "ungraded"
	}
	if res.Passed {
		return "pass"
	}
	if res.Stop != "" {
		return "fail (" + res.Stop + ")"
	}
	return "fail"
}

func fmtTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// fmtCost enforces the cost law in the one place it cannot be forgotten: a cost
// the provider did not report is rendered "unknown", never "$0.00", so a metered
// run is never mistaken for a free one.
func fmtCost(cost float64, known bool) string {
	if !known || cost <= 0 {
		return "unknown"
	}
	// Sub-dollar costs keep four decimals so a fraction-of-a-cent run stays
	// legible rather than rounding to $0.01 and reading as free-ish.
	if cost < 1 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

func fmtWall(sec int) string {
	if sec <= 0 {
		return "-"
	}
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	return fmt.Sprintf("%dm%02ds", sec/60, sec%60)
}
