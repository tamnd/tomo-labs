//go:build ignore

// rerun runs the full sweep and refreshes every number the docs and README
// quote, so a rerun leaves the published results correct rather than drifting
// from what actually ran.
//
// It sweeps three suites (the core scenarios, then the aider and evalplus eval
// tiers), captures each tool's version with meta, and then rewrites the results
// tables in the docs and the README from the captured runs. The tables live
// between HTML-comment markers like:
//
//	<!-- lab:results-plan:begin -->
//	...generated table...
//	<!-- lab:results-plan:end -->
//
// so only the numbers are regenerated and the surrounding prose is left alone.
// The prose paragraphs that quote specific figures ("187k against 732k") are
// not touched, since narrative cannot be generated; reread them after a rerun.
//
// Everything reads from the same captured runs `lab report` reads, so the docs
// can be refreshed without a fresh sweep once the runs exist:
//
//	go run scripts/rerun.go               # build off, sweep, meta, refresh docs
//	go run scripts/rerun.go -run=false    # refresh docs from the runs already captured
//	go run scripts/rerun.go -build -no-cache   # rebuild images first, then sweep
//
// A sweep needs OPENCODE_API_KEY in the environment; refreshing docs does not.
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/tomo-labs/pkg/lab"
)

// baselineTool is the tool whose per-scenario breakdown the results page prints,
// so the suite total is not a black box. It is the harness's own reference agent.
const baselineTool = "tomo"

// suites is the set the sweep covers: the core scenarios first, then each eval
// tier. An empty name is the core suite.
var suites = []string{"", "aider", "evalplus"}

func main() {
	root := flag.String("root", ".", "repo root holding scenarios/, docs/, and README.md")
	doBuild := flag.Bool("build", false, "build the images before sweeping")
	noCache := flag.Bool("no-cache", false, "build with --no-cache (implies -build)")
	doRun := flag.Bool("run", true, "run the core, aider, and evalplus sweeps")
	doDocs := flag.Bool("docs", true, "refresh the results tables in the docs and README")
	flag.Parse()

	ctx := context.Background()
	absRoot, err := filepath.Abs(*root)
	must(err)
	if *noCache {
		*doBuild = true
	}

	if *doBuild {
		fmt.Fprintln(os.Stderr, "[rerun] building images")
		must(newLab(ctx, absRoot, "").Build(ctx, "", *noCache))
	}

	if *doRun {
		for _, s := range suites {
			fmt.Fprintf(os.Stderr, "[rerun] sweep %s\n", suiteLabel(s))
			_, err := newLab(ctx, absRoot, s).RunAll(ctx, nil, nil)
			must(err)
		}
		fmt.Fprintln(os.Stderr, "[rerun] meta")
		must(newLab(ctx, absRoot, "").RefreshMeta(ctx))
	}

	if *doDocs {
		fmt.Fprintln(os.Stderr, "[rerun] refreshing docs")
		must(refreshDocs(ctx, absRoot))
	}
	fmt.Fprintln(os.Stderr, "[rerun] done")
}

func newLab(ctx context.Context, root, suite string) *lab.Lab {
	cfg := lab.DefaultConfig()
	cfg.Root = root
	cfg.Suite = suite
	l, err := lab.New(ctx, cfg)
	must(err)
	return l
}

func suiteLabel(s string) string {
	if s == "" {
		return "core"
	}
	return s
}

