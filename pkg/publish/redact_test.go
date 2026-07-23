package publish

import (
	"os"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestRedactMessage checks that a message carrying credential shapes is scrubbed
// on the way into a trace: the bearer token, the OPENCODE_API_KEY value, and an
// sk- key are all masked, while ordinary prose is untouched.
func TestRedactMessage(t *testing.T) {
	m := &stsMessage{
		Content:          "curl -H 'Authorization: Bearer sk-abc123DEF456ghi789JKL' https://api",
		ReasoningContent: "the key OPENCODE_API_KEY=abcdef123456ghijkl should stay hidden",
		ToolCalls: []stsToolCall{{
			Function: stsToolFunc{Name: "run", Arguments: `{"env":"HF_TOKEN=hf_abcdefghijklmnopqrstuvwxyz012345"}`},
		}},
	}
	redactMessage(m)

	if strings.Contains(m.Content, "sk-abc123DEF456ghi789JKL") {
		t.Errorf("bearer/sk key survived: %q", m.Content)
	}
	if !strings.Contains(m.Content, "[REDACTED") {
		t.Errorf("content not masked: %q", m.Content)
	}
	if strings.Contains(m.ReasoningContent, "abcdef123456ghijkl") {
		t.Errorf("OPENCODE_API_KEY value survived: %q", m.ReasoningContent)
	}
	if strings.Contains(m.ToolCalls[0].Function.Arguments, "hf_abcdefghijklmnopqrstuvwxyz012345") {
		t.Errorf("HF token in tool args survived: %q", m.ToolCalls[0].Function.Arguments)
	}
}

// TestScanFilesGate asserts the pre-commit gate blocks a file carrying a secret
// and names it, and passes a clean file.
func TestScanFilesGate(t *testing.T) {
	tainted := []HFOp{
		{PathInRepo: "data/x.jsonl", Content: []byte(`{"content":"Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"}`)},
	}
	f := ScanFiles(tainted)
	if f == nil {
		t.Fatal("gate missed a tainted file")
	}
	if f.Path != "data/x.jsonl" {
		t.Errorf("gate named wrong file: %q", f.Path)
	}

	clean := []HFOp{
		{PathInRepo: "data/y.jsonl", Content: []byte(`{"content":"just fixing an off-by-one in foo.py"}`)},
		{PathInRepo: "reports/board.md", Content: []byte("# Board\n\nnothing secret here\n")},
	}
	if f := ScanFiles(clean); f != nil {
		t.Errorf("gate flagged a clean set: %+v", f)
	}
}
