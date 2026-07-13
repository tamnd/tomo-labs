package inspect

import (
	"strings"
	"testing"
)

// The analysis tests use literal profiles rather than the built-in ones a tool
// registers, so this package stays testable on its own without importing the
// tools sub-package (which would be a cycle). The per-tool notes are tested where
// they live, in the tools package.
var (
	tomoProf     = ToolProfile{Lexicon: map[string]string{"grep": "search", "read": "read", "edit": "edit", "write": "edit", "bash": "shell"}}
	piProf       = ToolProfile{Lexicon: map[string]string{"read": "read", "edit": "edit", "bash": "shell"}}
	opencodeProf = ToolProfile{Lexicon: map[string]string{"grep": "search", "read": "read", "edit": "edit", "write": "edit", "bash": "shell", "webfetch": "other"}}
)

// A run's calls should classify through the lexicon: read is a read, grep a
// search, bash a shell, edit an edit; and the summary should count each and name
// the file that was edited. Analyze also tags each call with its action bucket for
// the walkthrough.
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
	s := Analyze("tomo", tomoProf, steps)
	if s.Searches != 1 || s.Reads != 1 || s.Edits != 1 || s.Shells != 1 {
		t.Fatalf("counts = %+v", s)
	}
	if len(s.FilesEdit) != 1 || s.FilesEdit[0] != "foo.py" {
		t.Errorf("files edited = %v", s.FilesEdit)
	}
	if !s.Verified {
		t.Errorf("running pytest after an edit should count as verification")
	}
	if steps[6].Act != "shell" || steps[0].Act != "search" {
		t.Errorf("acts not tagged: %q %q", steps[0].Act, steps[6].Act)
	}
}

// A tool with no dedicated search greps through bash; Analyze should reclassify
// those bash calls as searches while leaving pytest as real shell, and the
// reclassified search should carry its command into the walkthrough.
func TestBashSearchReclassified(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "bash", Text: `{"command":"grep -rn _ConanIgnoreMatcher conan/"}`},
		{Kind: "result", Text: "conan/internal/api/config/config_installer.py:15:"},
		{Kind: "call", Name: "bash", Text: `{"command":"cd /work && find . -name conftest.py"}`},
		{Kind: "result", Text: "./test/conftest.py"},
		{Kind: "call", Name: "edit", Text: `{"path":"config_installer.py"}`},
		{Kind: "result", Text: "edited config_installer.py"},
		{Kind: "call", Name: "bash", Text: `{"command":"cd /work && python -m pytest -q"}`},
		{Kind: "result", Text: "1 passed"},
	}
	s := Analyze("pi", piProf, steps)
	if s.Searches != 2 {
		t.Errorf("grep and find should be searches, got Searches=%d", s.Searches)
	}
	if s.Shells != 1 {
		t.Errorf("only pytest should remain shell, got Shells=%d", s.Shells)
	}
	if !s.Verified {
		t.Errorf("pytest after the edit should count as verification")
	}
	if steps[0].Act != "search" || moveLine(steps[0]) != "searched: grep -rn _ConanIgnoreMatcher conan/" {
		t.Errorf("bash search move = %q (act %q)", moveLine(steps[0]), steps[0].Act)
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
	s := Analyze("opencode", opencodeProf, steps)
	if s.Fetches != 3 {
		t.Fatalf("Fetches = %d, want 3 (two web tools and one curl)", s.Fetches)
	}
	if len(s.FetchHosts) != 2 {
		t.Fatalf("FetchHosts = %v, want two distinct hosts", s.FetchHosts)
	}
	if s.FetchHosts[0] != "github.com" || s.FetchHosts[1] != "raw.githubusercontent.com" {
		t.Errorf("hosts = %v", s.FetchHosts)
	}
	if steps[4].Act != "shell" {
		t.Errorf("curl act = %q, want shell", steps[4].Act)
	}
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
	s := Analyze("tomo", tomoProf, steps)
	if len(s.FilesEdit) != 1 || s.FilesEdit[0] != "src/cfnlint/rules/x.py" {
		t.Errorf("source edits = %v, want just the source file", s.FilesEdit)
	}
	if len(s.TestEdits) != 1 || s.TestEdits[0] != "test/rules/test_x.py" {
		t.Errorf("test edits = %v, want just the test file", s.TestEdits)
	}
}

