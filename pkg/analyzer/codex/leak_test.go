package codex

import (
	"encoding/json"
	"strings"
	"testing"
)

// rolloutWithCmds builds a minimal rollout whose items are exec_command tool
// calls carrying the given commands in the JS shim Codex actually writes, with
// each layer of JSON escaped exactly as Codex escapes it on disk.
func rolloutWithCmds(t *testing.T, cmds ...string) *Rollout {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"timestamp":"2026-07-13T12:00:00.000Z","type":"session_meta","payload":{"session_id":"s","cwd":"/w","cli_version":"1.0"}}` + "\n")
	for i, c := range cmds {
		arg, err := json.Marshal(map[string]string{"cmd": c, "workdir": "/w"})
		if err != nil {
			t.Fatal(err)
		}
		shim := "const r = await tools.exec_command(" + string(arg) + ");\ntext(JSON.stringify(r));"
		payload, err := json.Marshal(map[string]any{
			"type":  "custom_tool_call",
			"name":  "exec",
			"input": shim,
		})
		if err != nil {
			t.Fatal(err)
		}
		rec, err := json.Marshal(map[string]any{
			"timestamp": "2026-07-13T12:00:00.000Z",
			"type":      "response_item",
			"payload":   json.RawMessage(payload),
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = i
		b.Write(rec)
		b.WriteString("\n")
	}
	r, err := ParseRollout(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("ParseRollout: %v", err)
	}
	return r
}

func TestCommandExtractsFromShim(t *testing.T) {
	r := rolloutWithCmds(t, `pytest -q tests/test_base.py`, `git status --short`)
	got := r.Commands()
	if len(got) != 2 {
		t.Fatalf("commands = %v, want 2", got)
	}
	if got[0] != "pytest -q tests/test_base.py" {
		t.Errorf("cmd[0] = %q", got[0])
	}
}

func TestLeakScanNetworkDoor(t *testing.T) {
	r := rolloutWithCmds(t,
		`pytest -q`,
		`gh pr diff 1225 --repo dynaconf/dynaconf`,
	)
	hits := r.LeakScan()
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0].Door != DoorNetwork {
		t.Errorf("door = %q, want network", hits[0].Door)
	}
	if hits[0].PR != "1225" {
		t.Errorf("PR = %q, want 1225", hits[0].PR)
	}
	if r.Clean() {
		t.Error("Clean() = true on a run that fetched a PR")
	}
}

func TestLeakScanHistoryDoor(t *testing.T) {
	// The exact shapes the observed gpt-5.6-sol run used to mine the gold commit.
	r := rolloutWithCmds(t,
		`git log --all --oneline --grep='1225'`,
		`git diff --no-ext-diff --unified=3 39acdee..da0054e -- dynaconf`,
		`git log origin/master --oneline --decorate -30`,
	)
	hits := r.LeakScan()
	if len(hits) != 2 {
		t.Fatalf("history hits = %d, want 2 (the grep-all and the origin/ log): %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.Door != DoorHistory {
			t.Errorf("door = %q, want history for %q", h.Door, h.Command)
		}
	}
}

func TestLeakScanPackageDoor(t *testing.T) {
	// The exact shapes the winning gpt-5.6-terra offline run used to lift the fix
	// from the cached fixed release: find the archive, then diff the checkout
	// against it.
	r := rolloutWithCmds(t,
		`find /Users/apple/.cache/uv -type f -iname '*dynaconf*' | head`,
		`diff -u dynaconf/base.py /Users/apple/.cache/uv/archive-v0/KU9IGQ03/dynaconf/base.py`,
		`rg -n 'rsplit' /root/.cache/uv/archive-v0/x/dynaconf/loaders/__init__.py`,
	)
	hits := r.LeakScan()
	if len(hits) != 3 {
		t.Fatalf("package hits = %d, want 3: %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.Door != DoorPackage {
			t.Errorf("door = %q, want package for %q", h.Door, h.Command)
		}
	}
	if r.Clean() {
		t.Error("Clean() = true on a run that diffed against the cached release")
	}
}

func TestLeakScanCleanRun(t *testing.T) {
	r := rolloutWithCmds(t,
		`pytest -q tests/`,
		`git status --short`,
		`git diff --stat`,
		`grep -rn 'pull/1225' .`,                     // local grep, not a fetch
		`git log --oneline -5`,                       // local history, no remote ref, no grep-all
		`.venv/bin/python -m pytest tests/test_x.py`, // imports from site-packages but does not read it as a source
	)
	if hits := r.LeakScan(); len(hits) != 0 {
		t.Errorf("hits = %+v, want none on a clean run", hits)
	}
	if !r.Clean() {
		t.Error("Clean() = false on a clean run")
	}
}
