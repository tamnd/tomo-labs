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

// Dist is a token or latency column's shape over a tool's runs: the median with
// its interquartile range. Medians with spreads, never means, because agent-run
// distributions are long-tailed and a single runaway rep would drag a mean into
// fiction; the IQR is the spread statistic for the same reason, since a min-max
// range would be dominated by exactly that runaway. The spread renders in the
// same cell as the median so the median is never quoted alone.
type Dist struct {
	Median int `json:"median"`
	P25    int `json:"p25"`
	P75    int `json:"p75"`
}

// ToolSummary is one tool's aggregate over every graded run in the data dir, the
// row the comparison table is built from. Runs is the honest n: every run counts,
// repeats included, so a number quoted off this row always has its n attached.
// Which runs are in the data dir is a curation question, not a report question:
// a re-run campaign at new pins starts from a clean dir or prunes the stale runs
// it no longer stands behind.
type ToolSummary struct {
	Tool     string `json:"tool"`
	Version  string `json:"version,omitempty"`
	Released string `json:"released,omitempty"`
	// Scenarios is how many distinct scenarios the n runs cover, so a tool with
	// ten reps of one task never reads as a tool graded across ten tasks.
	Scenarios int `json:"scenarios"`
	Runs      int `json:"runs"`
	Passed    int `json:"passed"`
	FirstTry  int `json:"first_try"`
	Retried   int `json:"retried"`
	// Per-run medians with spread where the column is long-tailed. Sums are gone
	// on purpose: with aggregation over repeats, a sum scales with n and two tools
	// at different n stop being comparable.
	Tokens        Dist    `json:"tokens"`
	CachedMedian  int     `json:"cached_median,omitempty"`
	CostMedianUSD float64 `json:"cost_median_usd,omitempty"`
	ReqsMedian    int     `json:"reqs_median"`
	PlannedRuns   int     `json:"planned_runs"`
	Subagents     int     `json:"subagents"`
	InstallMB     int     `json:"install_mb"`
	RSSMedianMB   int     `json:"rss_median_mb"`
	TTFBMedianMS  int     `json:"ttfb_median_ms"`
	// The per-run time medians, in seconds: WallMedianS is the whole run,
	// ModelMedianS the part spent waiting on the model, ToolMedianS the rest of
	// that run (tool execution and agent glue), each medianed independently.
	WallMedianS  int `json:"wall_median_s"`
	ModelMedianS int `json:"model_median_s"`
	ToolMedianS  int `json:"tool_median_s"`
	// StreamFailRuns is how many of the tool's runs hit an upstream stream drop,
	// whether it was retried away or left in the recorded attempt. It is the honest
	// count of gateway faults the tool ran into, kept apart from real failures.
	StreamFailRuns int `json:"stream_fail_runs,omitempty"`
}

// ScenarioStats is one tool's aggregate over its repeats of one scenario, the
// per-task cell under the tool rows. This is where a flaky task shows its true
// pass rate and where a runaway rep shows up as spread rather than vanishing
// into a cross-scenario total.
type ScenarioStats struct {
	Scenario      string  `json:"scenario"`
	Tool          string  `json:"tool"`
	Runs          int     `json:"runs"`
	Passed        int     `json:"passed"`
	FirstTry      int     `json:"first_try"`
	Tokens        Dist    `json:"tokens"`
	CostMedianUSD float64 `json:"cost_median_usd,omitempty"`
	ReqsMedian    int     `json:"reqs_median"`
	WallMedianS   int     `json:"wall_median_s"`
}

