package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GenOptions tunes how much of a benchmark a generator pulls and whether it
// proves each task before keeping it.
type GenOptions struct {
	Langs      []string // aider: which language tracks to render; empty means its default
	Limit      int      // per-track sample size when All is false
	All        bool     // take every problem the benchmark offers
	NoValidate bool     // skip the reference-solution proof (inspection only)
	Difficulty []string // livecodebench: keep only these difficulty tiers (easy/medium/hard); empty takes any
}

// Generate materializes a public benchmark into the active suite's tasks/ dir.
// The suite selects which benchmark: "aider" rebuilds the Aider polyglot
// exercises, "evalplus" rebuilds HumanEval+ and MBPP+. It fetches upstream over
// HTTP, writes each problem into the lab task shape (prompt.txt, setup.sh,
// check.sh, plus the starting files), and unless NoValidate is set proves each
// task by grading a known-good solution and dropping any that does not pass. It
// returns how many tasks it kept.
//
// A generator writes the graded artifact's answer where the harness cannot mount
// it (answers/ or oracle/ beside tasks/), so an agent only ever sees the prompt
// and its starting files.
func (l *Lab) Generate(ctx context.Context, opts GenOptions) (int, error) {
	switch l.cfg.Suite {
	case "aider":
		return l.genAider(ctx, opts)
	case "evalplus":
		return l.genEvalPlus(ctx, opts)
	case "livecodebench":
		return l.genLiveCodeBench(ctx, opts)
	case "swebench":
		return l.genSWEBench(ctx, opts)
	case "swebench-live":
		return l.genSWEBenchLive(ctx, opts)
	case "":
		return 0, fmt.Errorf("gen needs a suite: try --suite aider, --suite evalplus, --suite livecodebench, --suite swebench, or --suite swebench-live")
	default:
		return 0, fmt.Errorf("no generator for suite %q", l.cfg.Suite)
	}
}

// httpGet fetches a URL with a bounded timeout, tied to the run's context so a
// cancel stops the fetch. It is the one path a generator reaches the network
// through, so every fetch shares the same manners.
func httpGet(ctx context.Context, url string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return body, nil
}

func httpGetJSON(ctx context.Context, url string, into any) error {
	body, err := httpGet(ctx, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, into)
}

// writeFile writes data under a task tree, creating parent dirs. It takes the
// same string-or-bytes content a generator has on hand.
func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

// validateTask proves a generated task by running it end to end over a known-good
// solution: it lays the starting files with setup.sh, lets applyRef overlay the
// answer the way an ideal agent would, then grades with check.sh. It returns
// whether the task passed and the grader's combined output for a drop report. A
// task that cannot pass on a correct solution cannot be trusted to grade, so the
// caller drops it.
func validateTask(ctx context.Context, taskDir string, applyRef func(work string) error) (bool, string, error) {
	work, err := os.MkdirTemp("", "labgen-")
	if err != nil {
		return false, "", err
	}
	defer os.RemoveAll(work)

	setup := exec.CommandContext(ctx, "bash", filepath.Join(taskDir, "setup.sh"), work)
	if out, err := setup.CombinedOutput(); err != nil {
		return false, string(out), fmt.Errorf("setup.sh: %w", err)
	}
	if err := applyRef(work); err != nil {
		return false, "", err
	}
	check := exec.CommandContext(ctx, "bash", filepath.Join(taskDir, "check.sh"), work)
	out, err := check.CombinedOutput()
	return err == nil, string(out), nil
}

// overlay copies a reference solution's files into the work tree, the way an ideal
// agent would leave them, preserving each file's path relative to the answer root
// so a solution nested in a package lands where its tests expect it.
func overlay(ansDir, work string) error {
	return filepath.WalkDir(ansDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(ansDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(work, rel), data, 0o644)
	})
}

// concat flattens several string slices into one, skipping nils.
func concat(slices ...[]string) []string {
	var out []string
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

// trim caps a grader's output for a one-line drop report.
func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
