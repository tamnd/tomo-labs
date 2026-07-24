package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/tomo-labs/pkg/trace"
)

// SweliveRun names a finished swelive container attempt to ingest: the raw run
// directory the harness produced (holding trace/ and grade/), and the metadata
// the harness knows but the files do not carry, so the ingest can label the run
// without re-deriving it.
type SweliveRun struct {
	Src      string // the run dir with trace/ and grade/, e.g. $ROOT/runs/$SLUG/$LABEL
	Tool     string // codex, pi, tomo-oi
	Scenario string // the task slug, e.g. dynaconf__dynaconf-1225
	Model    string // the served model, e.g. gpt-5.6-luna
	Runtime  string // docker, podman; defaults to docker
}

// IngestSwelive turns a raw swelive run directory into the labs data layout under
// dataRoot and returns the created run directory, ready for PublishRun. A swelive
// run records its conversation as a bridgetrace and grades offline into grade/,
// but it never writes the result.json the publisher indexes on, so its traces are
// invisible to the dataset until this runs. This reads the grade lists, the token
// usage, and the wall-and-rss timing, synthesizes the result.json the publisher
// expects, and copies the whole trace under attempt-1/trace so the bridgetrace is
// self-contained for reconstruction and any later audit.
func IngestSwelive(dataRoot string, r SweliveRun) (string, error) {
	if r.Runtime == "" {
		r.Runtime = "docker"
	}
	traceSrc := filepath.Join(r.Src, "trace")
	if fi, err := os.Stat(traceSrc); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("ingest: no trace/ under %s", r.Src)
	}

	f2p := readTestList(filepath.Join(r.Src, "grade", "f2p.json"))
	p2p := readTestList(filepath.Join(r.Src, "grade", "p2p.json"))
	log, _ := os.ReadFile(filepath.Join(r.Src, "grade", "test.log"))
	graded := len(f2p) > 0 || len(p2p) > 0
	passed, _, _, check := gradeSwelive(string(log), f2p, p2p)

	tokens, calls, firstTS := sumUsage(filepath.Join(traceSrc, "usage.jsonl"))
	wall, _, _ := parseTimeV(readString(filepath.Join(traceSrc, "time.txt")))

	// The run id names the trace file and orders the board, so it must be stable
	// across re-ingests of the same run: derive it from the first usage timestamp,
	// falling back to the exit_code file's mtime, so re-running ingest is idempotent
	// rather than minting a new run each time.
	runID := runIDFrom(firstTS, filepath.Join(traceSrc, "exit_code"))

	exitCode := 0
	if !passed {
		exitCode = 1
	}

	res := Result{
		Tool:        r.Tool,
		Scenario:    r.Scenario,
		Time:        runID,
		Model:       r.Model,
		Runtime:     r.Runtime,
		Passed:      passed,
		ExitCode:    exitCode,
		Attempts:    1,
		AttemptsMax: 1,
		WallSeconds: wall,
		Requests:    calls,
		Tokens:      tokens,
		Latency:     Latency{Calls: calls},
		Orchestration: Orchestration{
			ModelCalls: calls,
		},
		// CostUSD is deliberately left zero and omitted: a subscription-bridge model
		// is unmetered per token, so its dollar cost is unknown, never zero.
		Check:    check,
		Ungraded: !graded,
	}

	runDir := filepath.Join(dataRoot, "evals", "swebench-live", r.Tool, r.Scenario, runID)
	traceDst := filepath.Join(runDir, "attempt-1", "trace")
	if err := os.MkdirAll(traceDst, 0o755); err != nil {
		return "", err
	}
	if err := copyTree(traceSrc, traceDst); err != nil {
		return "", fmt.Errorf("ingest: copy trace: %w", err)
	}

	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), raw, 0o644); err != nil {
		return "", err
	}

	// Write the canonical session file to the codex-style date tree, the standard
	// self-describing artifact downstream reads, from the trace just copied in.
	// The run id is the timestamp, so the day tree orders by when the run happened.
	header := Run{Result: res, Eval: "swebench-live", RunID: runID}.meta()
	if _, err := trace.WriteSession(dataRoot, traceDst, header); err != nil {
		return "", fmt.Errorf("ingest: write session: %w", err)
	}
	return runDir, nil
}

