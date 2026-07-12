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

// opencode's SWE-bench failure mode is analysis paralysis: it greps, reads the
// buggy source, reads the tests, and then never edits, so the source ships
// unchanged and the hidden tests fail. opencodeNotes should call out "read the
// bug, never wrote the fix", and separately flag a run that edits a test file
// when the task said to leave tests alone.
func TestOpencodeNotesFlagsNeverEdited(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "grep", Text: `{"pattern":"conanignore"}`},
		{Kind: "result", Text: "config_installer.py:16:"},
		{Kind: "call", Name: "read", Text: `{"filePath":"conan/internal/api/config/config_installer.py"}`},
		{Kind: "result", Text: "class _ConanIgnoreMatcher:"},
		{Kind: "call", Name: "read", Text: `{"filePath":"test/integration/command/config_test.py"}`},
		{Kind: "result", Text: "def test_config_install():"},
	}
	s := analyze("opencode", builtinProfiles["opencode"], steps)
	if s.Edits != 0 {
		t.Fatalf("this run made no edits, got Edits=%d", s.Edits)
	}
	joined := strings.Join(s.Notes, " | ")
	if !strings.Contains(joined, "never edited") {
		t.Errorf("opencodeNotes should flag the never-edited run, got: %v", s.Notes)
	}

	// A run that does edit, but edits a test file, gets the softer flag instead.
	edited := []Step{
		{Kind: "call", Name: "read", Text: `{"filePath":"config_installer.py"}`},
		{Kind: "result", Text: "class _ConanIgnoreMatcher:"},
		{Kind: "call", Name: "edit", Text: `{"filePath":"test/integration/command/config_test.py"}`},
		{Kind: "result", Text: "edited"},
	}
	se := analyze("opencode", builtinProfiles["opencode"], edited)
	if !strings.Contains(strings.Join(se.Notes, " | "), "edited a test file") {
		t.Errorf("opencodeNotes should flag editing a test file, got: %v", se.Notes)
	}
}

// A run that leaves the repo should be counted once whether it fetched through a
// web tool or a curl in the shell, and the distinct hosts it reached should be
// collected so many pages on one site collapse to one entry.
func TestAnalyzeCountsNetworkAndHosts(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "webfetch", Text: `{"url":"https://github.com/aws-cloudformation/cfn-lint/pull/3798"}`},
		{Kind: "result", Text: "diff --git ..."},
		{Kind: "call", Name: "webfetch", Text: `{"url":"https://github.com/aws-cloudformation/cfn-lint/pull/3798/files"}`},
		{Kind: "result", Text: "more diff"},
		{Kind: "call", Name: "bash", Text: `{"command":"curl -s https://raw.githubusercontent.com/x/y/main/z.py"}`},
		{Kind: "result", Text: "content"},
		{Kind: "call", Name: "read", Text: `{"path":"foo.py"}`},
		{Kind: "result", Text: "def foo(): pass"},
	}
	s := analyze("opencode", builtinProfiles["opencode"], steps)
	if s.Fetches != 3 {
		t.Fatalf("Fetches = %d, want 3 (two web tools and one curl)", s.Fetches)
	}
	if len(s.FetchHosts) != 2 {
		t.Fatalf("FetchHosts = %v, want two distinct hosts", s.FetchHosts)
	}
	if s.FetchHosts[0] != "github.com" || s.FetchHosts[1] != "raw.githubusercontent.com" {
		t.Errorf("hosts = %v", s.FetchHosts)
	}
	// The curl is still a shell command, so it is not miscounted as a search.
	if steps[4].Act != "shell" {
		t.Errorf("curl act = %q, want shell", steps[4].Act)
	}
	// A local read is not a fetch, even though its name contains a path.
	if moveLine(steps[0]) != "fetched https://github.com/aws-cloudformation/cfn-lint/pull/3798" {
		t.Errorf("web-tool move = %q", moveLine(steps[0]))
	}
	if moveLine(steps[4]) != "fetched https://raw.githubusercontent.com/x/y/main/z.py" {
		t.Errorf("curl move = %q", moveLine(steps[4]))
	}
}

// fetch_file reads locally despite the "fetch" in its name, so it must not be
// counted as a network move.
func TestFetchFileIsNotNetwork(t *testing.T) {
	if isNetworkTool("fetch_file") {
		t.Errorf("fetch_file reads a local file, not the network")
	}
	if !isNetworkTool("webfetch") || !isNetworkTool("WebFetch") {
		t.Errorf("webfetch should be a network tool regardless of case")
	}
}

// An edit to a test tree should be split out of the source-file list, since the
// grader resets tests before grading and the fix has to land in the source.
func TestAnalyzeSplitsTestEdits(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "edit", Text: `{"path":"src/cfnlint/rules/x.py"}`},
		{Kind: "result", Text: "edited"},
		{Kind: "call", Name: "edit", Text: `{"path":"test/rules/test_x.py"}`},
		{Kind: "result", Text: "edited"},
	}
	s := analyze("tomo", builtinProfiles["tomo"], steps)
	if len(s.FilesEdit) != 1 || s.FilesEdit[0] != "src/cfnlint/rules/x.py" {
		t.Errorf("source edits = %v, want just the source file", s.FilesEdit)
	}
	if len(s.TestEdits) != 1 || s.TestEdits[0] != "test/rules/test_x.py" {
		t.Errorf("test edits = %v, want just the test file", s.TestEdits)
	}
}

// A run the upstream throttled should read as a floor, not the tool's best, so the
// narrative carries the rate-limit as its last word.
func TestNarrativeSurfacesThrottle(t *testing.T) {
	no := false
	t1 := &Transcript{
		Tool: "pi", Scenario: "cfn-lint", Passed: &no, Requests: 12, Tokens: 45814,
		Throttle: &RateLimit{Hits: 1, MaxRetryAfterS: 20291},
		Summary:  &RunSummary{Reads: 3},
	}
	joined := strings.Join(narrative(t1), " ")
	if !strings.Contains(joined, "rate-limited it 1 time") || !strings.Contains(joined, "20291") {
		t.Errorf("throttle not surfaced: %q", joined)
	}
	if !strings.Contains(joined, "floor") {
		t.Errorf("throttle caveat should frame the verdict as a floor: %q", joined)
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