// refreshDocs regenerates every marked results region in the docs and README
// from the captured runs. It reads the core suite, its 00-hello baseline, the
// per-scenario breakdown of the baseline tool, and each eval tier, then rewrites
// the tables in place.
func refreshDocs(ctx context.Context, root string) error {
	core := newLab(ctx, root, "")
	coreSums, err := core.Report(ctx, "")
	if err != nil {
		return err
	}
	if len(coreSums) == 0 {
		return fmt.Errorf("no core runs captured; run a sweep first")
	}
	plan, flat := splitPlanFlat(coreSums)

	hello, err := core.Report(ctx, "00-hello")
	if err != nil {
		return err
	}
	sortByTokens(hello)

	scens, err := core.Scenarios()
	if err != nil {
		return err
	}
	var scenRows []scenRow
	for _, s := range scens {
		rows, err := core.Report(ctx, s.Name)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.Tool == baselineTool {
				scenRows = append(scenRows, scenRow{s.Name, r.TotalTokens, ordinalAttempt(r.AvgAttempts)})
			}
		}
	}
	sort.Slice(scenRows, func(i, j int) bool { return scenRows[i].tokens < scenRows[j].tokens })

	date := time.Now().Format("2006-01-02")
	snapshot := fmt.Sprintf("Snapshot taken %s. All %d wired tools were rerun on %s, each at the version shown, against the same OpenCode Zen deepseek endpoint with the same deterministic settings.", date, len(coreSums), date)

	// docs/content/guides/results.md: the full tables.
	if err := replaceRegion(root, "docs/content/guides/results.md", "results-snapshot", snapshot); err != nil {
		return err
	}
	if err := replaceRegion(root, "docs/content/guides/results.md", "results-plan", fullTable(plan)); err != nil {
		return err
	}
	if err := replaceRegion(root, "docs/content/guides/results.md", "results-flat", fullTable(flat)); err != nil {
		return err
	}
	if err := replaceRegion(root, "docs/content/guides/results.md", "hello-baseline", helloTable(hello)); err != nil {
		return err
	}
	if err := replaceRegion(root, "docs/content/guides/results.md", "baseline-scenarios", scenTable(scenRows)); err != nil {
		return err
	}

	// README.md: the trimmed tables.
	if err := replaceRegion(root, "README.md", "results-plan", readmeTable(plan)); err != nil {
		return err
	}
	if err := replaceRegion(root, "README.md", "results-flat", readmeTable(flat)); err != nil {
		return err
	}

	// The eval tiers: one pass-rate table per tier.
	for _, s := range []string{"aider", "evalplus"} {
		sums, err := newLab(ctx, root, s).Report(ctx, "")
		if err != nil {
			return err
		}
		sortByPass(sums)
		rel := "docs/content/evals/" + s + ".md"
		if err := replaceRegion(root, rel, s+"-results", evalTable(sums)); err != nil {
			return err
		}
	}
	return nil
}

type scenRow struct {
	name    string
	tokens  int
	attempt string
}

// splitPlanFlat groups the summaries the way the report does: tools that ever
// laid out a plan or spawned a subagent, and tools that ran a flat loop. Each
// group is ordered by total tokens, cheapest first.
func splitPlanFlat(sums []lab.ToolSummary) (plan, flat []lab.ToolSummary) {
	for _, s := range sums {
		if s.PlannedRuns > 0 {
			plan = append(plan, s)
		} else {
			flat = append(flat, s)
		}
	}
	sortByTokens(plan)
	sortByTokens(flat)
	return
}

func sortByTokens(sums []lab.ToolSummary) {
	sort.Slice(sums, func(i, j int) bool { return sums[i].TotalTokens < sums[j].TotalTokens })
}

// sortByPass orders an eval tier by pass rate, best first, then by tokens so a
// tie breaks toward the cheaper tool.
func sortByPass(sums []lab.ToolSummary) {
	sort.Slice(sums, func(i, j int) bool {
		pi := passRate(sums[i])
		pj := passRate(sums[j])
		if pi != pj {
			return pi > pj
		}
		return sums[i].TotalTokens < sums[j].TotalTokens
	})
}

func passRate(s lab.ToolSummary) float64 {
	if s.Runs == 0 {
		return 0
	}
	return float64(s.Passed) / float64(s.Runs)
}

