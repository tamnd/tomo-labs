package trace

import (
	"path/filepath"
	"testing"
)

// TestAuditCodexParity builds a codex-shaped native rollout with a known turn
// count and checks the audit tallies exactly those items, ignoring the rollout's
// non-conversation lines (session_meta, event_msg, turn_context).
func TestAuditCodexParity(t *testing.T) {
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout-2026-07-22T00-00-00-abc.jsonl")
	lines := []string{
		`{"timestamp":"t","type":"session_meta","payload":{"session_id":"x"}}`,
		`{"timestamp":"t","type":"event_msg","payload":{"type":"task_started"}}`,
		`{"timestamp":"t","type":"turn_context","payload":{"model":"m"}}`,
		`{"timestamp":"t","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"be a coder"}]}}`,
		`{"timestamp":"t","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the bug"}]}}`,
		`{"timestamp":"t","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"let me look"}]}}`,
		`{"timestamp":"t","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"ls\"}","call_id":"c1"}}`,
		`{"timestamp":"t","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"file.py"}}`,
		`{"timestamp":"t","type":"event_msg","payload":{"type":"agent_message"}}`,
		`{"timestamp":"t","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done, fixed it"}]}}`,
	}
	writeLines(t, rollout, lines)

	msgs, err := AuditCodex(rollout)
	if err != nil {
		t.Fatalf("AuditCodex: %v", err)
	}
	c := Tally(msgs)
	// developer folds to system; one user; one reasoning; one call; one result;
	// one assistant reply that is the final reply.
	if c.System != 1 || c.User != 1 || c.Thinking != 1 || c.ToolCall != 1 || c.ToolResult != 1 || c.Assistant != 1 {
		t.Fatalf("tally = %+v", c)
	}
	if !c.FinalReply {
		t.Fatalf("final reply not detected: %+v", c)
	}
}

// TestCompareDetectsDroppedReply proves the audit flags the exact failure the
// original capture had: the tool calls survive but the final assistant reply is
// gone. The oracle ends on a reply; the reconstruction stops at the last tool
// result.
func TestCompareDetectsDroppedReply(t *testing.T) {
	oracle := []Message{
		{Role: "user", Content: []Block{textBlock("fix it")}},
		{Role: "assistant", Content: []Block{toolCallBlock(wireCall{ID: "c1", Name: "shell", Args: "{}"})}},
		{Role: "tool", Content: []Block{toolResultBlock("c1", "ok")}},
		{Role: "assistant", Content: []Block{textBlock("all done")}},
	}
	// The reconstruction dropped the trailing reply.
	ours := oracle[:3]

	rep := Compare(oracle, ours)
	if rep.OK() {
		t.Fatalf("expected a shortfall, got clean: %+v", rep)
	}
	found := false
	for _, m := range rep.Missing {
		if m == "final assistant reply is missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dropped reply not reported: %v", rep.Missing)
	}

	// A complete reconstruction audits clean, even carrying an extra system turn.
	full := append([]Message{{Role: "system", Content: []Block{textBlock("setup")}}}, oracle...)
	if rep := Compare(oracle, full); !rep.OK() {
		t.Fatalf("complete reconstruction should audit clean: %v", rep.Missing)
	}
}

// TestAuditOpencodeParity builds an opencode native-session dump in the exact
// shape the container's SQLite export writes — one {role, part} object per line —
// and checks the audit tallies each part kind: a user prompt, assistant
// reasoning, a completed tool part expanding to a call and a result, and the
// final assistant reply. The non-conversation parts (step-start, step-finish,
// patch) carry no turn and must be ignored.
func TestAuditOpencodeParity(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "opencode", "session.jsonl")
	lines := []string{
		`{"role":"user","part":{"type":"text","text":"fix the bug"}}`,
		`{"role":"assistant","part":{"type":"step-start"}}`,
		`{"role":"assistant","part":{"type":"reasoning","text":"let me look"}}`,
		`{"role":"assistant","part":{"type":"tool","tool":"bash","callID":"c1","state":{"status":"completed","input":{"command":"ls"},"output":"file.py"}}}`,
		`{"role":"assistant","part":{"type":"patch","text":"diff"}}`,
		`{"role":"assistant","part":{"type":"step-finish"}}`,
		`{"role":"assistant","part":{"type":"text","text":"done, fixed it"}}`,
	}
	writeLines(t, dump, lines)

	if got := OpencodeDump(filepath.Join(dir)); got != dump {
		t.Fatalf("OpencodeDump = %q, want %q", got, dump)
	}
	msgs, err := AuditOpencode(dump)
	if err != nil {
		t.Fatalf("AuditOpencode: %v", err)
	}
	c := Tally(msgs)
	if c.User != 1 || c.Thinking != 1 || c.ToolCall != 1 || c.ToolResult != 1 || c.Assistant != 1 {
		t.Fatalf("tally = %+v", c)
	}
	if !c.FinalReply {
		t.Fatalf("final reply not detected: %+v", c)
	}
}

// TestAuditClaudeParity builds a Claude Code transcript in the shape the CLI
// writes under ~/.claude/projects and checks the audit, reusing the analyzer's
// parser, tallies the opening prompt, the assistant's reasoning and tool call,
// the tool_result carried on the next user turn, and the final reply. It also
// checks ClaudeSession finds the transcript under native/claude.
func TestAuditClaudeParity(t *testing.T) {
	dir := t.TempDir()
	sess := filepath.Join(dir, "claude", "projects", "proj", "abc-123.jsonl")
	lines := []string{
		`{"type":"system","sessionId":"abc-123","cwd":"/testbed","version":"1.0"}`,
		`{"type":"user","message":{"role":"user","content":"fix the bug"}}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"thinking","thinking":"let me look"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"file.py"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"done, fixed it"}]}}`,
	}
	writeLines(t, sess, lines)

	if got := ClaudeSession(filepath.Join(dir)); got != sess {
		t.Fatalf("ClaudeSession = %q, want %q", got, sess)
	}
	msgs, err := AuditClaude(sess)
	if err != nil {
		t.Fatalf("AuditClaude: %v", err)
	}
	c := Tally(msgs)
	if c.User != 1 || c.Thinking != 1 || c.ToolCall != 1 || c.ToolResult != 1 || c.Assistant != 1 {
		t.Fatalf("tally = %+v", c)
	}
	if !c.FinalReply {
		t.Fatalf("final reply not detected: %+v", c)
	}
}

// TestNativeOracleSelects proves the auto-detector picks the right oracle from
// whatever native store a run captured, so one audit path serves every tool.
func TestNativeOracleSelects(t *testing.T) {
	// An opencode-only native dir resolves to the opencode oracle.
	oc := t.TempDir()
	writeLines(t, filepath.Join(oc, "opencode", "session.jsonl"), []string{
		`{"role":"user","part":{"type":"text","text":"hi"}}`,
		`{"role":"assistant","part":{"type":"text","text":"hello"}}`,
	})
	if tool, msgs, err := NativeOracle(oc); err != nil || tool != "opencode" || len(msgs) != 2 {
		t.Fatalf("opencode select: tool=%q n=%d err=%v", tool, len(msgs), err)
	}
	// A dir with no recognized store is un-auditable, not an error.
	if tool, _, err := NativeOracle(t.TempDir()); err != nil || tool != "" {
		t.Fatalf("empty select: tool=%q err=%v", tool, err)
	}
}

func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	write(t, filepath.Dir(path), filepath.Base(path), body)
}
