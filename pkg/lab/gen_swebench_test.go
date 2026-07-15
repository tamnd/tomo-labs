package lab

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRepoShort(t *testing.T) {
	cases := map[string]string{
		"psf/requests":      "requests",
		"pylint-dev/pylint": "pylint",
		"django/django":     "django",
		"sympy/sympy":       "sympy",
		"NoSlash":           "noslash",
	}
	for in, want := range cases {
		if got := repoShort(in); got != want {
			t.Errorf("repoShort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSweStrList(t *testing.T) {
	// The dataset ships the test lists as a JSON string holding an array.
	stringy := json.RawMessage(`"[\"a::t1\", \"b::t2\"]"`)
	got := sweStrList(stringy)
	if len(got) != 2 || got[0] != "a::t1" || got[1] != "b::t2" {
		t.Fatalf("stringy list = %v", got)
	}
	// A real JSON array decodes too.
	arr := json.RawMessage(`["x", "y"]`)
	if got := sweStrList(arr); len(got) != 2 || got[1] != "y" {
		t.Fatalf("array list = %v", got)
	}
	// Empty stays empty.
	if got := sweStrList(nil); got != nil {
		t.Fatalf("nil list = %v", got)
	}
}

func TestSweSlug(t *testing.T) {
	if got := sweSlug("sympy__sympy-12345"); got != "sympy__sympy-12345" {
		t.Errorf("clean id changed: %q", got)
	}
	if got := sweSlug("a/b c"); got != "a-b-c" {
		t.Errorf("dirty id = %q", got)
	}
}

func TestSweSkipAndGradeable(t *testing.T) {
	if !sweSkip["django"] {
		t.Error("django should be skipped")
	}
	if sweGradeable["django"] {
		t.Error("django is not gradeable under pytest")
	}
	if !sweGradeable["requests"] {
		t.Error("requests should be gradeable")
	}
}

func TestSweSpecFor(t *testing.T) {
	// requests installs non-editable on py3.9 with no era pins.
	if s := sweSpecFor("psf/requests", "2.4"); s.install != "." || s.python != "3.9" || len(s.pip) != 0 {
		t.Errorf("requests spec = %+v", s)
	}
	// flask splits by version: 2.0 pins Werkzeug on py3.9, later versions move up.
	f20 := sweSpecFor("pallets/flask", "2.0")
	if f20.python != "3.9" || f20.install != "-e ." || len(f20.pip) == 0 {
		t.Errorf("flask 2.0 spec = %+v", f20)
	}
	if f23 := sweSpecFor("pallets/flask", "2.3"); f23.python != "3.11" {
		t.Errorf("flask 2.3 python = %q, want 3.11", f23.python)
	}
	// sphinx installs the test extra so pytest can run its node ids directly.
	if s := sweSpecFor("sphinx-doc/sphinx", "5.0"); s.install != "-e .[test]" {
		t.Errorf("sphinx install = %q, want -e .[test]", s.install)
	}
	// Old pytest needs setuptools held below 81 so pkg_resources still imports.
	pt := sweSpecFor("pytest-dev/pytest", "5.0")
	if len(pt.pip) != 1 || pt.pip[0] != "setuptools<81" || pt.install != "-e ." {
		t.Errorf("pytest spec = %+v", pt)
	}
	// An unlisted repo (reached via --all) falls back to a plain editable install.
	if s := sweSpecFor("some/other", "1.0"); s.install != "-e ." || s.python == "" {
		t.Errorf("fallback spec = %+v", s)
	}
}

func TestSwePrompt(t *testing.T) {
	p := swePrompt(sweRow{Repo: "psf/requests", ProblemStatement: "the bug is X"})
	if !strings.Contains(p, "psf/requests") || !strings.Contains(p, "the bug is X") {
		t.Fatalf("prompt missing repo or statement:\n%s", p)
	}
	// The benchmark's own prompt says nothing about tests, so neither does ours:
	// the grader resets touched test files, and a test instruction only risks
	// chilling the model.
	for _, banned := range []string{"hidden test suite", "Do not edit or add tests", "test files"} {
		if strings.Contains(p, banned) {
			t.Errorf("prompt should carry no test instruction, found %q:\n%s", banned, p)
		}
	}
}
