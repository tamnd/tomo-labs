package lab

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tamnd/tomo-labs/pkg/container"
)

// maxInfraRetries bounds how many times a run re-issues after an upstream fault
// (a dropped stream) without spending a capability attempt. It floors the infra
// tolerance so a pass@1 run, which has only one capability attempt, still shrugs
// off a few gateway hiccups instead of scoring them as the model failing.
const maxInfraRetries = 3

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
	prep  string
	port  int
}

func (l *Lab) newSlot(i, basePort int) slot {
	if i == 0 {
		return slot{proxy: l.cfg.proxyName(), run: l.cfg.runName(), prep: l.cfg.runName() + "-prep", port: basePort}
	}
	suf := "-" + strconv.Itoa(i)
	return slot{proxy: l.cfg.proxyName() + suf, run: l.cfg.runName() + suf, prep: l.cfg.runName() + "-prep" + suf, port: basePort + i}
}

// baseURL is the in-network address the tool points its OpenAI base at. It uses
// the proxy's container name and fixed port 8080, not the published host port,
// so parallel workers never contend on it.
func (s slot) baseURL() string { return "http://" + s.proxy + ":8080/v1" }

// RunOne drives one tool through one scenario and writes a result.json.
//
// By default cfg.Attempts is 1, so a scenario is scored on the single first-try
// result: pure pass@1, the metric the report ranks on. The proxy still forces
// greedy decoding so a rerun means the same thing, and an upstream fault (a
// dropped stream) is re-issued off the books so a gateway hiccup is never scored
// as the model failing. Raising LAB_ATTEMPTS turns on opt-in best-of-N, feeding a
// failing attempt back for another try; it is a general harness lever, not a
// per-scenario patch. Each attempt keeps its own trace under attempt-N/; the
// returned Result and the canonical result.json reflect the winning attempt, or
// the last one if none passed, and record how many tries it took.
func (l *Lab) RunOne(ctx context.Context, tool, scenarioName string) (*Result, error) {
	if err := l.rt.EnsureNetwork(ctx, l.cfg.Network); err != nil {
		return nil, err
	}
	if l.needWeb([]string{scenarioName}) {
		if err := l.startWeb(ctx); err != nil {
			return nil, err
		}
		defer l.rt.Remove(ctx, l.cfg.webName())
	}
	return l.runOn(ctx, tool, scenarioName, l.newSlot(0, l.cfg.ProxyPort))
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
// scenario gets up to cfg.Attempts capability tries and stops at the first pass;
// with the default of 1 that is a single first-try grade (pass@1). An ungraded
// scenario (an ad-hoc prompt) has nothing to retry against, so it runs exactly
// once and captures the tool's answer instead of a pass or fail. Each attempt
// keeps its own trace under attempt-N/; the returned Result reflects the winning
// attempt, or the last one if none passed.
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
		edited                      []string
		wall, exitCode              int
		diskBefore, diskAfter, used int
		infraSkips                  int
	)
	// A failed attempt whose upstream dropped a completion mid-stream is not the
	// agent's fault, so it does not spend one of the graded attempts: it is re-run
	// up to maxInfra extra times. That keeps the free tier's flakiness from being
	// scored as the tool failing the task, while a persistently broken upstream
	// still terminates the loop. graded counts the genuine attempts; iter names the
	// attempt dir so every try, discounted or not, keeps its own trace.
	//
	// The infra budget is deliberately independent of the capability budget: pass@1
	// runs with attempts==1, but a single gateway drop must not be scored as the
	// model failing, so it stays at least maxInfraRetries regardless.
	maxInfra := max(attempts, maxInfraRetries)
	graded := 0
	for iter := 1; ; iter++ {
		work = filepath.Join(runDir, fmt.Sprintf("attempt-%d", iter), "work")
		trace = filepath.Join(runDir, fmt.Sprintf("attempt-%d", iter), "trace")
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

		// Build the task's Python environment before the agent runs, so it starts
		// from a prepared venv the way canonical SWE-bench arranges rather than
		// spending tokens bootstrapping one. The env lives outside the work tree,
		// so it never lands in the graded diff or the disk footprint.
		env := filepath.Join(runDir, fmt.Sprintf("attempt-%d", iter), "env")
		l.prepEnv(ctx, sc, work, env, sl)

		var err error
		wall, exitCode, err = l.runAttempt(ctx, tool, sc, work, trace, env, iter, attempts, sl)
		if err != nil {
			return nil, err
		}

		if sc.graded {
			passed, reason, edited = l.grade(ctx, sc, work)
		} else {
			answer = readAnswer(trace)
		}
		stripCaches(work)
		diskAfter = dirSizeKB(work)
		if passed {
			graded++
			break
		}
		// A graded failure the upstream caused (a dropped stream) is thrown out and
		// retried without counting, up to the bound; anything else spends a real try.
		// A mid-run drop leaves a flagged latency row; a final-turn drop the pod tore
		// down before the proxy could flush leaves only a truncated resp file, so both
		// paths count as the infra fault they are.
		if sc.graded && infraSkips < maxInfra &&
			(streamErrorStats(filepath.Join(trace, "latency.jsonl")) != nil || droppedFinalStream(trace)) {
			infraSkips++
			continue
		}
		graded++
		if graded >= attempts {
			break
		}
	}
	used = graded

	m := readTrace(trace)
	// Fold the discounted infra retries into the run's stream-fail record, so a
	// clean winning attempt still shows the upstream drops the loop swallowed.
	if infraSkips > 0 {
		if m.StreamFail == nil {
			m.StreamFail = &StreamFail{}
		}
		m.StreamFail.RetriedAttempts = infraSkips
	}
	installKB, _ := l.imageKB(tool)
	res := &Result{
		Tool: tool, Scenario: sc.Name, Time: ts,
		Model: l.cfg.Model, Runtime: l.rt.Bin,
		Passed: passed, ExitCode: exitCode,
		Attempts: used, AttemptsMax: attempts,
		WallSeconds: wall, ElapsedClock: m.ElapsedClock,
		MaxRSSKB: m.MaxRSSKB, Requests: m.Requests,
		Tokens: m.Tokens, Latency: m.Latency, CostUSD: m.CostUSD,
		Orchestration: m.Orch, RateLimit: m.RateLimit, StreamFail: m.StreamFail,
		DiskBeforeKB: diskBefore, DiskAfterKB: diskAfter, DiskDeltaKB: diskAfter - diskBefore,
		InstallKB:   installKB,
		Check:       firstLineOf(reason),
		EditedTests: edited,
		Ungraded:    !sc.graded,
		Answer:      answer,
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
func (l *Lab) runAttempt(ctx context.Context, tool string, sc Scenario, work, trace, env string, attempt, attempts int, sl slot) (wall, exitCode int, err error) {
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
	// Bound the attempt by a wall clock. A tool that never ends its turn, or one
	// driven by a weak model into a loop that keeps calling tools without
	// converging, would otherwise burn tokens until the upstream cut it off.
	// When the ceiling fires the container is killed and whatever it left in the
	// work tree is graded, so a runaway scores as a failed attempt rather than
	// hanging the sweep. A zero timeout opts out.
	runCtx := ctx
	if l.cfg.AttemptSecs > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(l.cfg.AttemptSecs)*time.Second)
		defer cancel()
	}
	// A headless tool is quiet while it works, so the run would sit silent for
	// minutes. Tail the proxy's trace on a ticker and print a live line, the
	// requests it has made and the tokens they cost so far, so an operator can
	// see the loop moving and spot a stall or a runaway without waiting for the
	// final summary.
	stopProgress := l.logProgress(tool, sc.Name, trace, start)
	mounts := []container.Mount{
		{Host: work, Container: "/work"},
		{Host: trace, Container: "/trace"},
		{Host: sc.dir, Container: "/scenario", ReadOnly: true},
	}
	// When prep built a Python environment for this attempt, carry it into the
	// agent container on the same two volumes, so the venv the prep step made is
	// the one the agent's `python` and `pytest` resolve to. A task with no
	// prepared env (every non-SWE-bench scenario) mounts nothing extra and runs
	// exactly as before.
	if exists(env) {
		mounts = append(mounts, l.envMounts(env)...)
	}
	err = l.rt.Run(runCtx, container.RunSpec{
		Name: sl.run, Image: toolPrefix + tool, Network: l.cfg.Network, Remove: true,
		Mounts: mounts,
		Env: []string{
			"LAB_BASE_URL=" + sl.baseURL(),
			"LAB_MODEL=" + l.cfg.Model,
			"OPENCODE_API_KEY=" + l.cfg.APIKey,
			"LAB_MAX_TURNS=" + strconv.Itoa(l.cfg.MaxTurns),
		},
		Stdout: os.Stdout, Stderr: os.Stderr,
	})
	stopProgress()
	wall = int(time.Since(start).Seconds())
	if runCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		// The tool ran past its wall clock, not the whole sweep being cancelled.
		// Kill the container with the live parent context and grade the partial
		// work: a runaway is a failed attempt, not a reason to abort the sweep.
		l.rt.Remove(ctx, sl.run)
		fmt.Fprintf(os.Stderr, "[run] %s / %s timed out after %ds\n", tool, sc.Name, wall)
		return wall, exitTimeout, nil
	}
	if err != nil {
		return wall, -1, err
	}
	// The container has exited, but a bind-mounted runtime does not always make
	// its final writes visible to the host the instant the run returns: podman on
	// macOS routes /work and /trace through a VM, so the exit_code file, the GNU
	// time report, and the agent's last edits to the work tree can lag by a
	// moment. The adapter writes exit_code last, after everything else, so wait
	// for it to land before reading the exit code, the resource report, or
	// grading the tree. Without this a real pass reads as an empty grade, peak
	// memory reads as zero, and the exit code reads as -1, all from looking too
	// soon. On a runtime with synchronous mounts the file is already there and
	// the wait returns at once.
	awaitFlush(trace)
	return wall, l.readExitCode(trace), nil
}

