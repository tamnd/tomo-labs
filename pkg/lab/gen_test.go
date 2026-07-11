package lab

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// A suite reads its tasks and lands its results in its own subtree, so a public
// dataset never mixes into the core comparison and one suite never mixes into
// another. The core suite keeps the bare roots so nothing about the old layout
// moves.
func TestSuitePaths(t *testing.T) {
	root, data := "/repo", "/data"
	cases := []struct {
		suite, tasks, results, sdir string
	}{
		{"", "/repo/scenarios", "/data", "/repo/evals"},
		{"aider", "/repo/evals/aider/tasks", "/data/evals/aider", "/repo/evals/aider"},
		{"evalplus", "/repo/evals/evalplus/tasks", "/data/evals/evalplus", "/repo/evals/evalplus"},
	}
	for _, c := range cases {
		l := &Lab{cfg: Config{Root: root, Data: data, Suite: c.suite}}
		if got := l.tasksDir(); got != filepath.FromSlash(c.tasks) {
			t.Errorf("suite %q tasksDir = %s, want %s", c.suite, got, c.tasks)
		}
		if got := l.resultsDir(); got != filepath.FromSlash(c.results) {
			t.Errorf("suite %q resultsDir = %s, want %s", c.suite, got, c.results)
		}
		if got := l.suiteDir(); got != filepath.FromSlash(c.sdir) {
			t.Errorf("suite %q suiteDir = %s, want %s", c.suite, got, c.sdir)
		}
	}
}

// The core report walks the data root but skips the evals/ subtree, so a suite run
// never leaks into the core table. A suite report walks only its own subtree.
func TestWalkResultsIsolatesSuites(t *testing.T) {
	data := t.TempDir()
	writeResultAt(t, filepath.Join(data, "tomo", "01-core", "t1"), "tomo", "01-core")
	writeResultAt(t, filepath.Join(data, "evals", "aider", "tomo", "go-bowling", "t1"), "tomo", "go-bowling")
	writeResultAt(t, filepath.Join(data, "evals", "evalplus", "tomo", "humaneval-0", "t1"), "tomo", "humaneval-0")

	seen := func(suite string) []string {
		l := &Lab{cfg: Config{Data: data, Suite: suite}}
		var names []string
		_ = l.walkResults(func(_ string, r *Result) { names = append(names, r.Scenario) })
		return names
	}

	if got := seen(""); len(got) != 1 || got[0] != "01-core" {
		t.Errorf("core walk = %v, want [01-core] only", got)
	}
	if got := seen("aider"); len(got) != 1 || got[0] != "go-bowling" {
		t.Errorf("aider walk = %v, want [go-bowling] only", got)
	}
	if got := seen("evalplus"); len(got) != 1 || got[0] != "humaneval-0" {
		t.Errorf("evalplus walk = %v, want [humaneval-0] only", got)
	}
}

func writeResultAt(t *testing.T, dir, tool, scenario string) {
	t.Helper()
	b, err := json.Marshal(Result{Tool: tool, Scenario: scenario})
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "result.json", string(b))
}
