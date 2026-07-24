package publish

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestIngestSwelive synthesizes a run from a raw swelive layout and checks the
// result.json the publisher indexes on: the grade is read from test.log against
// the FAIL_TO_PASS and PASS_TO_PASS lists, the tokens are summed from usage.jsonl
// skipping a non-200 row, the wall clock is parsed from time.txt, the run id is
// derived from the first usage timestamp so it is stable, the bridgetrace is
// copied under attempt-1/trace, and cost is absent because the model is unmetered.
func TestIngestSwelive(t *testing.T) {
	src := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, p, content)
	}

	// Two FAIL_TO_PASS, one green one red; one PASS_TO_PASS staying green.
	mk("grade/f2p.json", `["tests/x.py::test_a","tests/x.py::test_b"]`)
	mk("grade/p2p.json", `["tests/x.py::test_keep"]`)
	mk("grade/test.log", "PASSED tests/x.py::test_a\nFAILED tests/x.py::test_b\nPASSED tests/x.py::test_keep\n")
	// Three usage rows, the middle one a non-200 with no usage object.
	mk("trace/usage.jsonl",
		`{"ts":1784743872.8,"status":200,"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_cache_hit_tokens":20,"completion_tokens_details":{"reasoning_tokens":5}}}`+"\n"+
			`{"ts":1784743875.0,"status":429}`+"\n"+
			`{"ts":1784743880.0,"status":200,"usage":{"prompt_tokens":200,"completion_tokens":20,"total_tokens":220,"prompt_tokens_details":{"cached_tokens":40}}}`+"\n")
	mk("trace/time.txt", "\tElapsed (wall clock) time (h:mm:ss or m:ss): 2:05.20\n\tMaximum resident set size (kbytes): 122912\n")
	mk("trace/exit_code", "0")
	mk("trace/bridgetrace/0001.req.json", `{"instructions":"sys","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]}]}`)

	dataRoot := t.TempDir()
	runDir, err := IngestSwelive(dataRoot, SweliveRun{
		Src:      src,
		Tool:     "codex",
		Scenario: "dynaconf__dynaconf-1225",
		Model:    "gpt-5.6-luna",
	})
	if err != nil {
		t.Fatalf("IngestSwelive: %v", err)
	}

	// The run id derives from the first usage ts (2026-07-22 in UTC), so the run
	// lands under evals/swebench-live/codex/<scenario>/<runid>.
	wantDir := filepath.Join(dataRoot, "evals", "swebench-live", "codex", "dynaconf__dynaconf-1225", "20260722T181112Z")
	if runDir != wantDir {
		t.Fatalf("run dir = %s, want %s", runDir, wantDir)
	}

	raw, err := os.ReadFile(filepath.Join(runDir, "result.json"))
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if res.Tool != "codex" || res.Model != "gpt-5.6-luna" || res.Runtime != "docker" {
		t.Fatalf("labels wrong: %+v", res)
	}
	if res.Passed {
		t.Fatalf("run passed despite a red FAIL_TO_PASS: %+v", res)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.ExitCode)
	}
	if res.WallSeconds != 125 {
		t.Fatalf("wall = %d, want 125", res.WallSeconds)
	}
	if res.Requests != 2 {
		t.Fatalf("requests = %d, want 2 (non-200 row skipped)", res.Requests)
	}
	if res.Tokens.Total != 330 || res.Tokens.Prompt != 300 || res.Tokens.Reasoning != 5 {
		t.Fatalf("tokens wrong: %+v", res.Tokens)
	}
	if res.Tokens.Cached != 60 {
		t.Fatalf("cached = %d, want 60 (20 hit + 40 details)", res.Tokens.Cached)
	}
	if res.CostUSD != 0 {
		t.Fatalf("cost must be absent for an unmetered model, got %v", res.CostUSD)
	}
	if res.Check != "FAIL: 1/2 FAIL_TO_PASS green, 0 PASS_TO_PASS regressed" {
		t.Fatalf("check = %q", res.Check)
	}

	// The bridgetrace copied under the run's trace dir, so the publisher can
	// reconstruct without reaching back to the source.
	if _, err := os.Stat(filepath.Join(runDir, "attempt-1", "trace", "bridgetrace", "0001.req.json")); err != nil {
		t.Fatalf("bridgetrace not copied: %v", err)
	}

	// Re-ingesting the same run is idempotent: same run id, no second directory.
	runDir2, err := IngestSwelive(dataRoot, SweliveRun{Src: src, Tool: "codex", Scenario: "dynaconf__dynaconf-1225", Model: "gpt-5.6-luna"})
	if err != nil || runDir2 != runDir {
		t.Fatalf("re-ingest not idempotent: %s vs %s (%v)", runDir2, runDir, err)
	}
}

// TestGradeSweliveCollapse checks that a collection-abort log with no per-test
// line grades every FAIL_TO_PASS as red and every PASS_TO_PASS as regressed,
// which is the correct zero for an import that breaks the suite.
func TestGradeSweliveCollapse(t *testing.T) {
	log := "ImportError: cannot import name 'LazySettings'\n"
	f2p := []string{"tests/x.py::test_a", "tests/x.py::test_b"}
	p2p := []string{"tests/x.py::test_keep"}
	passed, npass, nreg, check := gradeSwelive(log, f2p, p2p)
	if passed || npass != 0 || nreg != 1 {
		t.Fatalf("collapse not zeroed: passed=%v npass=%d nreg=%d", passed, npass, nreg)
	}
	if check != "FAIL: 0/2 FAIL_TO_PASS green, 1 PASS_TO_PASS regressed" {
		t.Fatalf("check = %q", check)
	}
}