// A run that only reads and digs git history without ever editing is the
// investigation-runaway shape: zero edits, a long no-edit streak, history reads
// counted, the answer-shortcut probe flagged, and the guard nudge that fired read
// off the injected message. These are the marks that used to need a hand read of
// the raw trace.
func TestAnalyzeFlagsInvestigationRunaway(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "bash", Text: `{"command":"git status --short && git log --oneline -8"}`},
		{Kind: "result", Text: "39acdee fix"},
		{Kind: "call", Name: "read", Text: `{"path":"loaders/__init__.py"}`},
		{Kind: "result", Text: "def settings_loader(): ..."},
		{Kind: "call", Name: "bash", Text: `{"command":"git fsck --unreachable | head; git log --all --grep=1204"}`},
		{Kind: "result", Text: "no such commit"},
		// The convergence guard steps in on a user-role message.
		{Kind: "user", Text: "You have taken many steps without editing any file. If you have found the cause, make your edit now."},
		{Kind: "call", Name: "read", Text: `{"path":"base.py"}`},
		{Kind: "result", Text: "class Settings: ..."},
	}
	s := Analyze("tomo", tomoProf, steps)
	if !s.ZeroEdits {
		t.Errorf("a run that never edits should be flagged ZeroEdits")
	}
	if s.HistoryReads != 2 {
		t.Errorf("two git-history commands should be counted, got %d", s.HistoryReads)
	}
	if s.HistoryProbes != 1 {
		t.Errorf("the --all --grep dig should be flagged a history probe, got %d", s.HistoryProbes)
	}
	if s.NoEditStreak != 4 {
		t.Errorf("the longest no-edit streak should be all four calls, got %d", s.NoEditStreak)
	}
	if len(s.GuardNudges) != 1 || s.GuardNudges[0] != "no-edit" {
		t.Errorf("the no-edit guard nudge should be read off the transcript, got %v", s.GuardNudges)
	}
}

// An edit made through the shell (a patch, a sed -i, a heredoc into a source file)
// is a real edit a write-tool count misses, so Analyze should bucket it as an edit,
// tally it under ShellEdits, and reset the no-edit streak.
func TestAnalyzeCountsShellEdits(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "read", Text: `{"path":"loaders/__init__.py"}`},
		{Kind: "result", Text: "def settings_loader(): ..."},
		{Kind: "call", Name: "bash", Text: `{"command":"apply_patch <<'EOF'\n*** Update File: loaders/__init__.py\nEOF"}`},
		{Kind: "result", Text: "patched"},
		{Kind: "call", Name: "bash", Text: `{"command":"sed -i 's/foo/bar/' base.py"}`},
		{Kind: "result", Text: ""},
	}
	s := Analyze("tomo", tomoProf, steps)
	if s.Edits != 2 || s.ShellEdits != 2 {
		t.Errorf("two shell edits should count as edits and shell edits, got Edits=%d ShellEdits=%d", s.Edits, s.ShellEdits)
	}
	if s.ZeroEdits {
		t.Errorf("a run that edits through the shell is not a zero-edit run")
	}
	if steps[2].Act != "edit" {
		t.Errorf("an apply_patch should be bucketed as an edit, got %q", steps[2].Act)
	}
}

// The walkthrough is the per-step deep dive: it should tag a move for what it
// really was (git-history, network, shell-edit) and surface a harness event, the
// convergence-guard nudge, inline where it fired rather than dropping the message.
func TestWalkthroughTagsAndHarnessEvents(t *testing.T) {
	steps := []Step{
		{Kind: "call", Name: "bash", Act: "shell", Text: `{"command":"git log --all --grep=1204"}`},
		{Kind: "result", Text: "no hits"},
		{Kind: "user", Text: "You have taken many steps without editing any file."},
		{Kind: "call", Name: "bash", Act: "edit", Text: `{"command":"apply_patch <<'EOF'\n*** Update File: x.py\nEOF"}`},
		{Kind: "result", Text: "patched"},
	}
	var b strings.Builder
	writeWalkthrough(&b, steps, true)
	out := b.String()
	if !strings.Contains(out, "[history-probe]") {
		t.Errorf("the --all --grep dig should be tagged a history probe:\n%s", out)
	}
	if !strings.Contains(out, "⚑ harness: no-edit guard nudged the model") {
		t.Errorf("the guard nudge should surface as a harness event:\n%s", out)
	}
	if !strings.Contains(out, "[shell-edit]") {
		t.Errorf("the apply_patch should be tagged a shell edit:\n%s", out)
	}
}

// A run the upstream throttled should read as a floor, not the tool's best, so the
// narrative carries the rate-limit as its last word.
func TestNarrativeSurfacesThrottle(t *testing.T) {
	no := false
	t1 := &Transcript{
		Tool: "pi", Scenario: "cfn-lint", Passed: &no, Requests: 12, Tokens: 45814,
		Throttle: &Throttle{Hits: 1, MaxRetryAfterS: 20291},
		Summary:  &RunSummary{Reads: 3},
	}
	joined := strings.Join(Narrative(t1), " ")
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
