// Package probe is a cheap, fast, containerless turn driver for tomo's engines.
// It drives the real cx.Engine.Turn or agent.Agent.Turn loop in-process against a
// swebench-live task's offline tree, on a free or cheap model, so A/B testing the
// engine, its harness, or its prompt is a seconds-long inner loop instead of the
// minutes-and-dollars of a full container run on the paid bridge. It reuses tomo's
// own provider, tools, and engines, so what it observes is exactly what a real run
// would do, only without the container, the proxy, or the answer-leak surface
// (setup.sh strips the future git history, and no network fetch is wired).
//
// It makes real model calls: the point is to see how the model actually behaves
// under a change, not to project it. Point --system-file at a prompt file and edit
// it between runs, or rebuild with a harness variant, and every run drops a full
// trace (every request and response, every tool call and result, tokens and
// timing) plus a one-line summary, so a runaway or a wrong-neighbourhood edit is
// visible at a glance and every metric is on disk to compare across runs. The
// authoritative pass/fail stays with the full harness and its hidden pytest run.
package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/tamnd/tomo-labs/pkg/pricing"
	"github.com/tamnd/tomo/pkg/agent"
	"github.com/tamnd/tomo/pkg/builtin"
	"github.com/tamnd/tomo/pkg/config"
	"github.com/tamnd/tomo/pkg/engine/cx"
	"github.com/tamnd/tomo/pkg/provider"
	"github.com/tamnd/tomo/pkg/sandbox"
	"github.com/tamnd/tomo/pkg/tool"
)

// turnEngine is the single method both of tomo's engines share: run one user turn
// to completion and hand back every message it produced. The default agent and the
// cx engine implement it with the same signature, so the sim holds either one
// behind this interface and drives them head to head without any other change.
type turnEngine interface {
	Turn(ctx context.Context, history []provider.Message, user provider.Message, sink agent.Sink) ([]provider.Message, error)
}

// Options configures one simulated turn.
type Options struct {
	Root        string        // tomo-labs repo root holding evals/<suite>/
	Suite       string        // eval suite, default "swebench-live"
	Task        string        // task dir name, e.g. "dynaconf__dynaconf-1225"
	Model       string        // provider/model, default "opencode/deepseek-v4-flash-free"
	BaseURL     string        // openai base_url override (e.g. a local bridge); empty uses the provider default
	Engine      string        // "cx-offline" (default), "cx", or "agent"
	SystemFile  string        // prompt template file to render as the system prompt; empty uses the engine's embedded one
	HistoryFile string        // messages.json from a past run to resume the conversation from; empty starts fresh
	Message     string        // the user turn to send; empty uses the task's prompt.txt (the natural first turn)
	Timeout     time.Duration // wall clock before the turn is cut off, default 4m
	DataDir     string        // LAB_DATA for setup.sh's clone cache; empty uses ~/data/tomo-labs
	OutDir      string        // where the trace and summary are written; empty writes none
	Keep        bool          // keep the work tree instead of removing it
	Grade       bool          // after the turn, run the task's check.sh for the real hidden-test verdict
	MaxRounds   int           // hard cap on model calls in the turn; 0 leaves the governor in charge
	PrepEnv     bool          // build the task's venv before the turn so the agent starts with a working python and pytest, as the container does
}

// Result is what the simulated turn did.
type Result struct {
	Task              string         `json:"task"`
	Model             string         `json:"model"`
	Engine            string         `json:"engine"`
	SystemFile        string         `json:"system_file,omitempty"`
	WorkDir           string         `json:"work_dir,omitempty"`
	Rounds            int            `json:"rounds"`                 // model calls the loop made
	ToolCallsN        int            `json:"tool_calls"`             // total tool calls issued
	InputTokens       int            `json:"input_tokens"`           // summed prompt tokens across rounds, cached included
	CachedInputTokens int            `json:"cached_input_tokens"`    // subset of InputTokens the provider served from cache
	OutputTokens      int            `json:"output_tokens"`          // summed completion tokens across rounds
	ToolCalls         map[string]int `json:"tool_calls_by"`          // count per tool name
	Trajectory        []string       `json:"trajectory"`             // tool names in call order (capped)
	EditedFiles       []string       `json:"edited_files"`           // files the run changed in the tree
	EditedTests       []string       `json:"edited_tests"`           // test files it touched (a smell: the grader owns tests)
	GoldFiles         []string       `json:"gold_files"`             // non-test files the oracle's fix touches
	HitGold           []string       `json:"hit_gold"`               // EditedFiles that are in GoldFiles
	StopReason        string         `json:"stop_reason"`            // last model stop reason
	TimedOut          bool           `json:"timed_out"`              // the inner deadline cut the turn off
	ElapsedSecs       float64        `json:"elapsed_secs"`           //
	Graded            bool           `json:"graded"`                 // check.sh was run for the real verdict
	Passed            bool           `json:"passed"`                 // graded and the hidden tests are green
	CheckReason       string         `json:"check_reason,omitempty"` // the check.sh verdict line
	Priced            bool           `json:"priced"`                 // the model was found in the pricing table
	InputUSD          float64        `json:"input_usd"`              // fresh input tokens at list price
	CachedUSD         float64        `json:"cached_usd"`             // cached input reads at the discounted list price
	OutputUSD         float64        `json:"output_usd"`             // output tokens (reasoning included) at list price
	CostUSD           float64        `json:"cost_usd"`               // total list-price cost of the run
	Err               string         `json:"error,omitempty"`
}

