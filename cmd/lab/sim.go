package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tamnd/tomo-labs/pkg/lab"
	"github.com/tamnd/tomo-labs/pkg/simturn"
)

// cmdSim drives one engine turn against one offline swebench-live task in-process,
// with no container and no proxy, so tuning the engine or its prompt is a
// seconds-long loop instead of the minutes of a full container run. It reuses
// tomo's own provider, tools, and engines, so what it measures is what a real run
// would do. The authoritative pass/fail comes from --grade, which runs the task's
// hidden-test check.sh; without it the run reports the cheap gold-file heuristic.
//
//	lab sim [task] [flags]
//	  --engine <agent|cx|cx-offline>  which engine to drive (default cx-offline)
//	  --model <provider/model>        default opencode/deepseek-v4-flash-free
//	  --system-file <path>            prompt template to render instead of the embedded one
//	  --message <text>                the user turn (default: the task's prompt.txt)
//	  --history-file <path>           messages.json to resume a prior turn from
//	  --base-url <url>                point the openai client at a local bridge
//	  --out <dir>                     write trace.jsonl, events.jsonl, transcript.md, summary.json
//	  --timeout <dur>                 inner deadline, default 4m
//	  --grade                         run check.sh for the real hidden-test verdict
//	  --keep                          keep the work tree instead of removing it
func cmdSim(ctx context.Context, cfg lab.Config, suite string, rest []string) error {
	o := simturn.Options{Root: cfg.Root, Suite: suite, DataDir: cfg.Data}

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
	o.Grade = hasFlag(rest, "--grade")
	o.Keep = hasFlag(rest, "--keep")
	o.Task = arg(rest, 0)

	res, err := simturn.Run(ctx, o)
	if err != nil {
		return err
	}
	writeSimSummary(os.Stderr, res)
	if o.OutDir != "" {
		fmt.Fprintf(os.Stderr, "artifacts in %s\n", o.OutDir)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
	return nil
}

// writeSimSummary prints the one-line verdict a human reads first: the engine, the
// grade if one was taken, the convergence heuristic otherwise, and the cost shape.
func writeSimSummary(w *os.File, r simturn.Result) {
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
	fmt.Fprintf(w, "%-9s %s | %s --engine %s | rounds %d calls %d tokens in %d out %d | %.1fs%s\n",
		verdict, r.Task, r.Model, r.Engine, r.Rounds, r.ToolCallsN, r.InputTokens, r.OutputTokens, r.ElapsedSecs, timeout)
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
