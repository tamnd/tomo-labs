package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDiskPath builds the codex-style date-tree path from a run's timestamp and
// labels, with the day taken from the timestamp and every component slugged.
func TestDiskPath(t *testing.T) {
	got := DiskPath("/data", Header{
		Harness:   "tomo-oi",
		Scenario:  "dynaconf__dynaconf-1225",
		ID:        "20260722T181112Z",
		Timestamp: "20260722T181112Z",
	})
	want := "/data/sessions/2026/07/22/tomo-oi-dynaconf__dynaconf-1225-20260722T181112Z.jsonl"
	if got != want {
		t.Fatalf("DiskPath = %s, want %s", got, want)
	}
}

// TestWriteSession writes a run's session file to the date tree and checks it is
// the standard self-describing artifact: one file, whose leading session record
// carries the run provenance a downstream consumer needs without reading any
// other file.
func TestWriteSession(t *testing.T) {
	src := t.TempDir()
	body := map[string]any{"messages": []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "hello"},
	}}
	raw, _ := json.Marshal(map[string]any{"body": body})
	write(t, src, "requests.jsonl", string(raw)+"\n")

	root := t.TempDir()
	h := Header{
		Harness: "codex", Eval: "swebench-live", Scenario: "s", Model: "gpt-5.6-luna",
		ID: "20260722T181112Z", Timestamp: "20260722T181112Z", Passed: true,
		Tokens: &Tokens{Prompt: 100, Completion: 10, Total: 110},
	}
	path, err := WriteSession(root, src, h)
	if err != nil {
		t.Fatalf("WriteSession: %v", err)
	}
	want := filepath.Join(root, "sessions", "2026", "07", "22", "codex-s-20260722T181112Z.jsonl")
	if path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written session: %v", err)
	}
	// The first line is the self-describing session record.
	first := bytes.SplitN(data, []byte("\n"), 2)[0]
	var sess Session
	if json.Unmarshal(first, &sess) != nil || sess.Type != "session" {
		t.Fatalf("leading record is not a session: %s", first)
	}
	if sess.Meta == nil || sess.Meta.Harness != "codex" || sess.Meta.Scenario != "s" || !sess.Meta.Passed {
		t.Fatalf("session meta not self-describing: %+v", sess.Meta)
	}
	if sess.Meta.Tokens == nil || sess.Meta.Tokens.Total != 110 {
		t.Fatalf("session meta tokens missing: %+v", sess.Meta)
	}
}
