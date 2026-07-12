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
