package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFiles creates dir and writes each named file with a byte of content, so
// the finder has real files to walk.
func writeFiles(dir string, names ...string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// sampleRollout is a compact but faithful rollout: one session_meta, one
// turn_context on gpt-5.5 at high effort, a user prompt, a reasoning block, an
// exec_command call and its output, an apply_patch write and its output, a
// token_count with real usage, and a task_complete. It exercises every record
// type the parser handles.
const sampleRollout = `
{"timestamp":"2026-07-07T15:50:23.001Z","type":"session_meta","payload":{"session_id":"sess-1","cwd":"/work","originator":"codex_exec","cli_version":"0.142.3","source":"exec","model_provider":"openai","base_instructions":{"text":"You are Codex."}}}
{"timestamp":"2026-07-07T15:50:23.100Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/work","current_date":"2026-07-07","timezone":"Asia/Ho_Chi_Minh","approval_policy":"never","model":"gpt-5.5","effort":"high","summary":"auto"}}
{"timestamp":"2026-07-07T15:50:23.110Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-1","model_context_window":258400}}
{"timestamp":"2026-07-07T15:50:23.120Z","type":"event_msg","payload":{"type":"user_message","message":"fix the bug"}}
{"timestamp":"2026-07-07T15:50:24.000Z","type":"response_item","payload":{"type":"reasoning","id":"rs-1","summary":[],"encrypted_content":"abc","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}
{"timestamp":"2026-07-07T15:50:25.000Z","type":"response_item","payload":{"type":"function_call","id":"fc-1","name":"exec_command","arguments":"{\"cmd\":\"ls\"}","call_id":"c1","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}
{"timestamp":"2026-07-07T15:50:25.100Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"file.py","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}
{"timestamp":"2026-07-07T15:50:26.000Z","type":"response_item","payload":{"type":"custom_tool_call","id":"ctc-1","status":"completed","call_id":"c2","name":"apply_patch","input":"*** Begin Patch","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}
{"timestamp":"2026-07-07T15:50:26.100Z","type":"event_msg","payload":{"type":"patch_apply_end","call_id":"c2","turn_id":"turn-1","stdout":"ok","success":true,"changes":{"/work/file.py":{}}}}
{"timestamp":"2026-07-07T15:50:26.200Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c2","output":"Exit code: 0","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}
{"timestamp":"2026-07-07T15:50:27.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":400,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":1200},"last_token_usage":{"input_tokens":1000,"cached_input_tokens":400,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":1200},"model_context_window":258400}}}
{"timestamp":"2026-07-07T15:50:28.000Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"turn-1","last_agent_message":"done","duration_ms":5000,"time_to_first_token_ms":900}}
`

func parseSample(t *testing.T) *Rollout {
	t.Helper()
	r, err := ParseRollout(strings.NewReader(sampleRollout))
	if err != nil {
		t.Fatalf("ParseRollout: %v", err)
	}
	return r
}

func TestParseRolloutRecords(t *testing.T) {
	r := parseSample(t)
	if r.Meta.SessionID != "sess-1" {
		t.Errorf("session id = %q, want sess-1", r.Meta.SessionID)
	}
	if r.Meta.CLIVersion != "0.142.3" {
		t.Errorf("cli version = %q, want 0.142.3", r.Meta.CLIVersion)
	}
	if r.Meta.Instructions != "You are Codex." {
		t.Errorf("instructions = %q, want the system prompt", r.Meta.Instructions)
	}
	if len(r.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(r.Turns))
	}
	if r.Turns[0].Model != "gpt-5.5" || r.Turns[0].Effort != "high" {
		t.Errorf("turn model/effort = %q/%q, want gpt-5.5/high", r.Turns[0].Model, r.Turns[0].Effort)
	}
	if len(r.Items) != 5 {
		t.Fatalf("items = %d, want 5", len(r.Items))
	}
	for _, it := range r.Items {
		if it.TurnID != "turn-1" {
			t.Errorf("item %q turn id = %q, want turn-1", it.Type, it.TurnID)
		}
	}
}

