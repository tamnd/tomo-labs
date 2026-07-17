package lab

import (
	"os"
	"path/filepath"
	"testing"
)

// A trace dir with a GNU time report and the proxy's jsonl files parses into the
// numbers the report is built from, and only the timed 200 completions count
// toward latency.
func TestReadTrace(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "time.txt", "\tMaximum resident set size (kbytes): 20480\n\tElapsed (wall clock) time (h:mm:ss or m:ss): 0:12.34\n")
	write(t, dir, "requests.jsonl", `{"seq":1}
{"seq":2}
`)
	write(t, dir, "usage.jsonl", `{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
{"prompt_tokens":20,"completion_tokens":7,"total_tokens":27}
`)
	// One readiness GET (no completion path), one rejected 500, and two good
	// completions the average should be taken over.
	write(t, dir, "latency.jsonl", `{"status":200,"path":"/zen/","ttfb_ms":5,"total_ms":9}
{"status":500,"path":"/zen/v1/chat/completions","ttfb_ms":100,"total_ms":200}
{"status":429,"path":"/zen/v1/chat/completions","retry_after_s":20}
{"status":429,"path":"/zen/v1/chat/completions","retry_after_s":8}
{"status":200,"path":"/zen/v1/chat/completions","ttfb_ms":100,"total_ms":300}
{"status":200,"path":"/zen/v1/chat/completions","ttfb_ms":200,"total_ms":500}
`)

	m := readTrace(dir)
	if m.MaxRSSKB != 20480 {
		t.Errorf("rss = %d, want 20480", m.MaxRSSKB)
	}
	if m.ElapsedClock != "0:12.34" {
		t.Errorf("elapsed = %q, want 0:12.34", m.ElapsedClock)
	}
	if m.Requests != 2 {
		t.Errorf("requests = %d, want 2", m.Requests)
	}
	if m.Tokens != (Tokens{Prompt: 30, Completion: 12, Total: 42}) {
		t.Errorf("tokens = %+v, want 30/12/42", m.Tokens)
	}
	if m.Latency != (Latency{AvgTTFB: 150, AvgTotal: 400, SumTotal: 800, Calls: 2}) {
		t.Errorf("latency = %+v, want ttfb 150 total 400 calls 2", m.Latency)
	}
	// The two 429 rows are counted apart from the timed 200s, and the longest
	// Retry-After they carried is kept.
	if m.RateLimit == nil {
		t.Fatalf("rate_limit = nil, want 2 hits")
	}
	if *m.RateLimit != (RateLimit{Hits: 2, MaxRetryAfterS: 20}) {
		t.Errorf("rate_limit = %+v, want 2 hits / retry-after 20", *m.RateLimit)
	}
}

// A missing trace dir yields zero values, never a panic, so a tool that never
// wrote a trace still grades into a comparable row.
func TestReadTraceMissing(t *testing.T) {
	m := readTrace(filepath.Join(t.TempDir(), "does-not-exist"))
	if m.MaxRSSKB != 0 || m.Requests != 0 || m.Tokens.Total != 0 || m.Latency.Calls != 0 {
		t.Errorf("missing trace = %+v, want all zero", m)
	}
}

// dirSizeKB sums the files under a tree.
func TestDirSizeKB(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", string(make([]byte, 2048)))
	write(t, dir, filepath.Join("sub", "b.txt"), string(make([]byte, 1024)))
	if got := dirSizeKB(dir); got != 3 {
		t.Errorf("dirSizeKB = %d, want 3", got)
	}
}

