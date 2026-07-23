package publish

import (
	"strings"
	"testing"
)

func sampleRuns() []Run {
	return []Run{
		{
			Eval: "swebench-live", RunID: "1",
			Result: Result{
				Tool: "tomo-oi", Scenario: "dynaconf-1225", Model: "gpt-5.6-luna",
				Passed: true, Tokens: Tokens{Prompt: 40000, Completion: 6000, Total: 46000, Cached: 30000, Reasoning: 2000},
				CostUSD: 0.012, WallSeconds: 95, Orchestration: Orchestration{ModelCalls: 8, ToolCalls: 20},
			},
		},
		{
			Eval: "swebench-live", RunID: "2",
			Result: Result{
				Tool: "codex", Scenario: "dynaconf-1225", Model: "gpt-5.6-luna",
				Passed: false, Stop: "turns", Tokens: Tokens{Prompt: 120000, Completion: 9000, Total: 129000},
				// codex run reported no dollar cost: must render unknown, never $0.
				CostUSD: 0, WallSeconds: 300, Orchestration: Orchestration{ModelCalls: 25, ToolCalls: 40},
			},
		},
	}
}

// TestGenerateReportsCostDiscipline asserts the cost law: a run whose provider
// reported no cost renders "unknown", never "$0", while a metered run shows its
// dollar figure. It also checks the board flags the cheapest solver.
func TestGenerateReportsCostDiscipline(t *testing.T) {
	ag := Fold(sampleRuns())
	reports := GenerateReports(ag, "2026-07-22 10:00 UTC")

	cost := string(reports["reports/cost.md"])
	if strings.Contains(cost, "$0.00") {
		t.Errorf("cost report stated a zero cost:\n%s", cost)
	}
	if !strings.Contains(cost, "unknown") {
		t.Errorf("cost report did not label an unreported cost unknown:\n%s", cost)
	}
	if !strings.Contains(cost, "$0.0120") {
		t.Errorf("cost report lost the metered cost:\n%s", cost)
	}

	board := string(reports["reports/board.md"])
	if !strings.Contains(board, "tomo-oi") || !strings.Contains(board, "cheapest solver is `tomo-oi`") {
		t.Errorf("board did not flag the cheapest solver:\n%s", board)
	}

	if _, ok := reports["reports/by-eval/swebench-live.md"]; !ok {
		t.Errorf("missing per-eval report; got keys %v", keys(reports))
	}
	if _, ok := reports["reports/by-model/gpt-5.6-luna.md"]; !ok {
		t.Errorf("missing per-model report; got keys %v", keys(reports))
	}
}

// TestGenerateREADME asserts the front matter carries the agent-traces tag and
// the data glob config, and that the body leads with live coverage numbers.
func TestGenerateREADME(t *testing.T) {
	ag := Fold(sampleRuns())
	readme := GenerateREADME(ag, "2026-07-22 10:00 UTC")

	if !strings.HasPrefix(readme, "---\n") {
		t.Fatal("README missing YAML frontmatter")
	}
	for _, want := range []string{
		"- agent-traces",
		`path: "data/**/*.jsonl"`,
		"pretty_name: Tomo Agent Traces",
		"**2 traces**",
		"cheapest solver is `tomo-oi`",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README missing %q", want)
		}
	}
}

// TestDeterministic asserts the generators are a pure function of the run set:
// the same inputs produce byte-identical output, so a publish that changes
// nothing is a genuine no-op.
func TestDeterministic(t *testing.T) {
	ag := Fold(sampleRuns())
	a := GenerateREADME(ag, "t")
	b := GenerateREADME(Fold(sampleRuns()), "t")
	if a != b {
		t.Error("README generation is not deterministic")
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
