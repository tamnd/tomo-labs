package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
)

// writeResult writes a Result as pretty JSON. The file is the unit the report
// aggregates, so it is written whole and atomically enough for a local run.
func writeResult(path string, r *Result) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// RunAll runs a set of tools over a set of scenarios. Empty tools means every
// wired tool with a built image; empty scenarios means all of them. It keeps
// going on a per-run error so one broken pair does not abort the sweep, and
// returns the results it did capture, in a stable tool-then-scenario order
// regardless of which worker finished each one.
//
// Up to cfg.Concurrency runs proceed at once, each on its own worker slot (its
// own proxy container and published port), so the sweep is bounded by the
// slowest few runs rather than the sum of all of them. Every run still forces
// the same deterministic decoding and captures its own trace, so parallelism
// changes only wall-clock scheduling, not what a run measures. The one shared
// resource is the upstream model: at higher concurrency a free-tier rate limit
// can add queueing that shows up in TTFB, so a strict latency comparison is
// best taken at LAB_CONCURRENCY=1, while pass rate and tokens are unaffected.
func (l *Lab) RunAll(ctx context.Context, tools, scenarios []string) ([]*Result, error) {
	if len(tools) == 0 {
		all, err := l.Tools()
		if err != nil {
			return nil, err
		}
		tools = all
	}
	if len(scenarios) == 0 {
		all, err := l.Scenarios()
		if err != nil {
			return nil, err
		}
		for _, s := range all {
			scenarios = append(scenarios, s.Name)
		}
	}

	var jobs []job
	for _, t := range tools {
		if !l.rt.ImageExists(ctx, toolPrefix+t) {
			fmt.Fprintf(os.Stderr, "skip %s: image missing, run: lab build %s\n", t, t)
			continue
		}
		for _, name := range scenarios {
			sc, err := l.scenario(name)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, job{tool: t, sc: sc})
		}
	}
	return l.dispatch(ctx, jobs)
}

// RunPrompt runs one ad-hoc prompt through a set of tools and returns their
// results, so `lab -p` can compare what every tool does with the same instruction
// on the same model. Empty tools means every wired tool with a built image. The
// prompt is an ungraded scenario: there is no checker, so each run happens once
// and captures the tool's answer alongside the usual metrics rather than a pass or
// fail. The runs go through the same parallel worker pool as a graded sweep, so a
// prompt fans out across the tools at once.
func (l *Lab) RunPrompt(ctx context.Context, prompt string, tools []string) ([]*Result, error) {
	if len(tools) == 0 {
		all, err := l.Tools()
		if err != nil {
			return nil, err
		}
		tools = all
	}

	sc, cleanup, err := l.promptScenario(prompt)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var jobs []job
	for _, t := range tools {
		if !l.rt.ImageExists(ctx, toolPrefix+t) {
			fmt.Fprintf(os.Stderr, "skip %s: image missing, run: lab build %s\n", t, t)
			continue
		}
		jobs = append(jobs, job{tool: t, sc: sc})
	}
	return l.dispatch(ctx, jobs)
}

// job pairs one tool with one already-resolved scenario. Carrying the Scenario
// rather than its name lets an ad-hoc prompt run flow through the same pool as a
// named scenario without the pool having to know where the definition came from.
type job struct {
	tool string
	sc   Scenario
}

// dispatch runs a set of jobs through the worker pool and returns their results in
// job order, regardless of which worker finished each one. It ensures the shared
// container network and, when any job needs it, the shared web sidecar exactly
// once, then hands each worker its own slot for the life of the run. A per-job
// error is logged and skipped so one broken pair does not abort the rest.
func (l *Lab) dispatch(ctx context.Context, jobs []job) ([]*Result, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	if err := l.ensureNetworks(ctx); err != nil {
		return nil, err
	}
	// The web sidecar is shared across workers, so stand it up once here rather
	// than inside each run, and only when a job's scenario actually needs it.
	if jobsNeedWeb(jobs) {
		if err := l.startWeb(ctx); err != nil {
			return nil, err
		}
		defer l.rt.Remove(ctx, l.cfg.webName())
	}

	workers := min(max(l.cfg.Concurrency, 1), len(jobs))

	// results is indexed by job so the output stays ordered no matter the finish
	// order; each worker owns one slot for the life of the sweep.
	results := make([]*Result, len(jobs))
	jobCh := make(chan int)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(slotIdx int) {
			defer wg.Done()
			sl := l.newSlot(slotIdx, l.cfg.ProxyPort)
			for i := range jobCh {
				res, err := l.runScenario(ctx, jobs[i].tool, jobs[i].sc, sl)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error %s/%s: %v\n", jobs[i].tool, jobs[i].sc.Name, err)
					continue
				}
				results[i] = res
			}
		}(w)
	}