// summarize aggregates every run, repeats included: n is the run count, pass
// and pass@1 are counts over that n, and the token column is a median with its
// interquartile range rather than a sum.
func TestSummarize(t *testing.T) {
	results := []*Result{
		{Tool: "tomo", Scenario: "a", Passed: true, Attempts: 1, Tokens: Tokens{Total: 100, Cached: 20}, CostUSD: 0.01, MaxRSSKB: 1024 * 30, WallSeconds: 10, Latency: Latency{AvgTTFB: 200, Calls: 2}, InstallKB: 1024 * 40},
		{Tool: "tomo", Scenario: "a", Passed: true, Attempts: 2, Tokens: Tokens{Total: 200, Cached: 30}, CostUSD: 0.02, MaxRSSKB: 1024 * 50, WallSeconds: 20, Latency: Latency{AvgTTFB: 400, Calls: 3}, InstallKB: 1024 * 40},
		{Tool: "tomo", Scenario: "b", Passed: false, Attempts: 1, Tokens: Tokens{Total: 900, Cached: 10}, CostUSD: 0.09, MaxRSSKB: 1024 * 40, WallSeconds: 90, Latency: Latency{AvgTTFB: 300, Calls: 1}, InstallKB: 1024 * 40},
		{Tool: "openclaw", Scenario: "a", Passed: false, Attempts: 3, Tokens: Tokens{Total: 300}, MaxRSSKB: 1024 * 200, WallSeconds: 30, Latency: Latency{}, InstallKB: 1024 * 300},
	}
	sums := summarize(results)
	if len(sums) != 2 {
		t.Fatalf("got %d tools, want 2", len(sums))
	}
	// Ranked on aggregated pass@1 rate: tomo solved 1 of 3 first-try, openclaw
	// 0 of 1, so tomo leads regardless of cost.
	tomo, oc := sums[0], sums[1]
	if tomo.Tool != "tomo" || oc.Tool != "openclaw" {
		t.Fatalf("order = %s,%s want tomo,openclaw", sums[0].Tool, sums[1].Tool)
	}
	// Every run counts toward n, repeats included, and the distinct-scenario
	// count sits beside it so ten reps of one task never read as ten tasks.
	if tomo.Runs != 3 || tomo.Scenarios != 2 || tomo.Passed != 2 {
		t.Errorf("tomo n/scen/passed = %d/%d/%d, want 3/2/2", tomo.Runs, tomo.Scenarios, tomo.Passed)
	}
	if tomo.FirstTry != 1 || tomo.Retried != 1 {
		t.Errorf("tomo first_try/retried = %d/%d, want 1/1", tomo.FirstTry, tomo.Retried)
	}
	// Tokens are a median with IQR over 100/200/900: the 900 runaway widens the
	// spread instead of dragging the headline number.
	if tomo.Tokens != (Dist{Median: 200, P25: 150, P75: 550}) {
		t.Errorf("tomo tokens = %+v, want median 200 IQR 150-550", tomo.Tokens)
	}
	if tomo.CachedMedian != 20 {
		t.Errorf("tomo cached median = %d, want 20", tomo.CachedMedian)
	}
	if tomo.CostMedianUSD < 0.0199 || tomo.CostMedianUSD > 0.0201 {
		t.Errorf("tomo cost median = %v, want ~0.02", tomo.CostMedianUSD)
	}
	if tomo.RSSMedianMB != 40 {
		t.Errorf("tomo rss median mb = %d, want 40", tomo.RSSMedianMB)
	}
	if tomo.TTFBMedianMS != 300 {
		t.Errorf("tomo ttfb median = %d, want 300", tomo.TTFBMedianMS)
	}
	if tomo.WallMedianS != 20 {
		t.Errorf("tomo wall median = %d, want 20", tomo.WallMedianS)
	}
	if tomo.InstallMB != 40 {
		t.Errorf("tomo install mb = %d, want 40", tomo.InstallMB)
	}
	// openclaw never produced a timed completion, so its latency stays zero
	// instead of dividing by zero.
	if oc.TTFBMedianMS != 0 {
		t.Errorf("openclaw ttfb median = %d, want 0", oc.TTFBMedianMS)
	}
	if oc.Passed != 0 || oc.Retried != 1 || oc.FirstTry != 0 {
		t.Errorf("openclaw passed/retried/first = %d/%d/%d, want 0/1/0", oc.Passed, oc.Retried, oc.FirstTry)
	}
}