func fullTable(sums []lab.ToolSummary) string {
	var b strings.Builder
	b.WriteString("| tool | version | pass | 1st | plans | tokens | avg | cache | cost | rss | ttfb | wall | install |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, s := range sums {
		b.WriteString(fmt.Sprintf("\n| %s | %s | %d/%d | %d | %d/%d | %s | %s | %d%% | $%.4f | %dMB | %dms | %ds | %dMB |",
			s.Tool, s.Version, s.Passed, s.Runs, s.FirstTry, s.PlannedRuns, s.Runs,
			commas(s.TotalTokens), commas(s.AvgTokens), cachePct(s), s.TotalCostUSD,
			s.AvgRSSMB, s.AvgTTFBMS, s.AvgWallS, s.InstallMB))
	}
	return b.String()
}

func readmeTable(sums []lab.ToolSummary) string {
	var b strings.Builder
	b.WriteString("| tool | version | pass | plans | tokens | cost | install |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |")
	for _, s := range sums {
		b.WriteString(fmt.Sprintf("\n| %s | %s | %d/%d | %d/%d | %s | $%.3f | %dMB |",
			s.Tool, s.Version, s.Passed, s.Runs, s.PlannedRuns, s.Runs,
			commas(s.TotalTokens), s.TotalCostUSD, s.InstallMB))
	}
	return b.String()
}

func helloTable(sums []lab.ToolSummary) string {
	var b strings.Builder
	b.WriteString("| tool | tokens | ttfb | rss |\n")
	b.WriteString("| --- | --- | --- | --- |")
	for _, s := range sums {
		b.WriteString(fmt.Sprintf("\n| %s | %s | %dms | %dMB |",
			s.Tool, commas(s.TotalTokens), s.AvgTTFBMS, s.AvgRSSMB))
	}
	return b.String()
}

func scenTable(rows []scenRow) string {
	var b strings.Builder
	b.WriteString("| scenario | tokens | attempt |\n")
	b.WriteString("| --- | --- | --- |")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("\n| %s | %s | %s |", r.name, commas(r.tokens), r.attempt))
	}
	return b.String()
}

func evalTable(sums []lab.ToolSummary) string {
	var b strings.Builder
	b.WriteString("| tool | version | pass | tokens | cost |\n")
	b.WriteString("| --- | --- | --- | --- | --- |")
	for _, s := range sums {
		b.WriteString(fmt.Sprintf("\n| %s | %s | %d/%d | %s | $%.4f |",
			s.Tool, s.Version, s.Passed, s.Runs, commas(s.TotalTokens), s.TotalCostUSD))
	}
	return b.String()
}

// cachePct is the share of a tool's prompt tokens the provider served from its
// cache, the "cache" column. A tool that never reports caching reads as 0.
func cachePct(s lab.ToolSummary) int {
	if s.PromptTokens <= 0 {
		return 0
	}
	return int(math.Round(100 * float64(s.CachedTokens) / float64(s.PromptTokens)))
}

// ordinalAttempt renders the attempt a run passed on, rounded from the average
// over the scenario's runs, which is one run per tool in a suite so the average
// is exact.
func ordinalAttempt(avg float64) string {
	switch a := int(math.Round(avg)); a {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	default:
		return strconv.Itoa(a) + "th"
	}
}

// commas groups an integer with thousands separators, so 187404 reads 187,404.
func commas(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// replaceRegion rewrites the text between a region's begin and end markers in a
// file, leaving the markers and everything around them in place. A missing
// marker pair is an error, so a doc that forgot to mark a region fails loudly
// rather than drifting silently.
func replaceRegion(root, rel, key, body string) error {
	path := filepath.Join(root, rel)
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	begin := "<!-- lab:" + key + ":begin -->"
	end := "<!-- lab:" + key + ":end -->"
	i := strings.Index(s, begin)
	j := strings.Index(s, end)
	if i < 0 || j < 0 || j < i {
		return fmt.Errorf("%s: markers for %q not found", rel, key)
	}
	next := s[:i+len(begin)] + "\n" + body + "\n" + s[j:]
	if next == s {
		return nil
	}
	return os.WriteFile(path, []byte(next), 0o644)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
