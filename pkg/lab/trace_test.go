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

// writeLat writes a latency.jsonl with the given lines and returns its path.
func writeLat(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "latency.jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// streamErrorStats counts the model calls the proxy flagged as dropped
// mid-stream and keeps the first error message; a log with no such row yields
// nil so the result omits the field.
func TestStreamErrorStats(t *testing.T) {
	if got := streamErrorStats(writeLat(t,
		`{"seq":1,"status":200,"path":"/v1/chat/completions"}`,
		`{"seq":2,"status":200,"path":"/v1/chat/completions","stream_err":true,"stream_err_msg":"Streaming response failed"}`,
		`{"seq":3,"status":200,"path":"/v1/chat/completions","stream_err":true,"stream_err_msg":"truncated stream"}`,
	)); got == nil || got.Calls != 2 || got.Sample != "Streaming response failed" {
		t.Errorf("stream fail = %+v, want 2 calls sampling the first message", got)
	}

	if got := streamErrorStats(writeLat(t,
		`{"seq":1,"status":200,"path":"/v1/chat/completions"}`,
		`{"seq":2,"status":429,"path":"/v1/chat/completions"}`,
	)); got != nil {
		t.Errorf("no stream drop should yield nil, got %+v", got)
	}
}

// droppedFinalStream is the grade-time safety net for the proxy's blind spot: a
// final turn the gateway cut off before its [DONE]/usage, torn down before the
// proxy could write its latency row. It should fire when a resp file shows the
// truncation fingerprint and has no matching latency row, and stay quiet when the
// stream closed cleanly or when the truncated call did leave a row (already
// caught by streamErrorStats).
func TestDroppedFinalStreamCatchesUnflushedTruncation(t *testing.T) {
	clean := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: {\"usage\":{\"total_tokens\":9}}\n\ndata: [DONE]\n"
	truncated := "data: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"reasoning\":\"more\"}}]}\n"
	write := func(t *testing.T, dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Five clean turns each left a latency row; the sixth was cut off and, because
	// the pod died before onClose ran, left no row at all.
	dir := t.TempDir()
	write(t, dir, "resp-1.txt", clean)
	write(t, dir, "resp-2.txt", clean)
	write(t, dir, "resp-6.txt", truncated)
	write(t, dir, "latency.jsonl", "{\"seq\":1,\"status\":200}\n{\"seq\":2,\"status\":200}\n")
	if !droppedFinalStream(dir) {
		t.Fatal("a truncated resp with no latency row should be flagged as a dropped stream")
	}

	// If that same truncated call had left a latency row, streamErrorStats owns it
	// and droppedFinalStream must not double-count it.
	withRow := t.TempDir()
	write(t, withRow, "resp-6.txt", truncated)
	write(t, withRow, "latency.jsonl", "{\"seq\":6,\"status\":200,\"stream_err\":true}\n")
	if droppedFinalStream(withRow) {
		t.Error("a truncated call that already has a latency row must not be re-flagged here")
	}

	// A run whose every turn closed cleanly is not a dropped stream.
	allClean := t.TempDir()
	write(t, allClean, "resp-1.txt", clean)
	if droppedFinalStream(allClean) {
		t.Error("a cleanly closed stream must not be flagged")
	}
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
