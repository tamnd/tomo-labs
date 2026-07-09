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
	sc, err := l.scenario(scenarioName)
	if err != nil {
		return nil, err
	}
	if !l.rt.ImageExists(ctx, toolPrefix+tool) {
		return nil, fmt.Errorf("tool image missing, run: lab build %s", tool)
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	runDir := filepath.Join(l.cfg.Data, tool, scenarioName, ts)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, err
	}
	attempts := max(l.cfg.Attempts, 1)

	var (
		work, trace                 string
		passed                      bool
		reason                      string
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

		wall, exitCode, err = l.runAttempt(ctx, tool, sc, work, trace, attempt, attempts)
		if err != nil {
			return nil, err
		}

		passed, reason = l.grade(ctx, sc, work)
		stripCaches(work)
		diskAfter = dirSizeKB(work)
		if passed {
			break
		}
	}

	m := readTrace(trace)
	installKB, imageKB := l.imageKB(tool)
	res := &Result{
		Tool: tool, Scenario: scenarioName, Time: ts,
		Model: l.cfg.Model, Runtime: l.rt.Bin,
		Passed: passed, ExitCode: exitCode,
		Attempts: used, AttemptsMax: attempts,
		WallSeconds: wall, ElapsedClock: m.ElapsedClock,
		MaxRSSKB: m.MaxRSSKB, Requests: m.Requests,
		Tokens: m.Tokens, Latency: m.Latency,
		DiskBeforeKB: diskBefore, DiskAfterKB: diskAfter, DiskDeltaKB: diskAfter - diskBefore,
		InstallKB: installKB, ImageKB: imageKB,
		Check: firstLineOf(reason),
	}
	if err := writeResult(filepath.Join(runDir, "result.json"), res); err != nil {
		return nil, err
	}
	pruneOldRuns(filepath.Join(l.cfg.Data, tool, scenarioName), l.cfg.KeepRuns)
	l.printSummary(res)
	return res, nil
}

// runAttempt stands up the proxy and optional web sidecar, runs the tool in a
// throwaway container, and tears the sidecars back down. It returns the wall
// seconds the tool ran and the exit code it left behind.
func (l *Lab) runAttempt(ctx context.Context, tool string, sc Scenario, work, trace string, attempt, attempts int) (wall, exitCode int, err error) {
	if err = l.rt.EnsureNetwork(ctx, l.cfg.Network); err != nil {
		return 0, -1, err
	}
	l.rt.Remove(ctx, proxyName)
	l.rt.Remove(ctx, webName)
	l.rt.Remove(ctx, runName)

	if err = l.startProxy(ctx, trace); err != nil {
		return 0, -1, err
	}
	defer l.rt.Remove(ctx, proxyName)
	if err = l.waitReady(ctx); err != nil {
		fmt.Fprintln(os.Stderr, l.rt.Logs(ctx, proxyName))
		return 0, -1, fmt.Errorf("proxy never became ready: %w", err)
	}

	if exists(filepath.Join(sc.dir, "web")) {
		if err = l.startWeb(ctx); err != nil {
			return 0, -1, err
		}
		defer l.rt.Remove(ctx, webName)
	}

	fmt.Fprintf(os.Stderr, "[run] %s / %s (attempt %d/%d)\n", tool, sc.Name, attempt, attempts)
	start := time.Now()
	err = l.rt.Run(ctx, container.RunSpec{
		Name: runName, Image: toolPrefix + tool, Network: l.cfg.Network, Remove: true,
		Mounts: []container.Mount{
			{Host: work, Container: "/work"},
			{Host: trace, Container: "/trace"},
			{Host: sc.dir, Container: "/scenario", ReadOnly: true},
		},
		Env: []string{
			"LAB_BASE_URL=http://" + proxyName + ":8080/v1",
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

func (l *Lab) startProxy(ctx context.Context, trace string) error {
	det := "1"
	if !l.cfg.Deterministic {
		det = "0"
	}
	return l.rt.Run(ctx, container.RunSpec{
		Name: proxyName, Image: proxyImage, Network: l.cfg.Network, Detach: true,
		Mounts: []container.Mount{{Host: trace, Container: "/trace"}},
		Env: []string{
			"UPSTREAM=" + l.cfg.Upstream,
			"TRACE_DIR=/trace",
			"LAB_DETERMINISTIC=" + det,
			"LAB_TEMPERATURE=" + l.cfg.Temperature,
			"LAB_TOP_P=" + l.cfg.TopP,
			"LAB_SEED=" + l.cfg.Seed,
		},
		Publish: fmt.Sprintf("127.0.0.1:%d:8080", l.cfg.ProxyPort),
	})
}

func (l *Lab) startWeb(ctx context.Context) error {
	return l.rt.Run(ctx, container.RunSpec{
		Name: webName, Image: baseImage, Network: l.cfg.Network, Detach: true,
		Mounts:  []container.Mount{{Host: filepath.Join(l.cfg.Root, "webroot"), Container: "/srv", ReadOnly: true}},
		Workdir: "/srv",
		Cmd:     []string{"python3", "-m", "http.server", "80"},
	})
}

// waitReady polls the published proxy port until it answers with any HTTP
// status, which means the listener is up even if the upstream 404s a bare GET.
func (l *Lab) waitReady(ctx context.Context) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/", l.cfg.ProxyPort)
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
	if r.Passed {
		mark = "PASS"
	}
	fmt.Fprintf(os.Stderr, "  %s  %-8s %-20s try=%d/%d tokens=%d reqs=%d rss=%.1fMB disk=+%dKB ttfb=%dms  %s\n",
		mark, r.Tool, r.Scenario, r.Attempts, r.AttemptsMax,
		r.Tokens.Total, r.Requests, float64(r.MaxRSSKB)/1024, r.DiskDeltaKB, r.Latency.AvgTTFB,
		firstLineOf(r.Check))
}

func firstLineOf(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}