feed:
	for i := range jobs {
		select {
		case <-ctx.Done():
			break feed
		case jobCh <- i:
		}
	}
	close(jobCh)
	wg.Wait()

	out := make([]*Result, 0, len(jobs))
	for _, r := range results {
		if r != nil {
			out = append(out, r)
		}
	}
	return out, nil
}

// jobsNeedWeb reports whether any job's scenario ships a web fixture dir, so the
// pool only stands up the shared web sidecar when a run actually needs it.
func jobsNeedWeb(jobs []job) bool {
	for _, j := range jobs {
		if exists(filepath.Join(j.sc.dir, "web")) {
			return true
		}
	}
	return false
}

// ToolSummary is one tool's aggregate over the latest run of each scenario, the
// row the comparison table is built from. Runs is therefore the number of distinct
// scenarios the tool has a run for, and Passed is how many of those it passed.
type ToolSummary struct {
	Tool          string  `json:"tool"`
	Version       string  `json:"version,omitempty"`
	Released      string  `json:"released,omitempty"`
	Runs          int     `json:"runs"`
	Passed        int     `json:"passed"`
	FirstTry      int     `json:"first_try"`
	Retried       int     `json:"retried"`
	AvgModelCalls int     `json:"avg_model_calls"`
	PlannedRuns   int     `json:"planned_runs"`
	Subagents     int     `json:"subagents"`
	InstallMB     int     `json:"install_mb"`
	TotalTokens   int     `json:"total_tokens"`
	CachedTokens  int     `json:"cached_tokens,omitempty"`
	TotalCostUSD  float64 `json:"total_cost_usd,omitempty"`
	AvgRSSMB      int     `json:"avg_rss_mb"`
	AvgTTFBMS     int     `json:"avg_ttfb_ms"`
	// The three time totals over the tool's runs, in seconds: TotalS is the whole
	// wall time, ModelS is the part spent waiting on the model, and ToolS is the
	// rest (tool execution and agent glue), TotalS minus ModelS.
	TotalS int `json:"total_s"`
	ModelS int `json:"model_s"`
	ToolS  int `json:"tool_s"`
	// StreamFailRuns is how many of the tool's runs hit an upstream stream drop,
	// whether it was retried away or left in the recorded attempt. It is the honest
	// count of gateway faults the tool ran into, kept apart from real failures.
	StreamFailRuns int `json:"stream_fail_runs,omitempty"`
}

// Report reads every result.json under the data dir and aggregates it per tool,
// keeping only the latest run of each tool and scenario so the summary is the
// tool's current state, not its whole history. A tool gets re-run as adapters
// and scenarios change; an old failing run from before a fix should not drag its
// pass rate down forever, and counting the same scenario several times would make
// total tokens depend on how often it happened to be re-run rather than on the
// work. The result is that every tool's row is over the same scenarios, so pass
// reads as N of the scenarios and total tokens compares like for like. A
// non-empty scenario filter narrows the summary to runs whose scenario name
// contains it, which is how the report focuses on one scenario at a time
// (report 11).
func (l *Lab) Report(_ context.Context, scenario string) ([]ToolSummary, error) {
	var results []*Result
	err := l.walkResults(func(path string, r *Result) {
		// Ungraded prompt runs (lab -p) have no pass or fail, so they never belong
		// in the scenario comparison; skip them here.
		if !r.Ungraded && (scenario == "" || strings.Contains(r.Scenario, scenario)) {
			results = append(results, r)
		}
	})
	if err != nil {
		return nil, err
	}
	sums := summarize(latestPerScenario(results))
	// Version and release date are properties of the tool image, captured at build
	// time, so join them in here rather than reading them off every run.
	for i := range sums {
		m := l.toolMetaOf(sums[i].Tool)
		sums[i].Version = m.Version
		sums[i].Released = m.Released
	}
	return sums, nil
}

// walkResults reads every result.json under the active suite's results dir and
// calls fn with the parsed run. In the core suite it skips the evals/ subtree so
// a suite's runs never leak into the core report, since the eval tiers live under
// the same data root; a named suite walks only its own subtree, so no skip is
// needed. Unreadable or malformed files and rows with no tool are dropped.
func (l *Lab) walkResults(fn func(path string, r *Result)) error {
	root := l.resultsDir()
	evalsDir := filepath.Join(l.cfg.Data, "evals")
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if l.cfg.Suite == "" && path == evalsDir {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "result.json" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var r Result
		if json.Unmarshal(b, &r) == nil && r.Tool != "" {
			fn(path, &r)
		}
		return nil
	})
}