// Converged reports the cheap directional verdict: the run changed at least one
// of the files the oracle's fix lives in, touched no test file, and did not time
// out. It is a signal, not the graded result.
func (r Result) Converged() bool {
	return len(r.HitGold) > 0 && len(r.EditedTests) == 0 && !r.TimedOut && r.Err == ""
}

// Run materializes the task's offline tree, drives one cx turn against it, and
// reports what happened. The caller's context bounds the whole run; Timeout adds
// an inner deadline so a spinning turn is cut off even without cancellation.
func Run(ctx context.Context, o Options) (Result, error) {
	o = withDefaults(o)
	if !validEngine(o.Engine) {
		return Result{}, fmt.Errorf("unknown engine %q: want agent, cx, or cx-offline", o.Engine)
	}
	taskDir := filepath.Join(o.Root, "evals", o.Suite, "tasks", o.Task)
	oracleDir := filepath.Join(o.Root, "evals", o.Suite, "oracle", o.Task)
	if _, err := os.Stat(taskDir); err != nil {
		return Result{}, fmt.Errorf("task %q not found under %s: %w", o.Task, filepath.Join(o.Root, "evals", o.Suite), err)
	}

	work, err := os.MkdirTemp("", "probe-")
	if err != nil {
		return Result{}, err
	}
	if !o.Keep {
		defer os.RemoveAll(work)
	}
	if err := materialize(ctx, taskDir, work, o.DataDir); err != nil {
		return Result{}, fmt.Errorf("materialize tree: %w", err)
	}

	// Prep the runtime environment before the turn when asked, the way the container
	// image does, so the agent starts with a working python and pytest and spends its
	// budget on the fix rather than on installing the project. Without this the probe
	// measures dependency-install noise, not the engine's fix behaviour: a bare tree
	// has no venv, so the model burns round after round on pip before it can run a
	// test. The venv lives outside the work tree so its build artifacts never look
	// like an edit, and PATH is restored after the turn.
	if o.PrepEnv {
		binDir, cleanup, err := prepEnv(ctx, work, oracleDir)
		if err != nil {
			return Result{}, fmt.Errorf("prep env: %w", err)
		}
		if !o.Keep {
			defer cleanup()
		}
		oldPath := os.Getenv("PATH")
		os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
		os.Setenv("VIRTUAL_ENV", filepath.Dir(binDir))
		defer func() { os.Setenv("PATH", oldPath); os.Unsetenv("VIRTUAL_ENV") }()
	}

	user := o.Message
	if user == "" {
		prompt, err := os.ReadFile(filepath.Join(taskDir, "prompt.txt"))
		if err != nil {
			return Result{}, err
		}
		user = string(prompt)
	}

	var history []provider.Message
	if o.HistoryFile != "" {
		raw, err := os.ReadFile(o.HistoryFile)
		if err != nil {
			return Result{}, fmt.Errorf("history file: %w", err)
		}
		if err := json.Unmarshal(raw, &history); err != nil {
			return Result{}, fmt.Errorf("history file %s: %w", o.HistoryFile, err)
		}
	}

	system, err := systemPrompt(o, work)
	if err != nil {
		return Result{}, err
	}

	prov, modelID, err := buildProvider(o)
	if err != nil {
		return Result{}, err
	}

	var trace, events *os.File
	if o.OutDir != "" {
		if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
			return Result{}, err
		}
		if trace, err = os.Create(filepath.Join(o.OutDir, "trace.jsonl")); err != nil {
			return Result{}, err
		}
		defer trace.Close()
		if events, err = os.Create(filepath.Join(o.OutDir, "events.jsonl")); err != nil {
			return Result{}, err
		}
		defer events.Close()
		_ = os.WriteFile(filepath.Join(o.OutDir, "system.txt"), []byte(system), 0o644)
		_ = os.WriteFile(filepath.Join(o.OutDir, "prompt.txt"), []byte(user), 0o644)
	}
	cp := &countingProvider{inner: prov, trace: trace}

	box, err := sandbox.New("none", work)
	if err != nil {
		return Result{}, fmt.Errorf("sandbox: %w", err)
	}
	reg := tool.NewRegistry(engineTools(o.Engine, builtin.All(box, work))...)

	sink := newMetricsSink(events)

	e := buildEngine(o.Engine, cp, modelID, system, reg, work, o.MaxRounds)

	runCtx := ctx
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}

	start := time.Now()
	turnMsgs, turnErr := e.Turn(runCtx, history, provider.UserText(user), sink)
	elapsed := time.Since(start)

	res := Result{
		Task:              o.Task,
		Model:             o.Model,
		Engine:            o.Engine,
		SystemFile:        o.SystemFile,
		Rounds:            cp.rounds,
		ToolCallsN:        len(sink.trajectory),
		InputTokens:       cp.inTokens,
		CachedInputTokens: cp.cachedTokens,
		OutputTokens:      cp.outTokens,
		ToolCalls:         sink.calls,
		Trajectory:        capStrings(sink.trajectory, 200),
		StopReason:        cp.lastStop,
		ElapsedSecs:       elapsed.Seconds(),
	}
	// List-price cost, the number the user reads to compare a cheap run against an
	// expensive one. Every model the lab runs is priced at its published rate, the
	// free deepseek proxy included, so a free run still shows what it would cost. The
	// prompt splits into a fresh part billed at the input rate and a cached part
	// billed at the cache-read rate, so a long turn that re-sends a mostly-cached
	// history reads as cheap without reading as free.
	if m, ok := pricing.Default().Lookup(o.Model); ok {
		fresh := cp.inTokens - cp.cachedTokens
		if fresh < 0 {
			fresh = 0
		}
		c := m.Cost(pricing.Usage{InputTokens: fresh, CachedInputTokens: cp.cachedTokens, OutputTokens: cp.outTokens})
		res.Priced = true
		res.InputUSD = c.InputUSD
		res.CachedUSD = c.CachedUSD
		res.OutputUSD = c.OutputUSD
		res.CostUSD = c.TotalUSD
	}
	if turnErr != nil {
		res.Err = turnErr.Error()
		res.TimedOut = runCtx.Err() == context.DeadlineExceeded
	}
	edited := editedFiles(ctx, work)
	res.GoldFiles = goldSourceFiles(oracleDir)
	for _, f := range edited {
		if isTestPath(f) {
			res.EditedTests = append(res.EditedTests, f)
		} else {
			res.EditedFiles = append(res.EditedFiles, f)
		}
	}
	res.HitGold = intersect(res.EditedFiles, res.GoldFiles)
	// The authoritative verdict, if asked for: check.sh builds its own venv, applies
	// the hidden test patch, and runs the bug's tests. It is the same grade the
	// container harness gives, only in-process, so a sim run can be scored for real
	// and not just by the gold-file heuristic. It runs last because it mutates the
	// tree (it restores test files and applies the patch), after editedFiles is read.
	if o.Grade {
		res.Graded = true
		res.Passed, res.CheckReason = gradeTree(ctx, taskDir, work, o.DataDir)
	}
	if o.Keep {
		res.WorkDir = work
	}
	if o.OutDir != "" {
		if b, err := json.MarshalIndent(res, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(o.OutDir, "summary.json"), b, 0o644)
		}
		// The full conversation (prior history plus this turn) so a later run can
		// resume or fork it with --history-file.
		full := append(append([]provider.Message{}, history...), turnMsgs...)
		if b, err := json.MarshalIndent(full, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(o.OutDir, "messages.json"), b, 0o644)
		}
		// The readable view of the whole turn: text, tool calls, results, timing.
		_ = writeTranscript(filepath.Join(o.OutDir, "transcript.md"), res, sink.events)
	}
	return res, nil
}

