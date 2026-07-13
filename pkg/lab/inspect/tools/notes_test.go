package tools

import (
	"strings"
	"testing"

	"github.com/tamnd/tomo-labs/pkg/lab/inspect"
)

// These tests drive the per-tool notes through inspect.Analyze with each tool's
// registered built-in profile, so the notes read the same reclassified steps they
// see in a real run (bash searches split out, action buckets tagged). They live in
// this package because that is where the notes and the registry are.

// tomoNotes should flag the habits tomo is tuned to avoid: re-reading a file it
// just wrote, and re-running an identical shell command.
func TestTomoNotesFlagsWaste(t *testing.T) {
	steps := []inspect.Step{
		{Kind: "call", Name: "write", Text: `{"path":"a.py","content":"x"}`},
		{Kind: "call", Name: "read", Text: `{"path":"a.py"}`},
		{Kind: "call", Name: "bash", Text: `{"command":"pytest -q"}`},
		{Kind: "call", Name: "bash", Text: `{"command":"pytest -q"}`},
	}
	s := inspect.Analyze("tomo", inspect.BuiltinProfile("tomo"), steps)
	joined := strings.Join(s.Notes, " | ")
	if !strings.Contains(joined, "re-read a file") {
		t.Errorf("expected read-after-write note, got: %q", joined)
	}
	if !strings.Contains(joined, "re-ran an identical shell command") {
		t.Errorf("expected repeated-command note, got: %q", joined)
	}
}

// pi has no dedicated search tool, so it greps and finds through bash. piNotes
// should report how much searching went through the shell once Analyze has
// reclassified those calls.
func TestPiNotesShellSearch(t *testing.T) {
	steps := []inspect.Step{
		{Kind: "call", Name: "bash", Text: `{"command":"grep -rn _ConanIgnoreMatcher conan/"}`},
		{Kind: "result", Text: "conan/internal/api/config/config_installer.py:15:"},
		{Kind: "call", Name: "bash", Text: `{"command":"cd /work && find . -name conftest.py"}`},
		{Kind: "result", Text: "./test/conftest.py"},
		{Kind: "call", Name: "read", Text: `{"path":"config_installer.py"}`},
		{Kind: "result", Text: "class _ConanIgnoreMatcher:"},
		{Kind: "call", Name: "edit", Text: `{"path":"config_installer.py"}`},
		{Kind: "result", Text: "edited config_installer.py"},
		{Kind: "call", Name: "bash", Text: `{"command":"cd /work && python -m pytest -q"}`},
		{Kind: "result", Text: "1 passed"},
	}
	s := inspect.Analyze("pi", inspect.BuiltinProfile("pi"), steps)
	if !strings.Contains(strings.Join(s.Notes, " | "), "searching through the shell (2 commands)") {
		t.Errorf("piNotes should count shell searches, got: %v", s.Notes)
	}
}

// claude-code solves through subagents, so much of its transcript is Agent,
// SendMessage, and TaskOutput calls that coordinate other agents rather than touch
// the code. claudeNotes should surface that coordination as a share of the total.
func TestClaudeNotesFlagsOrchestration(t *testing.T) {
	steps := []inspect.Step{
		{Kind: "call", Name: "Bash", Text: `{"command":"grep -rn Matcher conan/"}`},
		{Kind: "result", Text: "conan/x.py:15:"},
		{Kind: "call", Name: "Agent", Text: `{"prompt":"investigate the matcher"}`},
		{Kind: "result", Text: "spawned"},
		{Kind: "call", Name: "SendMessage", Text: `{"id":"a1","text":"look at negation"}`},
		{Kind: "result", Text: "ok"},
		{Kind: "call", Name: "TaskOutput", Text: `{"id":"a1"}`},
		{Kind: "result", Text: "the matcher is here"},
		{Kind: "call", Name: "Read", Text: `{"path":"config_installer.py"}`},
		{Kind: "result", Text: "class Matcher:"},
		{Kind: "call", Name: "Edit", Text: `{"path":"config_installer.py"}`},
		{Kind: "result", Text: "edited"},
	}
	s := inspect.Analyze("claude-code", inspect.BuiltinProfile("claude-code"), steps)
	if s.Searches != 1 {
		t.Errorf("Bash grep should be a search, got Searches=%d", s.Searches)
	}
	if s.Other != 3 {
		t.Errorf("Agent, SendMessage, TaskOutput should be other, got Other=%d", s.Other)
	}
	joined := strings.Join(s.Notes, " | ")
	if !strings.Contains(joined, "coordinating subagents") || !strings.Contains(joined, "50%") {
		t.Errorf("claudeNotes should report the coordination share, got: %v", s.Notes)
	}
}

// opencode's SWE-bench failure mode is analysis paralysis: it greps, reads the
// buggy source, reads the tests, and then never edits, so the source ships
// unchanged and the hidden tests fail. opencodeNotes should call out "read the
// bug, never wrote the fix", and separately flag a run that edits a test file when
// the task said to leave tests alone.
func TestOpencodeNotesFlagsNeverEdited(t *testing.T) {
	steps := []inspect.Step{
		{Kind: "call", Name: "grep", Text: `{"pattern":"conanignore"}`},
		{Kind: "result", Text: "config_installer.py:16:"},
		{Kind: "call", Name: "read", Text: `{"filePath":"conan/internal/api/config/config_installer.py"}`},
		{Kind: "result", Text: "class _ConanIgnoreMatcher:"},
		{Kind: "call", Name: "read", Text: `{"filePath":"test/integration/command/config_test.py"}`},
		{Kind: "result", Text: "def test_config_install():"},
	}
	s := inspect.Analyze("opencode", inspect.BuiltinProfile("opencode"), steps)
	if s.Edits != 0 {
		t.Fatalf("this run made no edits, got Edits=%d", s.Edits)
	}
	if !strings.Contains(strings.Join(s.Notes, " | "), "never edited") {
		t.Errorf("opencodeNotes should flag the never-edited run, got: %v", s.Notes)
	}

	// A run that does edit, but edits a test file, gets the softer flag instead.
	edited := []inspect.Step{
		{Kind: "call", Name: "read", Text: `{"filePath":"config_installer.py"}`},
		{Kind: "result", Text: "class _ConanIgnoreMatcher:"},
		{Kind: "call", Name: "edit", Text: `{"filePath":"test/integration/command/config_test.py"}`},
		{Kind: "result", Text: "edited"},
	}
	se := inspect.Analyze("opencode", inspect.BuiltinProfile("opencode"), edited)
	if !strings.Contains(strings.Join(se.Notes, " | "), "edited a test file") {
		t.Errorf("opencodeNotes should flag editing a test file, got: %v", se.Notes)
	}
}
