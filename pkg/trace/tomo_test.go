package trace

import (
	"os"
	"path/filepath"
	"testing"
)

// A tomo run's canonical session.jsonl reconstructs straight to turns, and it
// is preferred over any wire capture in the same directory.
func TestTomoSessionReconstructsAndWins(t *testing.T) {
	dir := t.TempDir()
	session := `{"type":"session","harness":"tomo","model":"deepseek-v4"}
{"type":"message","message":{"role":"system","content":"Be exact."}}
{"type":"message","message":{"role":"user","content":"Fix the bug."}}
{"type":"message","message":{"role":"assistant","content":"Looking.","reasoningContent":"inspect first","toolCalls":[{"id":"c1","function":{"name":"shell","arguments":"{\"cmd\":\"ls\"}"}}]}}
{"type":"message","message":{"role":"tool","toolCallId":"c1","content":"main.py"}}
{"type":"message","message":{"role":"assistant","content":"Done."}}
`
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte(session), 0o644); err != nil {
		t.Fatal(err)
	}
	// A stray wire capture must not win over the canonical session file.
	if err := os.WriteFile(filepath.Join(dir, "requests.jsonl"), []byte(`{"body":{"messages":[{"role":"user","content":"stale"}]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs := Reconstruct(dir, "deepseek-v4")
	if len(msgs) != 5 {
		t.Fatalf("messages = %d, want 5\n%+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" || msgs[2].Role != "assistant" ||
		msgs[3].Role != "tool" || msgs[4].Role != "assistant" {
		t.Fatalf("roles = %v", []string{msgs[0].Role, msgs[1].Role, msgs[2].Role, msgs[3].Role, msgs[4].Role})
	}
	a := msgs[2].Content
	if len(a) != 3 || a[0].Type != "thinking" || a[1].Type != "text" || a[2].Type != "toolCall" || a[2].Name != "shell" {
		t.Fatalf("assistant blocks = %+v", a)
	}
	if r := msgs[3].Content; len(r) != 1 || r[0].Type != "toolResult" || r[0].ToolCallID != "c1" || r[0].Output != "main.py" {
		t.Fatalf("tool block = %+v", r)
	}
}
