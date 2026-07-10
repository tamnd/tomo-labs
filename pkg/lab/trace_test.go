package lab

import (
	"os"
	"path/filepath"
	"testing"
)

// writeReqs writes a requests.jsonl with the given lines and returns its path.
func writeReqs(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "requests.jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestOrchestrationPlanSpellings pins the metric to each wired tool's own name
// for its plan primitive: the union is what keeps a real plan from reading as a
// flat loop just because the tool spells it differently.
func TestOrchestrationPlanSpellings(t *testing.T) {
	// One completion request whose transcript holds an assistant turn calling a
	// plan tool. The largest body wins, so a single rich request is enough.
	req := func(tool string) string {
		return `{"method":"POST","path":"/v1/chat/completions","body":{"messages":[` +
			`{"role":"system","content":"you are a coding agent"},` +
			`{"role":"user","content":"do the task"},` +
			`{"role":"assistant","tool_calls":[{"function":{"name":"` + tool + `"}}]}]}}`
	}
	for _, name := range []string{"todowrite", "update_plan", "TaskCreate", "TaskUpdate", "todo", "write_todos"} {
		o := orchestration(writeReqs(t, req(name)))
		if !o.Planned || o.PlanCalls == 0 {
			t.Errorf("%q: want planned, got %+v", name, o)
		}
	}
	for _, name := range []string{"task", "invoke_agent", "subagents", "delegate_task"} {
		o := orchestration(writeReqs(t, req(name)))
		if !o.Planned || o.Subagents == 0 {
			t.Errorf("%q: want subagent, got %+v", name, o)
		}
	}
}

// TestOrchestrationPlannerPass covers tomo's shape: no plan tool call at all, a
// planning pass recognized by a dedicated planner system prompt.
func TestOrchestrationPlannerPass(t *testing.T) {
	planner := `{"method":"POST","path":"/v1/chat/completions","body":{"messages":[` +
		`{"role":"system","content":"You are tomo's planner. Turn a job into the smallest plan."},` +
		`{"role":"user","content":"build the list"}]}}`
	turn := `{"method":"POST","path":"/v1/chat/completions","body":{"messages":[` +
		`{"role":"system","content":"you are a coding agent"},` +
		`{"role":"user","content":"build the list"}]}}`
	o := orchestration(writeReqs(t, planner, turn))
	if !o.Planned || o.PlanCalls != 1 {
		t.Fatalf("planner pass: want planned with one plan call, got %+v", o)
	}
	if o.ModelCalls != 2 {
		t.Errorf("model calls: want 2 round-trips, got %d", o.ModelCalls)
	}
}

// TestOrchestrationFlat is the honest negative: an agent that only ran ordinary
// tools reads as unplanned, which is a real difference between approaches.
func TestOrchestrationFlat(t *testing.T) {
	flat := `{"method":"POST","path":"/v1/chat/completions","body":{"messages":[` +
		`{"role":"system","content":"you are a coding agent"},` +
		`{"role":"assistant","tool_calls":[{"function":{"name":"write"}},{"function":{"name":"read"}}]}]}}`
	o := orchestration(writeReqs(t, flat))
	if o.Planned || o.PlanCalls != 0 || o.Subagents != 0 {
		t.Errorf("flat loop should read unplanned, got %+v", o)
	}
	if o.ToolCalls != 2 {
		t.Errorf("tool calls: want 2, got %d", o.ToolCalls)
	}
}