func withDefaults(o Options) Options {
	if o.Suite == "" {
		o.Suite = "swebench-live"
	}
	if o.Task == "" {
		o.Task = "dynaconf__dynaconf-1225"
	}
	if o.Model == "" {
		o.Model = "opencode/deepseek-v4-flash-free"
	}
	if o.Engine == "" {
		o.Engine = "cx-offline"
	}
	if o.Timeout == 0 {
		o.Timeout = 4 * time.Minute
	}
	if o.DataDir == "" {
		if h, err := os.UserHomeDir(); err == nil {
			o.DataDir = filepath.Join(h, "data", "tomo-labs")
		}
	}
	return o
}

// systemPrompt returns the system prompt for the run: the file at SystemFile,
// rendered with the same fields the engine template uses, or the engine's own
// embedded prompt when no file is given. The file path is the zero-rebuild lever:
// edit it and re-run.
func systemPrompt(o Options, work string) (string, error) {
	if o.SystemFile == "" {
		if o.Engine == "agent" {
			return agent.SystemPrompt(time.Now(), work, "", "", ""), nil
		}
		return cx.SystemPrompt(time.Now(), work, "", "", "", o.Engine == "cx-offline"), nil
	}
	raw, err := os.ReadFile(o.SystemFile)
	if err != nil {
		return "", fmt.Errorf("system file: %w", err)
	}
	tmpl, err := template.New("system").Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("system file %s: %w", o.SystemFile, err)
	}
	var b strings.Builder
	err = tmpl.Execute(&b, map[string]string{
		"Workspace":   work,
		"Persona":     "",
		"Today":       time.Now().Format("Monday, 2006-01-02"),
		"MemoryIndex": "",
		"SkillsIndex": "",
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

// validEngine reports whether o.Engine names an engine the sim can drive.
func validEngine(engine string) bool {
	switch engine {
	case "agent", "cx", "cx-offline":
		return true
	}
	return false
}

// engineTools returns the toolset for the engine. The cx engine retunes the
// builtin tool descriptions to its own terser prompt style; the default agent
// takes them as they are, so what the sim measures matches each engine's real run.
func engineTools(engine string, base []tool.Tool) []tool.Tool {
	if engine == "cx" || engine == "cx-offline" {
		return cx.Retune(base)
	}
	return base
}

// buildEngine constructs the engine the run drives. Both the default agent and the
// cx engine expose the same Turn method, so the sim keeps either behind turnEngine
// and the rest of the run is identical. The gate is nil, which allows every tool:
// the sim is the yolo equivalent, run against a throwaway tree.
func buildEngine(engine string, prov provider.Provider, model, system string, reg *tool.Registry, work string, maxRounds int) turnEngine {
	if engine == "agent" {
		return &agent.Agent{Provider: prov, Model: model, System: system, Tools: reg, Workspace: work, MaxRounds: maxRounds}
	}
	return &cx.Engine{Provider: prov, Model: model, System: system, Tools: reg, Workspace: work, MaxRounds: maxRounds}
}

// gradeTree runs the task's check.sh over the work tree and reports the real
// hidden-test verdict: exit zero is a pass, and the check's own summary line is
// the reason. It is the authoritative grade, the same one the container harness
// runs, so a sim run can be scored for real. It builds its own venv and applies
// the test patch, so it is the slow part of a graded run and mutates the tree.
func gradeTree(ctx context.Context, taskDir, work, dataDir string) (bool, string) {
	check := filepath.Join(taskDir, "check.sh")
	if _, err := os.Stat(check); err != nil {
		return false, "no check.sh"
	}
	cmd := exec.CommandContext(ctx, "bash", check, work)
	cmd.Env = append(os.Environ(), "LAB_DATA="+dataDir)
	out, err := cmd.CombinedOutput()
	return err == nil, lastNonEmptyLine(string(out))
}

// lastNonEmptyLine returns the final non-blank line of s, which for check.sh is
// its "PASS: ..." or "FAIL: ..." verdict.
func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

// prepEnv builds the task's runtime environment before the turn, the way the
// container image does, so the agent starts with a working python and pytest
// instead of burning its budget installing the project. It mirrors check.sh's
// install: a uv venv at the task's Python version, the project installed editable,
// and pytest ensured. It returns the venv's bin dir to prepend to PATH and a
// cleanup that removes the venv, which lives outside the work tree so its build
// artifacts never look like an edit.
func prepEnv(ctx context.Context, work, oracleDir string) (binDir string, cleanup func(), err error) {
	pyver := "3.12"
	if b, rerr := os.ReadFile(filepath.Join(oracleDir, "python")); rerr == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			pyver = v
		}
	}
	venv, err := os.MkdirTemp("", "probe-venv-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { os.RemoveAll(venv) }
	mk := func(name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = work
		return cmd.CombinedOutput()
	}
	if out, verr := mk("uv", "venv", "--python", pyver, venv); verr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("uv venv %s: %v\n%s", pyver, verr, out)
	}
	py := filepath.Join(venv, "bin", "python")
	// Install the project. The specs mirror check.sh: try the test extras first so a
	// project with a test extra pulls its test deps, then fall back to a plain
	// install. The first recipe that succeeds wins.
	installed := false
	for _, spec := range [][]string{{"-e", ".[test]"}, {"-e", ".[tests]"}, {"-e", ".[dev]"}, {"-e", "."}, {"."}} {
		args := append([]string{"pip", "install", "--python", py, "-q"}, spec...)
		if _, ierr := mk("uv", args...); ierr == nil {
			installed = true
			break
		}
	}
	if !installed {
		cleanup()
		return "", func() {}, fmt.Errorf("could not install project into venv")
	}
	// Ensure a runner is present without clobbering a self-provided one.
	if err := exec.CommandContext(ctx, py, "-c", "import pytest").Run(); err != nil {
		_, _ = mk("uv", "pip", "install", "--python", py, "-q", "pytest")
	}
	return filepath.Join(venv, "bin"), cleanup, nil
}