// gradeSwelive reads the graded test.log and reports whether the run passed, how
// many FAIL_TO_PASS tests went green, how many PASS_TO_PASS tests regressed, and
// a one-line check string. A collapse that prints no per-test line, such as an
// ImportError that aborts collection, leaves every graded test un-passed, which is
// the correct zero.
func gradeSwelive(log string, f2p, p2p []string) (passed bool, npass, nreg int, check string) {
	status := map[string]string{}
	re := regexp.MustCompile(`(?m)^(PASSED|FAILED|ERROR)\s+(\S+)`)
	for _, m := range re.FindAllStringSubmatch(log, -1) {
		status[m[2]] = m[1]
	}
	for _, t := range f2p {
		if status[t] == "PASSED" {
			npass++
		}
	}
	for _, t := range p2p {
		if status[t] != "PASSED" {
			nreg++
		}
	}
	passed = len(f2p) > 0 && npass == len(f2p) && nreg == 0
	if passed {
		return true, npass, nreg, "PASS: all hidden tests satisfied"
	}
	return false, npass, nreg,
		fmt.Sprintf("FAIL: %d/%d FAIL_TO_PASS green, %d PASS_TO_PASS regressed", npass, len(f2p), nreg)
}

// usageRow is the subset of a proxy usage.jsonl row the ingest reads.
type usageRow struct {
	TS    float64 `json:"ts"`
	Usage *struct {
		Prompt        int `json:"prompt_tokens"`
		Completion    int `json:"completion_tokens"`
		Total         int `json:"total_tokens"`
		CacheHit      int `json:"prompt_cache_hit_tokens"`
		CacheWrite    int `json:"cache_write_tokens"`
		PromptDetails struct {
			Cached int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionDeets struct {
			Reasoning int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

// sumUsage totals the token counts over a usage.jsonl, skipping rows that carry no
// usage object (a non-200 upstream response), and returns the totals, the count of
// billed calls, and the first row's timestamp for run-id derivation.
func sumUsage(path string) (Tokens, int, float64) {
	var tok Tokens
	calls := 0
	firstTS := 0.0
	data, err := os.ReadFile(path)
	if err != nil {
		return tok, 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row usageRow
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		if firstTS == 0 && row.TS > 0 {
			firstTS = row.TS
		}
		if row.Usage == nil {
			continue
		}
		calls++
		tok.Prompt += row.Usage.Prompt
		tok.Completion += row.Usage.Completion
		tok.Total += row.Usage.Total
		cached := row.Usage.CacheHit
		if cached == 0 {
			cached = row.Usage.PromptDetails.Cached
		}
		tok.Cached += cached
		tok.CacheWrite += row.Usage.CacheWrite
		tok.Reasoning += row.Usage.CompletionDeets.Reasoning
	}
	return tok, calls, firstTS
}

// parseTimeV pulls the wall-clock seconds, the raw elapsed string, and the peak
// rss from a /usr/bin/time -v block.
func parseTimeV(txt string) (wallSeconds int, elapsed string, rssKB int) {
	if m := regexp.MustCompile(`Elapsed .*?:\s*([0-9:.]+)`).FindStringSubmatch(txt); m != nil {
		elapsed = m[1]
		wallSeconds = clockSeconds(elapsed)
	}
	if m := regexp.MustCompile(`Maximum resident set size \(kbytes\):\s*(\d+)`).FindStringSubmatch(txt); m != nil {
		rssKB, _ = strconv.Atoi(m[1])
	}
	return wallSeconds, elapsed, rssKB
}

// clockSeconds parses an h:mm:ss.ss, mm:ss.ss, or ss.ss elapsed string to whole
// seconds.
func clockSeconds(s string) int {
	parts := strings.Split(s, ":")
	secs := 0.0
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0
		}
		secs = secs*60 + v
	}
	return int(secs)
}

// runIDFrom builds the UTC run-id stamp from the first usage timestamp, falling
// back to the mtime of a witness file, so the id is stable across re-ingests.
func runIDFrom(firstTS float64, witness string) string {
	var t time.Time
	switch {
	case firstTS > 0:
		t = time.Unix(int64(firstTS), 0).UTC()
	default:
		if fi, err := os.Stat(witness); err == nil {
			t = fi.ModTime().UTC()
		} else {
			t = time.Now().UTC()
		}
	}
	return t.Format("20060102T150405Z")
}

func readTestList(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	_ = json.Unmarshal(data, &out)
	return out
}

func readString(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

// copyTree copies a directory tree, files and subdirectories, from src to dst.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
