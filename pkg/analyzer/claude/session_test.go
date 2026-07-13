package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleSession is a compact but faithful Claude Code transcript: a system header
// carrying the session identity, a plain-string user prompt, an assistant turn on
// opus with per-call usage across the three cache tiers that also makes a Bash call
// and an Edit, and a following user turn carrying the Bash tool result. It uses one
// Bash command that fetches an answer pull request, so the leak detector has
// something to catch, and one Edit, so the write and edit paths are exercised. The
// bookkeeping line types (mode, file-history-snapshot) are present to prove they
// are skipped.
const sampleSession = `
{"type":"system","timestamp":"2026-07-13T12:00:00.000Z","sessionId":"sess-abc","cwd":"/work/dynaconf","gitBranch":"main","version":"2.1.206"}
{"type":"mode","timestamp":"2026-07-13T12:00:00.100Z","mode":"default"}
{"type":"user","timestamp":"2026-07-13T12:00:01.000Z","message":{"role":"user","content":"fix the dynaconf bug"}}
{"type":"assistant","timestamp":"2026-07-13T12:00:05.000Z","message":{"role":"assistant","model":"claude-opus-4-8","stop_reason":"tool_use","usage":{"input_tokens":100,"cache_creation_input_tokens":50,"cache_read_input_tokens":900,"output_tokens":30,"service_tier":"standard","cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":0}},"content":[{"type":"thinking","thinking":"let me look"},{"type":"text","text":"I will check the PR."},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"gh pr diff 1225 --repo dynaconf/dynaconf"}},{"type":"tool_use","id":"t2","name":"Edit","input":{"file_path":"/work/dynaconf/base.py","old_string":"a","new_string":"b"}}]}}
{"type":"file-history-snapshot","timestamp":"2026-07-13T12:00:05.500Z"}
{"type":"user","timestamp":"2026-07-13T12:00:06.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":false,"content":"diff --git ..."}]}}
`

func parseSample(t *testing.T) *Session {
	t.Helper()
	s, err := ParseSession(strings.NewReader(sampleSession))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	return s
}

func TestParseSessionHeader(t *testing.T) {
	s := parseSample(t)
	if s.SessionID != "sess-abc" {
		t.Errorf("session id = %q, want sess-abc", s.SessionID)
	}
	if s.Cwd != "/work/dynaconf" {
		t.Errorf("cwd = %q, want /work/dynaconf", s.Cwd)
	}
	if s.GitBranch != "main" {
		t.Errorf("branch = %q, want main", s.GitBranch)
	}
	if s.Version != "2.1.206" {
		t.Errorf("version = %q, want 2.1.206", s.Version)
	}
	// Two message turns: one user prompt, one assistant, one user tool_result. The
	// mode and file-history-snapshot lines must not become messages.
	if len(s.Messages) != 3 {
		t.Fatalf("messages = %d, want 3 (user, assistant, user); bookkeeping must be skipped", len(s.Messages))
	}
}

func TestParseSessionUsagePerCall(t *testing.T) {
	s := parseSample(t)
	var a *Message
	for i := range s.Messages {
		if s.Messages[i].Role == "assistant" {
			a = &s.Messages[i]
		}
	}
	if a == nil {
		t.Fatal("no assistant turn")
	}
	if a.Model != "claude-opus-4-8" {
		t.Errorf("model = %q, want claude-opus-4-8", a.Model)
	}
	u := a.Usage
	if u.InputTokens != 100 || u.CacheCreationTokens != 50 || u.CacheReadTokens != 900 || u.OutputTokens != 30 {
		t.Errorf("usage = %+v, want 100/50/900/30 across the tiers", u)
	}
	if u.Ephemeral5mTokens != 50 {
		t.Errorf("5m cache write = %d, want 50", u.Ephemeral5mTokens)
	}
}

func TestBlockClassification(t *testing.T) {
	s := parseSample(t)
	var calls, writes, bash int
	for _, m := range s.Messages {
		for _, b := range m.Blocks {
			if b.IsToolCall() {
				calls++
			}
			if b.IsWrite() {
				writes++
				if b.WrittenPath() != "/work/dynaconf/base.py" {
					t.Errorf("written path = %q, want /work/dynaconf/base.py", b.WrittenPath())
				}
			}
			if b.BashCommand() != "" {
				bash++
			}
		}
	}
	if calls != 2 {
		t.Errorf("tool calls = %d, want 2 (Bash + Edit)", calls)
	}
	if writes != 1 {
		t.Errorf("writes = %d, want 1 (Edit)", writes)
	}
	if bash != 1 {
		t.Errorf("bash commands = %d, want 1", bash)
	}
}

func TestSummarize(t *testing.T) {
	s := parseSample(t)
	sum := s.Summarize()
	if len(sum.Models) != 1 || sum.Models[0] != "claude-opus-4-8" {
		t.Errorf("models = %v, want [claude-opus-4-8]", sum.Models)
	}
	if sum.Turns != 1 {
		t.Errorf("turns = %d, want 1 assistant turn", sum.Turns)
	}
	if sum.ToolCalls != 2 || sum.Writes != 1 || sum.Files != 1 {
		t.Errorf("calls/writes/files = %d/%d/%d, want 2/1/1", sum.ToolCalls, sum.Writes, sum.Files)
	}
	if sum.ByTool["Bash"] != 1 || sum.ByTool["Edit"] != 1 {
		t.Errorf("by tool = %v, want one Bash one Edit", sum.ByTool)
	}
	if sum.Prompt != "fix the dynaconf bug" {
		t.Errorf("prompt = %q, want the first user message", sum.Prompt)
	}
	// Usage is summed across turns; here one assistant turn.
	if sum.Tokens.InputTokens != 100 || sum.Tokens.CacheReadTokens != 900 {
		t.Errorf("summed tokens = %+v, want 100 fresh / 900 read", sum.Tokens)
	}
}

func TestEdits(t *testing.T) {
	s := parseSample(t)
	edits := s.Edits()
	if len(edits) != 1 {
		t.Fatalf("edits = %d, want 1", len(edits))
	}
	e := edits[0]
	if e.Tool != "Edit" || e.Path != "/work/dynaconf/base.py" || e.OldText != "a" || e.NewText != "b" {
		t.Errorf("edit = %+v, want Edit base.py a->b", e)
	}
}

func TestParseSessionSkipsBlankLines(t *testing.T) {
	in := "\n\n" + strings.TrimSpace(sampleSession) + "\n\n"
	s, err := ParseSession(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if len(s.Messages) != 3 {
		t.Errorf("messages = %d, want 3", len(s.Messages))
	}
}

func TestParseSessionMalformedLine(t *testing.T) {
	_, err := ParseSession(strings.NewReader("{not json}\n"))
	if err == nil {
		t.Fatal("want an error on a malformed line, got nil")
	}
}

func TestParseSessionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(path, []byte(sampleSession), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := ParseSessionFile(path)
	if err != nil {
		t.Fatalf("ParseSessionFile: %v", err)
	}
	if s.Path != path {
		t.Errorf("path = %q, want %q", s.Path, path)
	}
	if s.SessionID != "sess-abc" {
		t.Errorf("session id = %q, want sess-abc", s.SessionID)
	}
}