// materialize runs the task's setup.sh to check out the offline tree into work.
func materialize(ctx context.Context, taskDir, work, dataDir string) error {
	setup := filepath.Join(taskDir, "setup.sh")
	cmd := exec.CommandContext(ctx, "bash", setup, work)
	cmd.Env = append(os.Environ(), "LAB_DATA="+dataDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v\n%s", setup, err, out)
	}
	return nil
}

// buildProvider resolves the provider/model spec into a live provider and the
// bare model id. It knows opencode's zen endpoint and lets --base-url point an
// openai-shaped client at a local bridge for a gpt-5.x check.
func buildProvider(o Options) (provider.Provider, string, error) {
	name, modelID := "opencode", o.Model
	if i := strings.IndexByte(o.Model, '/'); i >= 0 {
		name, modelID = o.Model[:i], o.Model[i+1:]
	}
	pc := config.Provider{Type: "openai"}
	switch {
	case o.BaseURL != "":
		pc.BaseURL = o.BaseURL
		pc.APIKey = os.Getenv("OPENCODE_API_KEY")
	case name == "opencode":
		// zen serves the OpenAI wire at /zen/v1; tomo's client appends
		// /chat/completions, so the base_url stops at the /v1 root.
		pc.BaseURL = "https://opencode.ai/zen/v1"
		pc.APIKey = os.Getenv("OPENCODE_API_KEY")
	default:
		return nil, "", fmt.Errorf("unknown provider %q: pass --base-url for a custom endpoint", name)
	}
	p, err := provider.Build(pc)
	if err != nil {
		return nil, "", err
	}
	return p, modelID, nil
}