func TestParseRolloutWriteClassification(t *testing.T) {
	r := parseSample(t)
	var writes, calls int
	for _, it := range r.Items {
		if it.IsToolCall() {
			calls++
		}
		if it.IsWrite() {
			writes++
		}
	}
	if calls != 2 {
		t.Errorf("tool calls = %d, want 2 (exec + apply_patch)", calls)
	}
	if writes != 1 {
		t.Errorf("writes = %d, want 1 (apply_patch)", writes)
	}
}

func TestParseRolloutEvents(t *testing.T) {
	r := parseSample(t)
	var patch *Event
	var tok *Event
	for i := range r.Events {
		switch r.Events[i].Type {
		case "patch_apply_end":
			patch = &r.Events[i]
		case "token_count":
			tok = &r.Events[i]
		}
	}
	if patch == nil {
		t.Fatal("no patch_apply_end event")
	}
	if !patch.Success {
		t.Error("patch success = false, want true")
	}
	if len(patch.Changes) != 1 || patch.Changes[0] != "/work/file.py" {
		t.Errorf("patch changes = %v, want [/work/file.py]", patch.Changes)
	}
	if tok == nil || tok.Tokens == nil {
		t.Fatal("no token_count with usage")
	}
	if tok.Tokens.Total.TotalTokens != 1200 || tok.Tokens.Total.ReasoningOutputTokens != 50 {
		t.Errorf("total usage = %+v, want 1200 total / 50 reasoning", tok.Tokens.Total)
	}
}

func TestSummarize(t *testing.T) {
	r := parseSample(t)
	s := r.Summarize()
	if len(s.Models) != 1 || s.Models[0] != (ModelUse{Model: "gpt-5.5", Effort: "high"}) {
		t.Errorf("models = %v, want one gpt-5.5/high", s.Models)
	}
	if s.ToolCalls != 2 || s.Writes != 1 {
		t.Errorf("tool calls/writes = %d/%d, want 2/1", s.ToolCalls, s.Writes)
	}
	if s.ByTool["exec_command"] != 1 || s.ByTool["apply_patch"] != 1 {
		t.Errorf("by tool = %v, want one each", s.ByTool)
	}
	if s.Prompt != "fix the bug" {
		t.Errorf("prompt = %q, want 'fix the bug'", s.Prompt)
	}
	if s.Tokens.TotalTokens != 1200 {
		t.Errorf("tokens = %d, want 1200", s.Tokens.TotalTokens)
	}
	if !s.Complete || s.Aborted {
		t.Errorf("complete/aborted = %v/%v, want true/false", s.Complete, s.Aborted)
	}
	if s.WallMs != 5000 {
		t.Errorf("wall = %dms, want 5000", s.WallMs)
	}
}

func TestParseRolloutSkipsBlankLines(t *testing.T) {
	// Leading and trailing blank lines and a stray blank in the middle must not
	// break parsing or add phantom records.
	in := "\n\n" + strings.TrimSpace(sampleRollout) + "\n\n"
	r, err := ParseRollout(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseRollout: %v", err)
	}
	if len(r.Turns) != 1 {
		t.Errorf("turns = %d, want 1", len(r.Turns))
	}
}

func TestParseRolloutMalformedLine(t *testing.T) {
	_, err := ParseRollout(strings.NewReader("{not json}\n"))
	if err == nil {
		t.Fatal("want an error on a malformed line, got nil")
	}
}

func TestFindRolloutsNewestFirst(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "07", "07")
	if err := writeFiles(dir,
		"rollout-2026-07-07T10-00-00-a.jsonl",
		"rollout-2026-07-07T12-00-00-b.jsonl",
		"not-a-rollout.txt",
	); err != nil {
		t.Fatal(err)
	}
	got, err := FindRollouts(home)
	if err != nil {
		t.Fatalf("FindRollouts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("found %d rollouts, want 2: %v", len(got), got)
	}
	if !strings.HasSuffix(got[0], "12-00-00-b.jsonl") {
		t.Errorf("newest first failed, got[0] = %s", got[0])
	}
}

func TestFindRolloutsMissingTree(t *testing.T) {
	got, err := FindRollouts(t.TempDir())
	if err != nil {
		t.Fatalf("FindRollouts on empty home: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("found %d rollouts in empty home, want 0", len(got))
	}
}
