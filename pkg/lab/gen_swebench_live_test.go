package lab

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSweLivePytestRunnable(t *testing.T) {
	yes := [][]string{
		{"pytest -rA"},
		{"python -m pytest -rA -vv"},
		{"python3 -m pytest"},
	}
	for _, c := range yes {
		if !sweLivePytestRunnable(c) {
			t.Errorf("sweLivePytestRunnable(%v) = false, want true", c)
		}
	}
	no := [][]string{
		{"poetry run pytest -rA"},
		{"hatch run test:unit -rA -vv"},
		{"tox -e py311"},
		{"pytest -rA", "pytest tests/extra"}, // more than one command is ambiguous
		{},                                   // no command
	}
	for _, c := range no {
		if sweLivePytestRunnable(c) {
			t.Errorf("sweLivePytestRunnable(%v) = true, want false", c)
		}
	}
}

func TestSweTestFile(t *testing.T) {
	cases := map[string]string{
		"tests/test_state.py::test_dup":                          "tests/test_state.py",
		"test/unit/test_x.py::TestClass::test_method":            "test/unit/test_x.py",
		"test/unit/test_dr.py::test_validate[Invalid":            "test/unit/test_dr.py", // Live truncates the param, file stays whole
		"tests/test_dates.py::test_parse[minLength-schema62--e]": "tests/test_dates.py",
		"tests/test_plain.py":                                    "tests/test_plain.py", // no node id at all
	}
	for in, want := range cases {
		if got := sweTestFile(in); got != want {
			t.Errorf("sweTestFile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSweInFiles(t *testing.T) {
	files := map[string]bool{
		"tests/test_a.py": true,
		"tests/test_b.py": true,
	}
	ids := []string{
		"tests/test_a.py::t1",
		"tests/integration/test_big.py::t2", // different file, dropped
		"tests/test_b.py::t3[param]",
		"e2e/test_slow.py::t4", // dropped
	}
	got := sweInFiles(ids, files)
	want := []string{"tests/test_a.py::t1", "tests/test_b.py::t3[param]"}
	if len(got) != len(want) {
		t.Fatalf("sweInFiles kept %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sweInFiles[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSweLiveRowDecode(t *testing.T) {
	// Live ships FAIL_TO_PASS, PASS_TO_PASS, and test_cmds as real JSON arrays.
	raw := []byte(`{
		"instance_id":"acme__widget-42","repo":"acme/widget","base_commit":"abc123",
		"patch":"diff","test_patch":"tdiff","problem_statement":"boom","created_at":"2025-01-02",
		"test_cmds":["pytest -rA"],
		"FAIL_TO_PASS":["tests/test_w.py::t1"],
		"PASS_TO_PASS":["tests/test_w.py::t2","tests/test_x.py::t3"]
	}`)
	var row sweLiveRow
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	if row.InstanceID != "acme__widget-42" || row.Repo != "acme/widget" {
		t.Errorf("ids decoded wrong: %+v", row)
	}
	if cmds := sweStrList(row.TestCmds); len(cmds) != 1 || cmds[0] != "pytest -rA" {
		t.Errorf("test_cmds = %v", cmds)
	}
	if f2p := sweStrList(row.FailToPass); len(f2p) != 1 || f2p[0] != "tests/test_w.py::t1" {
		t.Errorf("FAIL_TO_PASS = %v", f2p)
	}
}

func TestSweLivePrompt(t *testing.T) {
	p := sweLivePrompt(sweLiveRow{Repo: "acme/widget", ProblemStatement: "the widget breaks"})
	if !strings.Contains(p, "acme/widget") || !strings.Contains(p, "the widget breaks") {
		t.Fatalf("prompt missing repo or statement:\n%s", p)
	}
	if !strings.Contains(p, "Do not edit or add tests") {
		t.Error("prompt should forbid editing tests")
	}
	// The two closed doors this harness holds shut are stated up front, so a tool
	// does not spend its opening rounds discovering them.
	if !strings.Contains(p, "no network access") {
		t.Error("prompt should state that the network is closed")
	}
	if !strings.Contains(p, "history is truncated") || !strings.Contains(p, "do not mine history") {
		t.Error("prompt should state that git history is truncated and not worth mining")
	}
}

func TestSweLiveKeepPerRepoCap(t *testing.T) {
	row := func(repo string) sweLiveRow {
		return sweLiveRow{Repo: repo, TestCmds: []byte(`["pytest -rA"]`)}
	}
	perRepo := map[string]int{}
	kept := 0
	// Twelve instances from one repo, but the cap bounds how many are kept.
	for i := 0; i < 12; i++ {
		if sweLiveKeep(row("acme/widget"), nil, perRepo, true, sweLivePerRepoCap) {
			kept++
		}
	}
	if kept != sweLivePerRepoCap {
		t.Fatalf("kept %d from one repo, want the cap %d", kept, sweLivePerRepoCap)
	}

	// A second repo gets its own budget.
	if !sweLiveKeep(row("other/thing"), nil, perRepo, true, sweLivePerRepoCap) {
		t.Fatal("a different repo should still be kept under its own cap")
	}

	// An uncapped (targeted) pull ignores the cap entirely.
	un := map[string]int{}
	keptUncapped := 0
	for i := 0; i < 12; i++ {
		if sweLiveKeep(row("acme/widget"), nil, un, false, sweLivePerRepoCap) {
			keptUncapped++
		}
	}
	if keptUncapped != 12 {
		t.Fatalf("uncapped pull kept %d, want all 12", keptUncapped)
	}

	// A per-repo override of 1 renders a one-task-per-repo, repo-diverse sample.
	one := map[string]int{}
	diverse := 0
	for i := 0; i < 5; i++ {
		if sweLiveKeep(row("acme/widget"), nil, one, true, 1) {
			diverse++
		}
	}
	if diverse != 1 {
		t.Fatalf("with per-repo cap 1, kept %d from one repo, want 1", diverse)
	}
}

func TestSweLiveKeepDropsUnrunnableAndUnmatched(t *testing.T) {
	perRepo := map[string]int{}
	// A non-pytest command is not host-runnable in this tier.
	tox := sweLiveRow{Repo: "acme/widget", TestCmds: []byte(`["tox -e py"]`)}
	if sweLiveKeep(tox, nil, perRepo, true, sweLivePerRepoCap) {
		t.Error("a non-pytest command should be dropped")
	}
	// With an only filter, a repo not named is dropped even if runnable.
	pt := sweLiveRow{Repo: "acme/widget", TestCmds: []byte(`["pytest -rA"]`)}
	only := map[string]bool{"other": true}
	if sweLiveKeep(pt, only, perRepo, false, sweLivePerRepoCap) {
		t.Error("a repo outside the only set should be dropped")
	}
	if sweLiveKeep(pt, map[string]bool{"widget": true}, perRepo, false, sweLivePerRepoCap) != true {
		t.Error("the named repo should be kept")
	}
}
