package lab

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/tomo-labs/pkg/container"
)

// slot is one worker's private set of container names and host port. Giving each
// concurrent run its own slot is what lets several tool/scenario runs be in
// flight at once without two of them colliding on a container name or on the
// published readiness port. Worker 0 keeps the bare names, so a single-worker
// run is byte-for-byte what the sequential harness did.
//
// The web sidecar is deliberately not part of the slot: it serves read-only
// static fixtures a scenario prompt names by a fixed hostname, so one shared
// instance backs every worker, started once for the sweep instead of per run.
type slot struct {
	proxy string
	run   string
	port  int
}

func newSlot(i, basePort int) slot {
	if i == 0 {
		return slot{proxy: proxyName, run: runName, port: basePort}
	}
	suf := "-" + strconv.Itoa(i)
	return slot{proxy: proxyName + suf, run: runName + suf, port: basePort + i}
}

// baseURL is the in-network address the tool points its OpenAI base at. It uses
// the proxy's container name and fixed port 8080, not the published host port,
// so parallel workers never contend on it.
func (s slot) baseURL() string { return "http://" + s.proxy + ":8080/v1" }

// RunOne drives one tool through one scenario and writes a result.json.
//
// It gives the tool up to cfg.Attempts tries and stops at the first pass. The
// retry is a general harness lever, not a per-scenario patch: the deterministic
// proxy strips client-side sampling variance, and a bounded best-of-N absorbs
// the residual run-to-run nondeterminism a hosted model still shows even under
// greedy decoding, so a capable agent lands a stable green without any coaching
// aimed at a specific task. Each attempt keeps its own trace under attempt-N/;
// the returned Result and the canonical result.json reflect the winning attempt,
// or the last one if none passed, and record how many tries it took.
func (l *Lab) RunOne(ctx context.Context, tool, scenarioName string) (*Result, error) {
	if err := l.rt.EnsureNetwork(ctx, l.cfg.Network); err != nil {
		return nil, err
	}
	if l.needWeb([]string{scenarioName}) {
		if err := l.startWeb(ctx); err != nil {
			return nil, err
		}
		defer l.rt.Remove(ctx, webName)
	}
	return l.runOn(ctx, tool, scenarioName, newSlot(0, l.cfg.ProxyPort))
}

// runOn is RunOne bound to a specific worker slot. RunAll calls it directly, one
// goroutine per slot; RunOne calls it on slot 0. It resolves the scenario by name
// and hands off to runScenario. It assumes the container network already exists,
// so the caller ensures it once rather than racing on it.
func (l *Lab) runOn(ctx context.Context, tool, scenarioName string, sl slot) (*Result, error) {
	sc, err := l.scenario(scenarioName)
	if err != nil {
		return nil, err
	}
	return l.runScenario(ctx, tool, sc, sl)
}

// runScenario drives one tool through one scenario on one worker slot. A graded
// scenario gets up to cfg.Attempts tries and stops at the first pass; an ungraded
// one (an ad-hoc prompt) has nothing to retry against, so it runs exactly once and
// captures the tool's answer instead of a pass or fail. Each attempt keeps its own
// trace under attempt-N/; the returned Result reflects the winning attempt, or the
// last one if none passed.
func (l *Lab) runScenario(ctx context.Context, tool string, sc Scenario, sl slot) (*Result, error) {
	if !l.rt.ImageExists(ctx, toolPrefix+tool) {
		return nil, fmt.Errorf("tool image missing, run: lab build %s", tool)
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	runDir := filepath.Join(l.resultsDir(), tool, sc.Name, ts)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, err
	}
	attempts := max(l.cfg.Attempts, 1)
	if !sc.graded {
		attempts = 1
	}

	var (
		work, trace                 string
		passed                      bool
		reason, answer              string
		wall, exitCode              int
		diskBefore, diskAfter, used int
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		used = attempt
		work = filepath.Join(runDir, fmt.Sprintf("attempt-%d", attempt), "work")
		trace = filepath.Join(runDir, fmt.Sprintf("attempt-%d", attempt), "trace")
		if err := os.MkdirAll(work, 0o755); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(trace, 0o755); err != nil {
			return nil, err
		}

		if err := l.runSetup(ctx, sc, work); err != nil {
			return nil, err
		}
		diskBefore = dirSizeKB(work)

		var err error
		wall, exitCode, err = l.runAttempt(ctx, tool, sc, work, trace, attempt, attempts, sl)
		if err != nil {
			return nil, err
		}

		if sc.graded {
			passed, reason = l.grade(ctx, sc, work)
		} else {
			answer = readAnswer(trace)
		}
		stripCaches(work)
		diskAfter = dirSizeKB(work)
		if passed {
			break
		}
	}

	m := readTrace(trace)
	installKB, _ := l.imageKB(tool)
	res := &Result{
		Tool: tool, Scenario: sc.Name, Time: ts,
		Model: l.cfg.Model, Runtime: l.rt.Bin,
		Passed: passed, ExitCode: exitCode,
		Attempts: used, AttemptsMax: attempts,
		WallSeconds: wall, ElapsedClock: m.ElapsedClock,
		MaxRSSKB: m.MaxRSSKB, Requests: m.Requests,
		Tokens: m.Tokens, Latency: m.Latency, CostUSD: m.CostUSD,
		Orchestration: m.Orch, RateLimit: m.RateLimit,
		DiskBeforeKB:  diskBefore, DiskAfterKB: diskAfter, DiskDeltaKB: diskAfter - diskBefore,
		InstallKB: installKB,
		Check:     firstLineOf(reason),
		Ungraded:  !sc.graded,
		Answer:    answer,
	}
	if err := writeResult(filepath.Join(runDir, "result.json"), res); err != nil {
		return nil, err
	}
	pruneOldRuns(filepath.Join(l.resultsDir(), tool, sc.Name), l.cfg.KeepRuns)
	l.printSummary(res)
	return res, nil
}