// latestPerScenario keeps only the most recent run of each tool and scenario.
// Run timestamps are the compact UTC form (20260710T140140Z), which sorts the
// same lexically as chronologically, so the largest string is the newest run.
func latestPerScenario(results []*Result) []*Result {
	best := map[string]*Result{}
	for _, r := range results {
		k := r.Tool + "\x00" + r.Scenario
		if b, ok := best[k]; !ok || r.Time > b.Time {
			best[k] = r
		}
	}
	out := make([]*Result, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	return out
}

// DeepSeek's published standard-hours rates for the shared model, in USD per 1M
// tokens: cache-miss input, cache-hit input, and output. The lab runs on the free
// tier, which bills nothing, so the provider reports no cost. Pricing the tokens
// at these reference rates turns the token gap between tools into the dollar gap
// it would be on the paid tier, which is the number that decides which tool you
// can afford to run at scale.
const (
	rateInputMissUSD = 0.27
	rateInputHitUSD  = 0.07
	rateOutputUSD    = 1.10
)

// estimatedCostUSD prices one run's tokens at the reference rates above, used when
// the provider billed nothing so the report still has a comparable cost column.
func estimatedCostUSD(t Tokens) float64 {
	miss := max(t.Prompt-t.Cached, 0)
	return float64(miss)/1e6*rateInputMissUSD +
		float64(t.Cached)/1e6*rateInputHitUSD +
		float64(t.Completion)/1e6*rateOutputUSD
}

func summarize(results []*Result) []ToolSummary {
	byTool := map[string][]*Result{}
	for _, r := range results {
		byTool[r.Tool] = append(byTool[r.Tool], r)
	}
	var out []ToolSummary
	for tool, rs := range byTool {
		s := ToolSummary{Tool: tool, Runs: len(rs)}
		var tokens, cached, rss, ttfb, wall, model, timed, modelCalls int
		var cost float64
		for _, r := range rs {
			if r.Passed {
				s.Passed++
			}
			a := max(r.Attempts, 1)
			if a == 1 && r.Passed {
				s.FirstTry++
			}
			if a > 1 {
				s.Retried++
			}
			modelCalls += r.Orchestration.ModelCalls
			if r.Orchestration.Planned {
				s.PlannedRuns++
			}
			s.Subagents += r.Orchestration.Subagents
			tokens += r.Tokens.Total
			cached += r.Tokens.Cached
			// A paid provider reports the real cost; the free tier reports none, so
			// price its tokens at the reference rates to keep the column comparable.
			if r.CostUSD > 0 {
				cost += r.CostUSD
			} else {
				cost += estimatedCostUSD(r.Tokens)
			}
			rss += r.MaxRSSKB
			wall += r.WallSeconds
			model += r.Latency.SumTotal / 1000
			if r.StreamFail != nil {
				s.StreamFailRuns++
			}
			if r.Latency.Calls > 0 {
				ttfb += r.Latency.AvgTTFB
				timed++
			}
			// Install footprint is a property of the tool, not the run, so the
			// last one seen wins; they are all the same.
			s.InstallMB = r.InstallKB / 1024
		}
		n := len(rs)
		s.TotalTokens = tokens
		s.CachedTokens = cached
		s.TotalCostUSD = cost
		s.AvgRSSMB = rss / n / 1024
		s.TotalS = wall
		s.ModelS = model
		s.ToolS = max(wall-model, 0)
		s.AvgModelCalls = modelCalls / n
		if timed > 0 {
			s.AvgTTFBMS = ttfb / timed
		}
		out = append(out, s)
	}
	// Rank on pass@1 first, then cost. pass@1 (a task solved on the first attempt,
	// no retry) is the capability metric every code benchmark headlines, so the tool
	// that solves the most goes to the top. Cost breaks ties: among tools that solve
	// the same number first-try, the cheapest wins, since cost is what the tokens
	// actually buy. When a suite is saturated and every tool ties on pass@1 (as on
	// LiveCodeBench), this degrades to a pure cheapest-first order. Cost weights a
	// cached-input token far below a generated one, so a cache-heavy multi-turn tool
	// reads as the bargain it is, not as a spendthrift on raw token count. The tool
	// name is the final tie-break for a stable order.
	sort.Slice(out, func(i, j int) bool {
		if out[i].FirstTry != out[j].FirstTry {
			return out[i].FirstTry > out[j].FirstTry
		}
		if out[i].TotalCostUSD != out[j].TotalCostUSD {
			return out[i].TotalCostUSD < out[j].TotalCostUSD
		}
		return out[i].Tool < out[j].Tool
	})
	return out
}

// WriteTable renders the summaries as aligned text, split into two tables: the
// tools that planned (wrote a plan or todo list, or spawned a subagent) and the
// tools that ran a flat loop. The split keeps the comparison honest and clean:
// planning trades tokens and round-trips for structure, so the two groups are
// read against different expectations, not lined up as if they did the same
// thing. A tool that planned on some runs and not others lands in the planned
// group, since PlannedRuns counts any run that did.
func WriteTable(w io.Writer, sums []ToolSummary) {
	var planned, flat []ToolSummary
	for _, s := range sums {
		if s.PlannedRuns > 0 {
			planned = append(planned, s)
		} else {
			flat = append(flat, s)
		}
	}
	if len(planned) > 0 {
		fmt.Fprintln(w, "planned (wrote a plan or spawned a subagent)")
		writeSummaryTable(w, planned, true)
	}
	if len(flat) > 0 {
		if len(planned) > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "no plan (single flat loop)")
		writeSummaryTable(w, flat, false)
	}
}