// The ranking compares pass@1 as a fraction of n, not as a raw count, so a tool
// at n=5 with 4 first-try passes outranks a tool at n=20 with 10, and median
// cost per run breaks ties rather than an n-dependent total.
func TestRankingByRate(t *testing.T) {
	var results []*Result
	for i := range 20 {
		results = append(results, &Result{Tool: "big-n", Scenario: "a", Passed: i < 10, Attempts: 1, Tokens: Tokens{Total: 100}})
	}
	for i := range 5 {
		results = append(results, &Result{Tool: "small-n", Scenario: "a", Passed: i < 4, Attempts: 1, Tokens: Tokens{Total: 100}})
	}
	sums := summarize(results)
	if sums[0].Tool != "small-n" {
		t.Errorf("leader = %s, want small-n (4/5 beats 10/20)", sums[0].Tool)
	}
}

// summarizeScenarios builds the per-task cells: repeats of one tool on one
// scenario aggregate into one row with n and the pass fraction over it.
func TestSummarizeScenarios(t *testing.T) {
	results := []*Result{
		{Tool: "tomo", Scenario: "flaky", Passed: true, Attempts: 1, Tokens: Tokens{Total: 100}, WallSeconds: 10},
		{Tool: "tomo", Scenario: "flaky", Passed: false, Attempts: 1, Tokens: Tokens{Total: 300}, WallSeconds: 30},
		{Tool: "tomo", Scenario: "flaky", Passed: true, Attempts: 1, Tokens: Tokens{Total: 200}, WallSeconds: 20},
		{Tool: "tomo", Scenario: "easy", Passed: true, Attempts: 1, Tokens: Tokens{Total: 50}, WallSeconds: 5},
	}
	cells := summarizeScenarios(results)
	if len(cells) != 2 {
		t.Fatalf("got %d cells, want 2", len(cells))
	}
	// Ordered by scenario name, so easy first.
	if cells[0].Scenario != "easy" || cells[0].Runs != 1 {
		t.Errorf("cell 0 = %s n=%d, want easy n=1", cells[0].Scenario, cells[0].Runs)
	}
	flaky := cells[1]
	if flaky.Runs != 3 || flaky.Passed != 2 || flaky.FirstTry != 2 {
		t.Errorf("flaky n/pass/first = %d/%d/%d, want 3/2/2", flaky.Runs, flaky.Passed, flaky.FirstTry)
	}
	if flaky.Tokens.Median != 200 || flaky.WallMedianS != 20 {
		t.Errorf("flaky medians = %d tokens %ds wall, want 200/20", flaky.Tokens.Median, flaky.WallMedianS)
	}
}

// pctl interpolates between ranks; distOf is median plus IQR over the values.
func TestDistOf(t *testing.T) {
	d := distOf([]int{100, 200, 900})
	if d != (Dist{Median: 200, P25: 150, P75: 550}) {
		t.Errorf("distOf(100,200,900) = %+v, want 200/150/550", d)
	}
	if d := distOf([]int{42}); d != (Dist{Median: 42, P25: 42, P75: 42}) {
		t.Errorf("distOf(42) = %+v, want all 42", d)
	}
	if d := distOf(nil); d != (Dist{}) {
		t.Errorf("distOf(nil) = %+v, want zero", d)
	}
	// Even n: the median is the midpoint of the two middle values.
	if m := medianInt([]int{10, 20, 30, 40}); m != 25 {
		t.Errorf("medianInt(10,20,30,40) = %d, want 25", m)
	}
	if m := medianFloat([]float64{1, 3}); m != 2 {
		t.Errorf("medianFloat(1,3) = %v, want 2", m)
	}
}

// The token cell carries the spread with the median; a single run renders as
// the bare number so n=1 never grows a fake spread.
func TestDistCell(t *testing.T) {
	if got := distCell(Dist{Median: 200, P25: 150, P75: 550}, 3); got != "200 (150-550)" {
		t.Errorf("distCell n=3 = %q", got)
	}
	if got := distCell(Dist{Median: 200, P25: 200, P75: 200}, 1); got != "200" {
		t.Errorf("distCell n=1 = %q, want bare median", got)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
