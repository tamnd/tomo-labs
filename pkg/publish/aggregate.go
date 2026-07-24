package publish

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/result"
	"github.com/tamnd/tomo-labs/pkg/trace"
)

// This file is the read side of the publisher: it loads every result.json under
// a runs directory into a common model and folds it into the aggregate the
// README and the reports are generated from. The publish package deliberately
// does not import pkg/lab (which imports publish for the run hook, a cycle), so
// it redeclares the stable JSON contract of a result here, decoding only the
// fields the dataset front matter needs. The contract is the result.json shape;
// pkg/lab/result.go is its source of truth.

// The publisher reads back the same run-outcome model the run loop wrote, so it
// aliases the single definition in pkg/result rather than keeping a hand-synced
// copy that could drift from it. It reads the full model, not a subset: extra
// fields it does not report simply decode and go unused, which is safer than a
// narrower struct that silently stops matching when the writer's shape changes.
type (
	Result        = result.Result
	Latency       = result.Latency
	Orchestration = result.Orchestration
)

// Run is one loaded result with the on-disk locations the publisher needs: the
// run directory that holds it, the trace directory to reconstruct from, and the
// run id that names the trace file. Eval is the suite the run belongs to,
// recovered from the runs-directory layout.
type Run struct {
	Result   Result
	Eval     string
	RunID    string
	Dir      string // the timestamped run directory
	TraceDir string // the attempt trace directory, or "" when none survives
}

// LoadRuns walks a runs root and loads every result.json into a Run, sorted by
// timestamp so a commit log and a board read in run order. The eval label is
// derived from the layout: the core suite lives at
// <root>/<tool>/<scenario>/<ts>/, and every other suite lives at
// <root>/evals/<suite>/<tool>/<scenario>/<ts>/, so a run under evals/<suite> is
// stamped with that suite and everything else is "core". This lets one walk of
// the data root produce the full cross-suite aggregate the README needs.
func LoadRuns(root string) ([]Run, error) {
	var runs []Run
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable subtrees rather than abort the whole load
		}
		if d.IsDir() || d.Name() != "result.json" {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		var res Result
		if json.Unmarshal(data, &res) != nil {
			return nil
		}
		runDir := filepath.Dir(path)
		runs = append(runs, Run{
			Result:   res,
			Eval:     evalFromPath(root, runDir),
			RunID:    filepath.Base(runDir),
			Dir:      runDir,
			TraceDir: latestTraceDir(runDir),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].Result.Time < runs[j].Result.Time
	})
	return runs, nil
}

// evalFromPath derives the eval/suite name for a run directory under root.
// A path under evals/<suite>/ takes that suite; anything else is "core".
func evalFromPath(root, runDir string) string {
	rel, err := filepath.Rel(root, runDir)
	if err != nil {
		return "core"
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 2 && parts[0] == "evals" {
		return parts[1]
	}
	return "core"
}

// latestTraceDir returns the trace directory of the highest-numbered attempt in
// a run directory, which is the attempt the result was graded on. It returns ""
// when no attempt trace is present.
func latestTraceDir(runDir string) string {
	attempts, _ := filepath.Glob(filepath.Join(runDir, "attempt-*"))
	if len(attempts) == 0 {
		return ""
	}
	sort.Slice(attempts, func(i, j int) bool {
		return attemptN(attempts[i]) < attemptN(attempts[j])
	})
	for i := len(attempts) - 1; i >= 0; i-- {
		trace := filepath.Join(attempts[i], "trace")
		if fi, err := os.Stat(trace); err == nil && fi.IsDir() {
			return trace
		}
	}
	return ""
}

func attemptN(path string) int {
	base := filepath.Base(path)
	n := 0
	for _, r := range strings.TrimPrefix(base, "attempt-") {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// meta builds the trace session header for a run from its result.
func (r Run) meta() trace.Header {
	res := r.Result
	tok := res.Tokens
	name := res.Tool + " on " + res.Scenario
	if res.Model != "" {
		name += " (" + res.Model + ")"
	}
	return trace.Header{
		Harness:     res.Tool,
		ID:          r.RunID,
		Name:        name,
		Eval:        r.Eval,
		Scenario:    res.Scenario,
		Model:       res.Model,
		Passed:      res.Passed,
		Ungraded:    res.Ungraded,
		ExitCode:    res.ExitCode,
		Attempts:    res.Attempts,
		WallSeconds: res.WallSeconds,
		Tokens:      &tok,
		CostUSD:     res.CostUSD,
		Stop:        res.Stop,
		Timestamp:   res.Time,
	}
}

// Aggregate is the fold over every run the README and board are generated from.
// It is a pure function of the run set, so a backfill and a live run over the
// same results produce identical numbers.
type Aggregate struct {
	Runs      []Run
	Traces    int
	Evals     []string
	Scenarios []string
	Tools     []string
	Models    []string

	// Cells is the board: one entry per (eval, tool), folding every run of that
	// tool on that eval into a solve rate and cost.
	Cells []BoardCell
}

// BoardCell is one tool's record on one eval: how many of its graded runs passed,
// and the token and dollar cost summed over them.
type BoardCell struct {
	Eval      string
	Tool      string
	Model     string
	Passed    int
	Graded    int
	Tokens    int
	CostUSD   float64
	CostKnown bool // true when at least one run reported a dollar cost
	WallSec   int
}

// SolveRate is passed over graded, or zero when nothing was graded.
func (c BoardCell) SolveRate() float64 {
	if c.Graded == 0 {
		return 0
	}
	return float64(c.Passed) / float64(c.Graded)
}

// Aggregate folds the runs into the board and the coverage sets.
func Fold(runs []Run) Aggregate {
	ag := Aggregate{Runs: runs, Traces: len(runs)}
	evals := map[string]bool{}
	scen := map[string]bool{}
	tools := map[string]bool{}
	models := map[string]bool{}
	cells := map[string]*BoardCell{}

	for _, r := range runs {
		res := r.Result
		evals[r.Eval] = true
		scen[res.Scenario] = true
		tools[res.Tool] = true
		if res.Model != "" {
			models[res.Model] = true
		}
		key := r.Eval + "\x00" + res.Tool
		c := cells[key]
		if c == nil {
			c = &BoardCell{Eval: r.Eval, Tool: res.Tool, Model: res.Model}
			cells[key] = c
		}
		if !res.Ungraded {
			c.Graded++
			if res.Passed {
				c.Passed++
			}
		}
		c.Tokens += res.Tokens.Total
		c.WallSec += res.WallSeconds
		if res.CostUSD > 0 {
			c.CostUSD += res.CostUSD
			c.CostKnown = true
		}
	}

	ag.Evals = sortedKeys(evals)
	ag.Scenarios = sortedKeys(scen)
	ag.Tools = sortedKeys(tools)
	ag.Models = sortedKeys(models)
	for _, c := range cells {
		ag.Cells = append(ag.Cells, *c)
	}
	sort.Slice(ag.Cells, func(i, j int) bool {
		if ag.Cells[i].Eval != ag.Cells[j].Eval {
			return ag.Cells[i].Eval < ag.Cells[j].Eval
		}
		if ag.Cells[i].SolveRate() != ag.Cells[j].SolveRate() {
			return ag.Cells[i].SolveRate() > ag.Cells[j].SolveRate()
		}
		return ag.Cells[i].Tool < ag.Cells[j].Tool
	})
	return ag
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
