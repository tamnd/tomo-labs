package lab

import (
	"strings"
	"testing"
)

// transcribe should walk the fullest request in a tap into an ordered step list:
// system, task, the agent's reasoning, each tool call, and each result, dropping
// the shorter earlier requests in favor of the one that carries the whole history.
func TestTranscribeWalksFullestRequest(t *testing.T) {
	short := `{"seq":1,"body":{"messages":[{"role":"system","content":"you are an agent"},{"role":"user","content":"fix the bug"}]}}`
	full := `{"seq":2,"body":{"messages":[` +
		`{"role":"system","content":"you are an agent"},` +
		`{"role":"user","content":"fix the bug"},` +
		`{"role":"assistant","content":"let me look","tool_calls":[{"function":{"name":"grep","arguments":"{\"pattern\":\"foo\"}"}}]},` +
		`{"role":"tool","content":"foo.py:1: def foo()"},` +
		`{"role":"assistant","content":"","tool_calls":[{"function":{"name":"edit","arguments":"{\"path\":\"foo.py\"}"}}]},` +
		`{"role":"tool","content":"edited foo.py"}` +
		`]}}`
	p := writeReqs(t, short, full)

	steps, err := transcribe(p)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, s := range steps {
		kinds = append(kinds, s.Kind)
	}
	want := []string{"system", "user", "assistant", "call", "result", "call", "result"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	if steps[2].Text != "let me look" {
		t.Errorf("assistant text = %q", steps[2].Text)
	}
	if steps[3].Name != "grep" || !strings.Contains(steps[3].Text, "foo") {
		t.Errorf("first call = %+v", steps[3])
	}
	if steps[5].Name != "edit" {
		t.Errorf("second call name = %q, want edit", steps[5].Name)
	}
}

// A tomo run's calls should classify through the shipped lexicon: read is a read,
// grep a search, bash a shell, edit an edit; and the summary should count each and
// name the file that was edited.
func TestAnalyzeClassifiesAndCounts(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "grep", Text: `{"pattern":"foo"}`},
		{Kind: "result", Text: "foo.py:1"},
		{Kind: "call", Name: "read", Text: `{"path":"foo.py"}`},
		{Kind: "result", Text: "def foo(): pass"},
		{Kind: "call", Name: "edit", Text: `{"path":"foo.py"}`},
		{Kind: "result", Text: "edited foo.py"},
		{Kind: "call", Name: "bash", Text: `{"command":"pytest -q"}`},
		{Kind: "result", Text: "1 passed"},
	}
	s := analyze("tomo", builtinProfiles["tomo"], steps)
	if s.Searches != 1 || s.Reads != 1 || s.Edits != 1 || s.Shells != 1 {
		t.Fatalf("counts = %+v", s)
	}
	if len(s.FilesEdit) != 1 || s.FilesEdit[0] != "foo.py" {
		t.Errorf("files edited = %v", s.FilesEdit)
	}
	if !s.Verified {
		t.Errorf("running pytest after an edit should count as verification")
	}
	// analyze tags each call with its action bucket for the walkthrough.
	if steps[6].Act != "shell" || steps[0].Act != "search" {
		t.Errorf("acts not tagged: %q %q", steps[0].Act, steps[6].Act)
	}
}

// tomoNotes should flag the habits tomo is tuned to avoid: re-reading a file it
// just wrote, and re-running an identical shell command.
func TestTomoNotesFlagsWaste(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "write", Text: `{"path":"a.py","content":"x"}`},
		{Kind: "call", Name: "read", Text: `{"path":"a.py"}`},
		{Kind: "call", Name: "bash", Text: `{"command":"pytest -q"}`},
		{Kind: "call", Name: "bash", Text: `{"command":"pytest -q"}`},
	}
	notes := tomoNotes(builtinProfiles["tomo"], steps)
	joined := strings.Join(notes, " | ")
	if !strings.Contains(joined, "re-read a file") {
		t.Errorf("expected read-after-write note, got: %q", joined)
	}
	if !strings.Contains(joined, "re-ran an identical shell command") {
		t.Errorf("expected repeated-command note, got: %q", joined)
	}
}

// pi has no dedicated search tool, so it greps and finds through bash. analyze
// should reclassify those bash calls as searches while leaving pytest and git as
// real shell, and piNotes should report how much searching went through the shell.
func TestPiBashSearchReclassified(t *testing.T) {
	steps := []Step{
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
	s := analyze("pi", builtinProfiles["pi"], steps)
	if s.Searches != 2 {
		t.Errorf("grep and find should be searches, got Searches=%d", s.Searches)
	}
	if s.Shells != 1 {
		t.Errorf("only pytest should remain shell, got Shells=%d", s.Shells)
	}
	if !s.Verified {
		t.Errorf("pytest after the edit should count as verification")
	}
	// The reclassified bash searches carry their command into the walkthrough.
	if steps[0].Act != "search" || moveLine(steps[0]) != "searched: grep -rn _ConanIgnoreMatcher conan/" {
		t.Errorf("bash search move = %q (act %q)", moveLine(steps[0]), steps[0].Act)
	}
	if !strings.Contains(strings.Join(s.Notes, " | "), "searching through the shell (2 commands)") {
		t.Errorf("piNotes should count shell searches, got: %v", s.Notes)
	}
}

// claude-code solves through subagents, so much of its transcript is Agent,
// SendMessage, and TaskOutput calls that coordinate other agents rather than
// touch the code. claudeNotes should surface that coordination as a share of the
// total, and the walkthrough should name those calls plainly.
func TestClaudeNotesFlagsOrchestration(t *testing.T) {
	steps := []Step{
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
	s := analyze("claude-code", builtinProfiles["claude-code"], steps)
	// grep-through-Bash counts as a search; the three orchestration calls land in other.
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
	if moveLine(steps[2]) != "dispatched a subagent" || moveLine(steps[6]) != "collected a subagent's output" {
		t.Errorf("orchestration moves not named: %q / %q", moveLine(steps[2]), moveLine(steps[6]))
	}
}

// The walkthrough should group a fix into Investigate, Fix, and Verify phases and
// clip long lines unless full is set.
func TestWalkthroughPhasesAndClipping(t *testing.T) {
	long := strings.Repeat("x", 400)
	steps := []Step{
		{Kind: "call", Name: "grep", Act: "search", Text: `{"pattern":"foo"}`},
		{Kind: "result", Text: "foo.py:1"},
		{Kind: "call", Name: "edit", Act: "edit", Text: `{"path":"foo.py"}`},
		{Kind: "result", Text: "edited foo.py"},
		{Kind: "call", Name: "bash", Act: "shell", Text: `{"command":"echo ` + long + `"}`},
		{Kind: "result", Text: "ok"},
	}
	var clipped, whole strings.Builder
	writeWalkthrough(&clipped, steps, false)
	writeWalkthrough(&whole, steps, true)

	for _, phase := range []string{"Investigate", "Fix", "Verify"} {
		if !strings.Contains(clipped.String(), phase) {
			t.Errorf("walkthrough missing phase %q:\n%s", phase, clipped.String())
		}
	}
	if !strings.Contains(clipped.String(), "…") {
		t.Errorf("expected a clipped line:\n%s", clipped.String())
	}
	if !strings.Contains(whole.String(), long) {
		t.Errorf("full walkthrough should carry the whole command")
	}
}