// runAttempt stands up the proxy and optional web sidecar, runs the tool in a
// throwaway container, and tears the sidecars back down. It returns the wall
// seconds the tool ran and the exit code it left behind. Every container it
// touches is named from the slot, so a run on another slot can proceed in
// parallel without stepping on it.
func (l *Lab) runAttempt(ctx context.Context, tool string, sc Scenario, work, trace string, attempt, attempts int, sl slot) (wall, exitCode int, err error) {
	l.rt.Remove(ctx, sl.proxy)
	l.rt.Remove(ctx, sl.run)

	if err = l.startProxy(ctx, trace, sl); err != nil {
		return 0, -1, err
	}
	defer l.rt.Remove(ctx, sl.proxy)
	if err = l.waitReady(ctx, sl.port); err != nil {
		fmt.Fprintln(os.Stderr, l.rt.Logs(ctx, sl.proxy))
		return 0, -1, fmt.Errorf("proxy never became ready: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[run] %s / %s (attempt %d/%d)\n", tool, sc.Name, attempt, attempts)
	start := time.Now()
	err = l.rt.Run(ctx, container.RunSpec{
		Name: sl.run, Image: toolPrefix + tool, Network: l.cfg.Network, Remove: true,
		Mounts: []container.Mount{
			{Host: work, Container: "/work"},
			{Host: trace, Container: "/trace"},
			{Host: sc.dir, Container: "/scenario", ReadOnly: true},
		},
		Env: []string{
			"LAB_BASE_URL=" + sl.baseURL(),
			"LAB_MODEL=" + l.cfg.Model,
			"OPENCODE_API_KEY=" + l.cfg.APIKey,
			"LAB_MAX_TURNS=" + strconv.Itoa(l.cfg.MaxTurns),
		},
		Stdout: os.Stdout, Stderr: os.Stderr,
	})
	wall = int(time.Since(start).Seconds())
	if err != nil {
		return wall, -1, err
	}
	return wall, l.readExitCode(trace), nil
}

func (l *Lab) startProxy(ctx context.Context, trace string, sl slot) error {
	det := "1"
	if !l.cfg.Deterministic {
		det = "0"
	}
	return l.rt.Run(ctx, container.RunSpec{
		Name: sl.proxy, Image: proxyImage, Network: l.cfg.Network, Detach: true,
		Mounts: []container.Mount{{Host: trace, Container: "/trace"}},
		Env: []string{
			"UPSTREAM=" + l.cfg.Upstream,
			"TRACE_DIR=/trace",
			"LAB_DETERMINISTIC=" + det,
			"LAB_TEMPERATURE=" + l.cfg.Temperature,
			"LAB_TOP_P=" + l.cfg.TopP,
			"LAB_SEED=" + l.cfg.Seed,
		},
		Publish: fmt.Sprintf("127.0.0.1:%d:8080", sl.port),
	})
}

// startWeb stands up the one shared web sidecar that serves scenario fixtures,
// under the fixed name a prompt refers to. It is idempotent within a sweep: the
// caller starts it once before running and tears it down after, and every
// worker's tool reaches the same instance over the network.
func (l *Lab) startWeb(ctx context.Context) error {
	l.rt.Remove(ctx, webName)
	return l.rt.Run(ctx, container.RunSpec{
		Name: webName, Image: baseImage, Network: l.cfg.Network, Detach: true,
		Mounts:  []container.Mount{{Host: filepath.Join(l.cfg.Root, "webroot"), Container: "/srv", ReadOnly: true}},
		Workdir: "/srv",
		Cmd:     []string{"python3", "-m", "http.server", "80"},
	})
}

// needWeb reports whether any of the named scenarios ships a web fixture dir, so
// the sweep only stands up the web sidecar when a scenario actually needs it.
func (l *Lab) needWeb(scenarios []string) bool {
	for _, name := range scenarios {
		if sc, err := l.scenario(name); err == nil && exists(filepath.Join(sc.dir, "web")) {
			return true
		}
	}
	return false
}

// waitReady polls the published proxy port until it answers with any HTTP
// status, which means the listener is up even if the upstream 404s a bare GET.
func (l *Lab) waitReady(ctx context.Context, port int) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	client := &http.Client{Timeout: 2 * time.Second}
	for range 40 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("no response after 20s")
}

