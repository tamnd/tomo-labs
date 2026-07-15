package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	analyze "github.com/tamnd/tomo-labs/pkg/analyzer/probe"
	"github.com/tamnd/tomo-labs/pkg/lab"
	"github.com/tamnd/tomo-labs/pkg/probe"
)

// cmdProbe drives one real engine turn against one offline swebench-live task
// in-process, with no container and no proxy, so A/B testing the engine, its
// harness, or its prompt is a seconds-long loop instead of the minutes of a full
// container run. It makes real model calls through tomo's own provider, tools, and
// engines, so what it observes is real behaviour, not a projection. The
// authoritative pass/fail comes from --grade, which runs the task's hidden-test
// check.sh; without it the run reports the cheap gold-file heuristic.
//
//	lab probe [task] [flags]      drive one real engine turn against one offline task
//	lab probe analyze <out-dir>   read a prior run's trace and print the token curve
//	lab probe project <out-dir>   re-cost a recorded run under caching and elision, no tokens spent
//
//	  --engine <agent|cx|cx-offline>  which engine to drive (default cx-offline)
//	  --model <provider/model>        default opencode/deepseek-v4-flash-free
//	  --system-file <path>            prompt template to render instead of the embedded one
//	  --message <text>                the user turn (default: the task's prompt.txt)
//	  --history-file <path>           messages.json to resume a prior turn from
//	  --base-url <url>                point the openai client at a local bridge
//	  --out <dir>                     write trace.jsonl, events.jsonl, transcript.md, summary.json
//	  --timeout <dur>                 inner deadline, default 4m
//	  --max-rounds <n>                hard cap on model calls, to bound an A/B probe (0 = governor decides)
//	  --prep-env                      build the task's venv first so the agent starts with working python and pytest, as the container does
//	  --grade                         run check.sh for the real hidden-test verdict
//	  --keep                          keep the work tree instead of removing it
func cmdProbe(ctx context.Context, cfg lab.Config, suite string, rest []string) error {
	// analyze reads back the raw trace a prior run dropped and prints the token
	// curve: no model call, just the numbers that explain where the tokens went.
	if len(rest) > 0 && rest[0] == "analyze" {
		return cmdProbeAnalyze(rest[1:])
	}
	// project re-costs a recorded run under transcript-shaping strategies without
	// touching a model, so a caching or elision change can be judged in the time it
	// takes to read a file. It is a hint, not a verdict: only a real probe run shows
	// whether the shaped transcript still keeps the model on track.
	if len(rest) > 0 && rest[0] == "project" {
		return cmdProbeProject(rest[1:])
	}

	o := probe.Options{Root: cfg.Root, Suite: suite, DataDir: cfg.Data}

	o.Engine, rest = takeFlagValue(rest, "--engine")
	o.Model, rest = takeFlagValue(rest, "--model")
	o.SystemFile, rest = takeFlagValue(rest, "--system-file")
	o.Message, rest = takeFlagValue(rest, "--message")
	o.HistoryFile, rest = takeFlagValue(rest, "--history-file")
	o.BaseURL, rest = takeFlagValue(rest, "--base-url")
	o.OutDir, rest = takeFlagValue(rest, "--out")
	if to, r := takeFlagValue(rest, "--timeout"); to != "" {
		d, err := time.ParseDuration(to)
		if err != nil {
			return fmt.Errorf("--timeout: %w", err)
		}
		o.Timeout, rest = d, r
	}
	if v, r := takeFlagValue(rest, "--max-rounds"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("--max-rounds: %w", err)
		}
		o.MaxRounds, rest = n, r
	}
	o.Grade = hasFlag(rest, "--grade")
	o.Keep = hasFlag(rest, "--keep")
	o.PrepEnv = hasFlag(rest, "--prep-env")
	o.Task = arg(rest, 0)

	res, err := probe.Run(ctx, o)
	if err != nil {
		return err
	}
	writeProbeSummary(os.Stderr, res)
	if o.OutDir != "" {
		fmt.Fprintf(os.Stderr, "artifacts in %s\n", o.OutDir)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
	return nil
}