// editedFiles lists the tree-relative paths the run changed, staged or not,
// tracked or new, so a fresh file counts as an edit too.
func editedFiles(ctx context.Context, work string) []string {
	cmd := exec.CommandContext(ctx, "git", "-C", work, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// porcelain: XY<space>path ; a rename is "R  old -> new".
		path := strings.TrimSpace(line[2:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		if isBuildArtifact(path) {
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

// goldSourceFiles reads the oracle's gold.diff and returns the non-test files it
// touches: the places the real fix lives, which a converging run should edit.
func goldSourceFiles(oracleDir string) []string {
	data, err := os.ReadFile(filepath.Join(oracleDir, "gold.diff"))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "+++ b/") {
			continue
		}
		path := strings.TrimPrefix(line, "+++ b/")
		if path == "" || isTestPath(path) {
			continue
		}
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

// isBuildArtifact reports whether a changed path is a build or install byproduct
// rather than a real source edit. An editable install of the project (prepEnv, or
// the model's own pip run) drops egg-info, pyc, and cache files into the tree; they
// are never the fix and would otherwise inflate the edited-file list and its sprawl
// signal, so they are dropped before an edit is counted.
func isBuildArtifact(p string) bool {
	return strings.Contains(p, ".egg-info") ||
		strings.Contains(p, "__pycache__") ||
		strings.HasSuffix(p, ".pyc") ||
		strings.HasPrefix(p, ".venv") ||
		strings.HasPrefix(p, ".eggs") ||
		strings.HasPrefix(p, "build/") ||
		strings.HasPrefix(p, ".pytest_cache")
}

func isTestPath(p string) bool {
	base := filepath.Base(p)
	return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") ||
		strings.Contains(p, "/tests/") || strings.HasPrefix(p, "tests/") || base == "conftest.py"
}

func intersect(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	var out []string
	for _, x := range a {
		if set[x] {
			out = append(out, x)
		}
	}
	return out
}

func capStrings(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(s[:n:n], fmt.Sprintf("... +%d more", len(s)-n))
}