// writeSummaryTable renders one group of summaries. The planned group carries the
// PLANNED and SUBAG columns; the flat group drops them, since both are zero there
// by definition and a column of zeros is noise, not information.
func writeSummaryTable(w io.Writer, sums []ToolSummary, withPlan bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if withPlan {
		fmt.Fprintln(tw, "TOOL\tVERSION\tRELEASED\tRUNS\tPASS@1\tPASS\tDROPS\tREQS\tPLANNED\tSUBAG\tTOKENS\tCACHED\tCOST-USD\tRSS-MB\tTTFB-MS\tTOTAL-S\tMODEL-S\tTOOL-S\tINSTALL-MB")
	} else {
		fmt.Fprintln(tw, "TOOL\tVERSION\tRELEASED\tRUNS\tPASS@1\tPASS\tDROPS\tREQS\tTOKENS\tCACHED\tCOST-USD\tRSS-MB\tTTFB-MS\tTOTAL-S\tMODEL-S\tTOOL-S\tINSTALL-MB")
	}
	for _, s := range sums {
		if withPlan {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\t%d\t%d\t%d\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n",
				s.Tool, blankDash(s.Version), blankDash(s.Released),
				s.Runs, s.FirstTry, s.Passed, blankZero(s.StreamFailRuns),
				s.AvgModelCalls, s.PlannedRuns, s.Subagents,
				s.TotalTokens, blankZero(s.CachedTokens), blankCost(s.TotalCostUSD),
				s.AvgRSSMB, s.AvgTTFBMS, s.TotalS, s.ModelS, s.ToolS, s.InstallMB)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\t%d\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n",
				s.Tool, blankDash(s.Version), blankDash(s.Released),
				s.Runs, s.FirstTry, s.Passed, blankZero(s.StreamFailRuns),
				s.AvgModelCalls,
				s.TotalTokens, blankZero(s.CachedTokens), blankCost(s.TotalCostUSD),
				s.AvgRSSMB, s.AvgTTFBMS, s.TotalS, s.ModelS, s.ToolS, s.InstallMB)
		}
	}
	tw.Flush()
}

// WritePromptReport renders the results of an ad-hoc prompt run: a metrics table
// comparing what each tool spent on the same instruction, then each tool's answer
// in full. There is no pass or fail here, so the comparison is the cost of the
// answer and the answer itself, side by side on the one shared model.
func WritePromptReport(w io.Writer, prompt string, results []*Result) {
	fmt.Fprintf(w, "prompt: %s\n\n", firstLineOf(prompt))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TOOL\tTOKENS\tCACHED\tTTFB-MS\tRSS-MB\tWALL-S\tREQS\tEXIT")
	for _, r := range results {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%d\t%d\t%d\t%d\t%d\n",
			r.Tool, r.Tokens.Total, blankZero(r.Tokens.Cached), r.Latency.AvgTTFB,
			r.MaxRSSKB/1024, r.WallSeconds, r.Requests, r.ExitCode)
	}
	tw.Flush()
	for _, r := range results {
		fmt.Fprintf(w, "\n=== %s ===\n%s\n", r.Tool, answerOrDash(r.Answer))
	}
}

// answerOrDash renders an empty answer as a dash so a tool that produced nothing
// reads as such rather than as a blank gap.
func answerOrDash(s string) string {
	if s == "" {
		return "(no output)"
	}
	return s
}

// blankZero renders an unreported count as a dash, so a provider that never
// reports prompt caching reads as unknown rather than as a real zero.
func blankZero(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

// blankCost renders an unreported cost as a dash and otherwise a dollar figure
// with enough precision to show a fraction of a cent.
func blankCost(c float64) string {
	if c == 0 {
		return "-"
	}
	return fmt.Sprintf("%.4f", c)
}