// writeProbeSummary prints the one-line verdict a human reads first: the engine,
// the grade if one was taken, the convergence heuristic otherwise, and the cost
// shape.
func writeProbeSummary(w *os.File, r probe.Result) {
	verdict := "converged"
	if !r.Converged() {
		verdict = "off-track"
	}
	if r.Graded {
		if r.Passed {
			verdict = "PASS"
		} else {
			verdict = "FAIL"
		}
	}
	timeout := ""
	if r.TimedOut {
		timeout = " (timed out)"
	}
	fmt.Fprintf(w, "%-9s %s | %s --engine %s | rounds %d calls %d tokens in %d out %d | %s | %.1fs%s\n",
		verdict, r.Task, r.Model, r.Engine, r.Rounds, r.ToolCallsN, r.InputTokens, r.OutputTokens, costLabel(r), r.ElapsedSecs, timeout)
	if len(r.HitGold) > 0 {
		fmt.Fprintf(w, "  hit gold: %v\n", r.HitGold)
	}
	if len(r.EditedTests) > 0 {
		fmt.Fprintf(w, "  edited tests (a smell, the grader owns tests): %v\n", r.EditedTests)
	}
	if r.CheckReason != "" {
		fmt.Fprintf(w, "  check: %s\n", r.CheckReason)
	}
	if r.Err != "" {
		fmt.Fprintf(w, "  error: %s\n", r.Err)
	}
}

// costLabel renders the run's list-price cost for the one-line summary. Every model
// the lab runs is priced at its published rate, the free deepseek proxy included, so
// a free run still shows what it would cost and stays comparable to a paid one. The
// cached share is broken out when the provider reported a prefix-cache read, so a
// long turn's cheap re-sent history is visible next to its fresh cost. A model the
// table does not know reports "unpriced", not a misleading $0.00.
func costLabel(r probe.Result) string {
	if !r.Priced {
		return "unpriced"
	}
	s := fmt.Sprintf("$%.4f (in $%.4f out $%.4f)", r.CostUSD, r.InputUSD, r.OutputUSD)
	if r.CachedInputTokens > 0 {
		s += fmt.Sprintf(" cached %d tok $%.4f", r.CachedInputTokens, r.CachedUSD)
	}
	return s
}

// cmdProbeAnalyze reads a run's trace.jsonl and prints the concrete token report:
// the re-send ratio, the biggest input jumps, and the round-by-round curve. The
// argument is the run's --out dir (or the trace.jsonl path itself).
func cmdProbeAnalyze(rest []string) error {
	target := arg(rest, 0)
	if target == "" {
		return fmt.Errorf("usage: lab probe analyze <run-out-dir>")
	}
	tracePath, dir := target, target
	if fi, err := os.Stat(target); err == nil && fi.IsDir() {
		tracePath = filepath.Join(target, "trace.jsonl")
	} else {
		dir = filepath.Dir(target)
	}
	rep, err := analyze.Analyze(tracePath)
	if err != nil {
		return err
	}
	summary := analyze.LoadSummary(dir)
	analyze.WriteReport(os.Stdout, rep, summary)
	analyze.WriteToolMix(os.Stdout, summary)
	return nil
}

// cmdProbeProject re-costs a recorded run under transcript-shaping strategies:
// prefix caching (what strong tools survive the quadratic re-send with) and
// eliding stale tool results. It spends no tokens and calls no model, so it is a
// cheap hint at where the tokens could go: try a loop change here, see the
// projected curve, then confirm with a real probe run that the shaped transcript
// still keeps the model on track. --cache-rate sets the provider's cache-read
// discount (0.1 = 10% of full price), --keep-last how many recent tool results
// survive elision.
func cmdProbeProject(rest []string) error {
	cacheRate := 0.1
	if v, r := takeFlagValue(rest, "--cache-rate"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("--cache-rate: %w", err)
		}
		cacheRate, rest = f, r
	}
	keepLast := 8
	if v, r := takeFlagValue(rest, "--keep-last"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("--keep-last: %w", err)
		}
		keepLast, rest = n, r
	}
	target := arg(rest, 0)
	if target == "" {
		return fmt.Errorf("usage: lab probe project <run-out-dir> [--cache-rate 0.1] [--keep-last 8]")
	}
	tracePath := target
	if fi, err := os.Stat(target); err == nil && fi.IsDir() {
		tracePath = filepath.Join(target, "trace.jsonl")
	}
	actual, strategies, err := analyze.ProjectFile(tracePath, cacheRate, keepLast)
	if err != nil {
		return err
	}
	analyze.WriteProjection(os.Stdout, actual, strategies)
	return nil
}