// promptScenario builds a throwaway ungraded scenario from a prompt string. It
// lives in a temp dir that the returned cleanup removes, so an ad-hoc prompt run
// leaves nothing behind on the host outside the normal data dir. It is named
// "prompt" so its runs land under data/<tool>/prompt/ like any scenario.
func (l *Lab) promptScenario(prompt string) (Scenario, func(), error) {
	dir, err := os.MkdirTemp("", "tomolab-prompt-")
	if err != nil {
		return Scenario{}, func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	if err := os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(prompt), 0o644); err != nil {
		cleanup()
		return Scenario{}, func() {}, err
	}
	return Scenario{Name: "prompt", dir: dir, graded: false}, cleanup, nil
}

// runSetup lays down a scenario's fixtures into the work tree, if it ships a
// setup script.
func (l *Lab) runSetup(ctx context.Context, sc Scenario, work string) error {
	setup := filepath.Join(sc.dir, "setup.sh")
	if !exists(setup) {
		return nil
	}
	cmd := exec.CommandContext(ctx, "bash", setup, work)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setup %s: %w", sc.Name, err)
	}
	return nil
}

// grade runs the scenario's checker over the work tree on the host. Exit zero is
// a pass; the combined output is the reason either way.
func (l *Lab) grade(ctx context.Context, sc Scenario, work string) (bool, string) {
	check := filepath.Join(sc.dir, "check.sh")
	if !exists(check) {
		return false, "no check.sh"
	}
	out, err := exec.CommandContext(ctx, "bash", check, work).CombinedOutput()
	return err == nil, strings.TrimSpace(string(out))
}

func (l *Lab) readExitCode(trace string) int {
	b, err := os.ReadFile(filepath.Join(trace, "exit_code"))
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return -1
	}
	return n
}

func (l *Lab) printSummary(r *Result) {
	mark := "FAIL"
	switch {
	case r.Ungraded:
		mark = "DONE"
	case r.Passed:
		mark = "PASS"
	}
	note := firstLineOf(r.Check)
	if r.RateLimit != nil {
		// A throttled run leaves no tokens and no answer, so call it out plainly
		// rather than letting it read as a plain failure.
		tag := fmt.Sprintf("rate-limited x%d", r.RateLimit.Hits)
		if r.RateLimit.MaxRetryAfterS > 0 {
			tag += fmt.Sprintf(" (retry-after %ds)", r.RateLimit.MaxRetryAfterS)
		}
		if note == "" {
			note = tag
		} else {
			note = tag + "; " + note
		}
	}
	fmt.Fprintf(os.Stderr, "  %s  %-8s %-20s try=%d/%d tokens=%d reqs=%d plan=%d sub=%d rss=%.1fMB disk=+%dKB ttfb=%dms  %s\n",
		mark, r.Tool, r.Scenario, r.Attempts, r.AttemptsMax,
		r.Tokens.Total, r.Orchestration.ModelCalls, r.Orchestration.PlanCalls, r.Orchestration.Subagents,
		float64(r.MaxRSSKB)/1024, r.DiskDeltaKB, r.Latency.AvgTTFB,
		note)
}

// readAnswer reads a tool's stdout from a run's trace and returns its tail,
// trimmed and length-capped, as the answer to show for an ungraded prompt run.
// The tail is what matters: a coding agent narrates as it works and lands its
// final message last. The full stream stays in the trace for anyone who wants it.
func readAnswer(trace string) string {
	b, err := os.ReadFile(filepath.Join(trace, "stdout.log"))
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	const cap = 2000
	if len(s) > cap {
		s = "..." + s[len(s)-cap:]
	}
	return s
}

func firstLineOf(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}
