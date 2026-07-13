package claude

import (
	"strings"
	"testing"
)

func TestLeakFetchCatchesPRFetch(t *testing.T) {
	s := parseSample(t)
	hits := s.LeakFetch()
	if len(hits) != 1 {
		t.Fatalf("leaks = %d, want 1", len(hits))
	}
	if hits[0].PR != "1225" {
		t.Errorf("PR = %q, want 1225", hits[0].PR)
	}
	if !strings.Contains(hits[0].Command, "gh pr diff 1225") {
		t.Errorf("command = %q, want the gh pr diff", hits[0].Command)
	}
	if s.Summarize().Clean() {
		t.Error("Clean() = true on a session that fetched a PR, want false")
	}
}

func TestLeakInCommand(t *testing.T) {
	cases := []struct {
		cmd  string
		leak bool
		pr   string
	}{
		{"gh pr diff 1225 --repo dynaconf/dynaconf", true, "1225"},
		{"gh pr view 1204 --repo dynaconf/dynaconf --json title", true, "1204"},
		{"curl -sL https://github.com/dynaconf/dynaconf/pull/1225.diff", true, "1225"},
		{"gh api repos/dynaconf/dynaconf/pulls/1225", true, "1225"},
		// git fetch of an explicit GitHub URL is a fetch aimed at the forge.
		{"git fetch https://github.com/dynaconf/dynaconf refs/pull/1225/head", true, "1225"},
		// git fetch of a bare origin names no host, so it is left alone: the
		// detector needs both a network verb and a GitHub mention.
		{"git fetch origin && git log", false, ""},
		// A local grep for a PR number is not a fetch: no network verb.
		{"grep -rn 'pull/1225' .", false, ""},
		// A fetch that does not name GitHub is left alone.
		{"curl -sL https://pypi.org/simple/dynaconf/", false, ""},
		// An ordinary build command is not a leak.
		{"python -m pytest tests/", false, ""},
	}
	for _, c := range cases {
		h, ok := leakInCommand(c.cmd)
		if ok != c.leak {
			t.Errorf("leakInCommand(%q) = %v, want %v", c.cmd, ok, c.leak)
			continue
		}
		if ok && h.PR != c.pr {
			t.Errorf("leakInCommand(%q) PR = %q, want %q", c.cmd, h.PR, c.pr)
		}
	}
}

func TestLeakFetchCleanSession(t *testing.T) {
	// An assistant turn whose only Bash call is a local test run is clean.
	const clean = `
{"type":"system","timestamp":"2026-07-13T12:00:00.000Z","sessionId":"s","cwd":"/w","version":"2.1.206"}
{"type":"assistant","timestamp":"2026-07-13T12:00:05.000Z","message":{"role":"assistant","model":"claude-haiku-4-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"python -m pytest tests/test_settings_loader.py"}}]}}
`
	s, err := ParseSession(strings.NewReader(clean))
	if err != nil {
		t.Fatal(err)
	}
	if hits := s.LeakFetch(); len(hits) != 0 {
		t.Errorf("leaks = %v, want none on a clean run", hits)
	}
	if !s.Summarize().Clean() {
		t.Error("Clean() = false on a clean run, want true")
	}
}

func TestLeakFetchDedupes(t *testing.T) {
	// The same fetch issued twice is reported once, so a report is not padded by
	// retries of the identical command.
	const dup = `
{"type":"system","timestamp":"2026-07-13T12:00:00.000Z","sessionId":"s","cwd":"/w","version":"2.1.206"}
{"type":"assistant","timestamp":"2026-07-13T12:00:05.000Z","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"gh pr diff 1225 --repo dynaconf/dynaconf"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"gh pr diff 1225 --repo dynaconf/dynaconf"}}]}}
`
	s, err := ParseSession(strings.NewReader(dup))
	if err != nil {
		t.Fatal(err)
	}
	if hits := s.LeakFetch(); len(hits) != 1 {
		t.Errorf("leaks = %d, want 1 after dedup", len(hits))
	}
}