// Report reads every result.json under the data dir and aggregates it, per tool
// and per tool-and-scenario, over every graded run. Repeats are the point: the
// campaign's measurement law says a pass rate is a raw fraction with its n
// visible and a token claim is a median over repeats with the spread shown, so
// the report keeps every run rather than the latest one and lets n say how much
// the numbers mean. Staleness is handled by data-dir hygiene (prune or re-run),
// not by the report silently discarding history. A non-empty scenario filter
// narrows both aggregates to runs whose scenario name contains it, which is how
// the report focuses on one scenario at a time.
func (l *Lab) Report(_ context.Context, scenario string) ([]ToolSummary, []ScenarioStats, error) {
	var results []*Result
	err := l.walkResults(func(path string, r *Result) {
		// Ungraded prompt runs (lab -p) have no pass or fail, so they never belong
		// in the scenario comparison; skip them here.
		if !r.Ungraded && (scenario == "" || strings.Contains(r.Scenario, scenario)) {
			results = append(results, r)
		}
	})
	if err != nil {
		return nil, nil, err
	}
	sums := summarize(results)
	// Version and release date are properties of the tool image, captured at build
	// time, so join them in here rather than reading them off every run.
	for i := range sums {
		m := l.toolMetaOf(sums[i].Tool)
		sums[i].Version = m.Version
		sums[i].Released = m.Released
	}
	return sums, summarizeScenarios(results), nil
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

// pctl returns the p-th quantile (0..1) of already-sorted values by linear
// interpolation between the two nearest ranks, rounded to the nearest int.
func pctl(sorted []int, p float64) int {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	rank := p * float64(n-1)
	lo := int(rank)
	if lo >= n-1 {
		return sorted[n-1]
	}
	frac := rank - float64(lo)
	return int(float64(sorted[lo])*(1-frac) + float64(sorted[lo+1])*frac + 0.5)
}

// distOf sorts a copy of the values and returns their median with the
// interquartile range. At n=1 all three collapse to the one value, which the
// renderer shows as the bare number rather than a fake spread.
func distOf(vals []int) Dist {
	s := make([]int, len(vals))
	copy(s, vals)
	sort.Ints(s)
	return Dist{Median: pctl(s, 0.5), P25: pctl(s, 0.25), P75: pctl(s, 0.75)}
}

// medianInt is distOf for the columns that render without a spread.
func medianInt(vals []int) int {
	return distOf(vals).Median
}

// medianFloat is the median of a float column (cost), same convention as pctl.
func medianFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := make([]float64, len(vals))
	copy(s, vals)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
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

// runCostUSD is one run's cost: what the provider billed, or the reference-rate
// estimate when it billed nothing, so the column stays comparable across tiers.
func runCostUSD(r *Result) float64 {
	if r.CostUSD > 0 {
		return r.CostUSD
	}
	return estimatedCostUSD(r.Tokens)
}

func summarize(results []*Result) []ToolSummary {
	byTool := map[string][]*Result{}
	for _, r := range results {
		byTool[r.Tool] = append(byTool[r.Tool], r)
	}
	var out []ToolSummary
	for tool, rs := range byTool {
		s := ToolSummary{Tool: tool, Runs: len(rs)}
		scenarios := map[string]bool{}
		var tokens, cached, reqs, rss, ttfb, wall, model, toolS []int
		var cost []float64
		for _, r := range rs {
			scenarios[r.Scenario] = true
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
			if r.Orchestration.Planned {
				s.PlannedRuns++
			}
			s.Subagents += r.Orchestration.Subagents
			tokens = append(tokens, r.Tokens.Total)
			cached = append(cached, r.Tokens.Cached)
			reqs = append(reqs, r.Orchestration.ModelCalls)
			cost = append(cost, runCostUSD(r))
			rss = append(rss, r.MaxRSSKB)
			wall = append(wall, r.WallSeconds)
			m := r.Latency.SumTotal / 1000
			model = append(model, m)
			toolS = append(toolS, max(r.WallSeconds-m, 0))
			if r.StreamFail != nil {
				s.StreamFailRuns++
			}
			if r.Latency.Calls > 0 {
				ttfb = append(ttfb, r.Latency.AvgTTFB)
			}
			// Install footprint is a property of the tool, not the run, so the
			// last one seen wins; they are all the same.
			s.InstallMB = r.InstallKB / 1024
		}
		s.Scenarios = len(scenarios)
		s.Tokens = distOf(tokens)
		s.CachedMedian = medianInt(cached)
		s.CostMedianUSD = medianFloat(cost)
		s.ReqsMedian = medianInt(reqs)
		s.RSSMedianMB = medianInt(rss) / 1024
		s.TTFBMedianMS = medianInt(ttfb)
		s.WallMedianS = medianInt(wall)
		s.ModelMedianS = medianInt(model)
		s.ToolMedianS = medianInt(toolS)
		out = append(out, s)
	}
	sortSummaries(out)
	return out
}

// sortSummaries ranks on aggregated pass@1 first, then median cost per run.
// pass@1 as a fraction of n (a task solved on the first attempt, no retry) is
// the capability metric every code benchmark headlines, and the fraction rather
// than the raw count keeps tools at different n comparable. Median cost breaks
// ties: among tools that solve the same fraction first-try, the cheapest run
// wins, since cost is what the tokens actually buy, and cost weights a cached
// token far below a generated one, so a cache-heavy multi-turn tool reads as
// the bargain it is. The tool name is the final tie-break for a stable order.
func sortSummaries(out []ToolSummary) {
	rate := func(s ToolSummary) float64 {
		if s.Runs == 0 {
			return 0
		}
		return float64(s.FirstTry) / float64(s.Runs)
	}
	sort.Slice(out, func(i, j int) bool {
		if ri, rj := rate(out[i]), rate(out[j]); ri != rj {
			return ri > rj
		}
		if out[i].CostMedianUSD != out[j].CostMedianUSD {
			return out[i].CostMedianUSD < out[j].CostMedianUSD
		}
		return out[i].Tool < out[j].Tool
	})
}

// summarizeScenarios builds the per-task cells: one row per tool and scenario
// over that pair's repeats, ordered by scenario then by the same pass@1-then-
// cost ranking as the tool table so the leader reads off the top of each block.
func summarizeScenarios(results []*Result) []ScenarioStats {
	byCell := map[string][]*Result{}
	for _, r := range results {
		byCell[r.Scenario+"\x00"+r.Tool] = append(byCell[r.Scenario+"\x00"+r.Tool], r)
	}
	var out []ScenarioStats
	for _, rs := range byCell {
		c := ScenarioStats{Scenario: rs[0].Scenario, Tool: rs[0].Tool, Runs: len(rs)}
		var tokens, reqs, wall []int
		var cost []float64
		for _, r := range rs {
			if r.Passed {
				c.Passed++
			}
			if max(r.Attempts, 1) == 1 && r.Passed {
				c.FirstTry++
			}
			tokens = append(tokens, r.Tokens.Total)
			reqs = append(reqs, r.Orchestration.ModelCalls)
			wall = append(wall, r.WallSeconds)
			cost = append(cost, runCostUSD(r))
		}
		c.Tokens = distOf(tokens)
		c.ReqsMedian = medianInt(reqs)
		c.WallMedianS = medianInt(wall)
		c.CostMedianUSD = medianFloat(cost)
		out = append(out, c)
	}
	rate := func(c ScenarioStats) float64 {
		return float64(c.FirstTry) / float64(c.Runs)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scenario != out[j].Scenario {
			return out[i].Scenario < out[j].Scenario
		}
		if ri, rj := rate(out[i]), rate(out[j]); ri != rj {
			return ri > rj
		}
		if out[i].CostMedianUSD != out[j].CostMedianUSD {
			return out[i].CostMedianUSD < out[j].CostMedianUSD
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
// by definition and a column of zeros is noise, not information. PASS@1 and PASS
// render as raw fractions over n, never bare percentages, and the token cell
// carries the median with its spread so neither is ever quoted without the other.
func writeSummaryTable(w io.Writer, sums []ToolSummary, withPlan bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if withPlan {
		fmt.Fprintln(tw, "TOOL\tVERSION\tRELEASED\tSCEN\tN\tPASS@1\tPASS\tDROPS\tREQS\tPLANNED\tSUBAG\tTOKENS\tCACHED\tCOST-USD\tRSS-MB\tTTFB-MS\tWALL-S\tMODEL-S\tTOOL-S\tINSTALL-MB")
	} else {
		fmt.Fprintln(tw, "TOOL\tVERSION\tRELEASED\tSCEN\tN\tPASS@1\tPASS\tDROPS\tREQS\tTOKENS\tCACHED\tCOST-USD\tRSS-MB\tTTFB-MS\tWALL-S\tMODEL-S\tTOOL-S\tINSTALL-MB")
	}
	for _, s := range sums {
		if withPlan {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\t%s\t%d\t%d\t%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n",
				s.Tool, blankDash(s.Version), blankDash(s.Released),
				s.Scenarios, s.Runs, fraction(s.FirstTry, s.Runs), fraction(s.Passed, s.Runs),
				blankZero(s.StreamFailRuns),
				s.ReqsMedian, s.PlannedRuns, s.Subagents,
				distCell(s.Tokens, s.Runs), blankZero(s.CachedMedian), blankCost(s.CostMedianUSD),
				s.RSSMedianMB, s.TTFBMedianMS, s.WallMedianS, s.ModelMedianS, s.ToolMedianS, s.InstallMB)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n",
				s.Tool, blankDash(s.Version), blankDash(s.Released),
				s.Scenarios, s.Runs, fraction(s.FirstTry, s.Runs), fraction(s.Passed, s.Runs),
				blankZero(s.StreamFailRuns),
				s.ReqsMedian,
				distCell(s.Tokens, s.Runs), blankZero(s.CachedMedian), blankCost(s.CostMedianUSD),
				s.RSSMedianMB, s.TTFBMedianMS, s.WallMedianS, s.ModelMedianS, s.ToolMedianS, s.InstallMB)
		}
	}
	tw.Flush()
}

// WriteScenarioTable renders the per-task cells: one block per scenario, one row
// per tool over that pair's repeats. This is where flakiness is visible: a task a
// tool passes 3 of 5 reads as exactly that, not as whatever the latest coin flip
// happened to land on.
func WriteScenarioTable(w io.Writer, cells []ScenarioStats) {
	if len(cells) == 0 {
		return
	}
	fmt.Fprintln(w, "per scenario (all repeats)")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SCENARIO\tTOOL\tN\tPASS@1\tPASS\tREQS\tTOKENS\tCOST-USD\tWALL-S")
	prev := ""
	for _, c := range cells {
		name := c.Scenario
		// Repeating the scenario name on every row of its block is noise; the
		// first row labels the block.
		if name == prev {
			name = ""
		} else {
			prev = name
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%d\t%s\t%s\t%d\n",
			name, c.Tool, c.Runs, fraction(c.FirstTry, c.Runs), fraction(c.Passed, c.Runs),
			c.ReqsMedian, distCell(c.Tokens, c.Runs), blankCost(c.CostMedianUSD), c.WallMedianS)
	}
	tw.Flush()
}

// fraction renders a pass count over its n, the only shape a rate is quoted in:
// 9/12 says both the rate and how much it means, where 75% alone says neither.
func fraction(k, n int) string {
	return fmt.Sprintf("%d/%d", k, n)
}

// distCell renders a median with its interquartile range in one cell, so the
// median is never quoted without its spread. A single run renders as the bare
// number: with n=1 there is no spread to show, and the N column already says
// how little the number means.
func distCell(d Dist, n int) string {
	if n <= 1 {
		return fmt.Sprintf("%d", d.Median)
	}
	return fmt.Sprintf("%d (%d-%d)", d.Median, d.P25, d.P75)
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