// flushWait bounds how long awaitFlush waits for the container's final mounted
// write to appear on the host; flushPoll is how often it checks. The wait only
// runs on a clean exit, where the adapter is guaranteed to write the file, so it
// resolves in a poll or two and never sits out the full bound.
const (
	flushWait = 15 * time.Second
	flushPoll = 50 * time.Millisecond
)

// awaitFlush waits for the adapter's last artifact, the exit_code file, to be
// visible and non-empty on the host, so a mount that propagates writes lazily
// has settled before the harness reads the trace or grades the work tree.
func awaitFlush(trace string) {
	deadline := time.Now().Add(flushWait)
	for {
		if b, err := os.ReadFile(filepath.Join(trace, "exit_code")); err == nil && len(bytes.TrimSpace(b)) > 0 {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(flushPoll)
	}
}

// exitTimeout marks an attempt the wall clock killed, borrowing the shell's
// conventional 124 for a timed-out command so a reader of the result can tell a
// runaway from a clean non-zero exit.
const exitTimeout = 124

// progressInterval is how often a running attempt prints its live trace line.
const progressInterval = 15 * time.Second

// logProgress tails the proxy trace while an attempt runs and prints a compact
// line on a ticker: the completions the tool has made so far, the tokens they
// cost, and the seconds elapsed. It returns a stop function the caller invokes
// once the attempt ends, so the ticker never outlives the run. Nothing prints
// until the first request lands, so a fast task stays quiet, and the elapsed
// clock keeps ticking even when a single long completion is in flight, so a
// live run is never mistaken for a stalled one.
func (l *Lab) logProgress(tool, scenario, trace string, start time.Time) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(progressInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				reqs := countLines(filepath.Join(trace, "requests.jsonl"))
				if reqs == 0 {
					continue
				}
				tok, _ := sumTokens(filepath.Join(trace, "usage.jsonl"))
				fmt.Fprintf(os.Stderr, "[run] %s / %s  %d reqs, %dk tokens, %ds\n",
					tool, scenario, reqs, tok.Total/1000, int(time.Since(start).Seconds()))
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

func (l *Lab) startProxy(ctx context.Context, trace string, sl slot) error {
	return l.rt.Run(ctx, container.RunSpec{
		Name: sl.proxy, Image: proxyImage, Network: l.cfg.Network, Detach: true,
		Mounts: []container.Mount{{Host: trace, Container: "/trace"}},
		Env: []string{
			"UPSTREAM=" + l.cfg.Upstream,
			"TRACE_DIR=/trace",
		},
		Publish: fmt.Sprintf("127.0.0.1:%d:8080", sl.port),
	})
}

// startWeb stands up the one shared web sidecar that serves scenario fixtures,
// under the fixed name a prompt refers to. It is idempotent within a sweep: the
// caller starts it once before running and tears it down after, and every
// worker's tool reaches the same instance over the network.
func (l *Lab) startWeb(ctx context.Context) error {
	l.rt.Remove(ctx, l.cfg.webName())
	return l.rt.Run(ctx, container.RunSpec{
		Name: l.cfg.webName(), Image: baseImage, Network: l.cfg.Network, Detach: true,
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
// a pass; the combined output is the reason either way. A checker may also print
// an "EDITED_TESTS:" line naming test files the tool changed; grade lifts that
// out of the reason and returns it separately, so the marker never becomes the
// pass or fail reason itself.
func (l *Lab) grade(ctx context.Context, sc Scenario, work string) (bool, string, []string) {
	check := filepath.Join(sc.dir, "check.sh")
	if !exists(check) {
		return false, "no check.sh", nil
	}
	out, err := exec.CommandContext(ctx, "bash", check, work).CombinedOutput()
	reason, edited := splitEditedTests(string(out))
	return err == nil, strings.TrimSpace(reason), edited
}

// splitEditedTests pulls any "EDITED_TESTS: a b c" lines out of checker output
// and returns the rest of the output plus the names those lines carried.
func splitEditedTests(out string) (string, []string) {
	var kept, edited []string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "EDITED_TESTS:"); ok {
			edited = append(edited, strings.Fields(rest)...)
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), edited
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
