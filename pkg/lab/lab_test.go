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
	write(t, dir, "usage.jsonl", `{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"reasoning_tokens":3}
{"prompt_tokens":20,"completion_tokens":7,"total_tokens":27,"reasoning_tokens":4}
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
	if m.Tokens != (Tokens{Prompt: 30, Completion: 12, Total: 42, Reasoning: 7}) {
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

func TestLatencyStatsIncludesNativeResponses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latency.jsonl")
	if err := os.WriteFile(path, []byte("{\"status\":200,\"path\":\"/v1/responses\",\"ttfb_ms\":120,\"total_ms\":450}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := latencyStats(path); got != (Latency{AvgTTFB: 120, AvgTotal: 450, SumTotal: 450, Calls: 1}) {
		t.Fatalf("latency = %+v", got)
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
// interquartile range rather than a sum. The token metric is fresh (prompt
// minus cached, plus output); the headline prompt total has no column.
func TestSummarize(t *testing.T) {
	results := []*Result{
		{Tool: "tomo", Scenario: "a", Passed: true, Attempts: 1, Tokens: Tokens{Prompt: 100, Cached: 20, Completion: 20}, CostUSD: 0.01, MaxRSSKB: 1024 * 30, WallSeconds: 10, Latency: Latency{AvgTTFB: 200, Calls: 2}, InstallKB: 1024 * 40},
		{Tool: "tomo", Scenario: "a", Passed: true, Attempts: 2, Tokens: Tokens{Prompt: 200, Cached: 30, Completion: 30}, CostUSD: 0.02, MaxRSSKB: 1024 * 50, WallSeconds: 20, Latency: Latency{AvgTTFB: 400, Calls: 3}, InstallKB: 1024 * 40},
		{Tool: "tomo", Scenario: "b", Passed: false, Attempts: 1, Tokens: Tokens{Prompt: 950, Cached: 100, Completion: 50}, CostUSD: 0.09, MaxRSSKB: 1024 * 40, WallSeconds: 90, Latency: Latency{AvgTTFB: 300, Calls: 1}, InstallKB: 1024 * 40},
		{Tool: "openclaw", Scenario: "a", Passed: false, Attempts: 3, Tokens: Tokens{Prompt: 300}, MaxRSSKB: 1024 * 200, WallSeconds: 30, Latency: Latency{}, InstallKB: 1024 * 300},
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
	// Fresh tokens per run are 100/200/900 (prompt minus cached, plus output);
	// the 900 runaway widens the spread instead of dragging the headline number.
	if tomo.Fresh != (Dist{Median: 200, P25: 150, P75: 550}) {
		t.Errorf("tomo fresh = %+v, want median 200 IQR 150-550", tomo.Fresh)
	}
	if tomo.CachedMedian != 30 {
		t.Errorf("tomo cached median = %d, want 30", tomo.CachedMedian)
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
	// openclaw's run has no bill and no model in the rate table, so its cost is
	// unknown and stays out of the median rather than averaging in as zero.
	if oc.CostUnpricedRuns != 1 {
		t.Errorf("openclaw unpriced runs = %d, want 1", oc.CostUnpricedRuns)
	}
}

// runCostUSD prefers the real bill, then the vendored rate table: a free tier
// with a paid twin prices at the twin's list rate and is flagged as a
// reference figure, a zero-rate free tier prices as an honest $0, and a model
// missing from the table is unpriced rather than free.
func TestRunCost(t *testing.T) {
	if usd, kind := runCostUSD(&Result{CostUSD: 0.5, Model: "deepseek-v4-flash-free"}); usd != 0.5 || kind != costBilled {
		t.Errorf("billed = %v/%v, want 0.5/billed", usd, kind)
	}
	usd, kind := runCostUSD(&Result{Model: "deepseek-v4-flash-free", Tokens: Tokens{Prompt: 1_000_000}})
	if kind != costReference || usd < 0.279 || usd > 0.281 {
		t.Errorf("reference = %v/%v, want ~0.28/reference", usd, kind)
	}
	if usd, kind := runCostUSD(&Result{Model: "hy3-free", Tokens: Tokens{Prompt: 1_000_000, Completion: 5000}}); usd < 0.2039 || usd > 0.2041 || kind != costReference {
		t.Errorf("hy3 reference = %v/%v, want ~0.204/reference", usd, kind)
	}
	if usd, kind := runCostUSD(&Result{Model: "mimo-v2.5-free", Tokens: Tokens{Prompt: 1_000_000}}); usd != 0 || kind != costListed {
		t.Errorf("zero-rate = %v/%v, want 0/listed", usd, kind)
	}
	if _, kind := runCostUSD(&Result{Model: "some-unknown-model"}); kind != costUnpriced {
		t.Errorf("unknown model kind = %v, want unpriced", kind)
	}
}

// The cost cell distinguishes unknown from free from estimated: no priced run
// is a dash, a reference-priced cell carries the tilde, and a true zero is a
// price.
func TestCostCell(t *testing.T) {
	if got := costCell(0, 0, 0); got != "-" {
		t.Errorf("unpriced cell = %q, want -", got)
	}
	if got := costCell(0.0012, 1, 3); got != "~0.0012" {
		t.Errorf("reference cell = %q, want ~0.0012", got)
	}
	if got := costCell(0, 0, 2); got != "0.0000" {
		t.Errorf("zero-rate cell = %q, want 0.0000", got)
	}
	if got := costCell(0.0300, 0, 2); got != "0.0300" {
		t.Errorf("billed cell = %q", got)
	}
}

// The ranking compares pass@1 as a fraction of n, not as a raw count, so a tool
// at n=5 with 4 first-try passes outranks a tool at n=20 with 10, and median
// cost per run breaks ties rather than an n-dependent total.
func TestRankingByRate(t *testing.T) {
	var results []*Result
	for i := range 20 {
		results = append(results, &Result{Tool: "big-n", Scenario: "a", Passed: i < 10, Attempts: 1, Tokens: Tokens{Prompt: 100}})
	}
	for i := range 5 {
		results = append(results, &Result{Tool: "small-n", Scenario: "a", Passed: i < 4, Attempts: 1, Tokens: Tokens{Prompt: 100}})
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
		{Tool: "tomo", Scenario: "flaky", Passed: true, Attempts: 1, Tokens: Tokens{Prompt: 100}, WallSeconds: 10},
		{Tool: "tomo", Scenario: "flaky", Passed: false, Attempts: 1, Tokens: Tokens{Prompt: 300}, WallSeconds: 30},
		{Tool: "tomo", Scenario: "flaky", Passed: true, Attempts: 1, Tokens: Tokens{Prompt: 200}, WallSeconds: 20},
		{Tool: "tomo", Scenario: "easy", Passed: true, Attempts: 1, Tokens: Tokens{Prompt: 50}, WallSeconds: 5},
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
	if flaky.Fresh.Median != 200 || flaky.WallMedianS != 20 {
		t.Errorf("flaky medians = %d fresh %ds wall, want 200/20", flaky.Fresh.Median, flaky.WallMedianS)
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

// A missing or blank tags file reads as explicit unaudited, never as a blank,
// and a recorded audit reads back exactly as written.
func TestReadTags(t *testing.T) {
	dir := t.TempDir()
	got := readTags(dir)
	if got.Reachability != TagUnaudited || got.Fairness != TagUnaudited {
		t.Errorf("missing tags.json = %+v, want unaudited/unaudited", got)
	}

	write(t, dir, "tags.json", `{"reachability":"substitution","fairness":"fair","source":"one-line gold","audited":"2026-07-17"}`)
	got = readTags(dir)
	if got.Reachability != "substitution" || got.Fairness != "fair" || got.Audited != "2026-07-17" {
		t.Errorf("readTags = %+v", got)
	}

	// A file with empty fields still yields explicit unaudited values.
	write(t, dir, "tags.json", `{"source":"placeholder"}`)
	got = readTags(dir)
	if got.Reachability != TagUnaudited || got.Fairness != TagUnaudited {
		t.Errorf("blank-field tags.json = %+v, want unaudited defaults", got)
	}
}

// A generator writes explicit unaudited tags into a fresh task dir but never
// clobbers a recorded audit on re-generation.
func TestWriteDefaultTags(t *testing.T) {
	dir := t.TempDir()
	if err := writeDefaultTags(dir); err != nil {
		t.Fatal(err)
	}
	if got := readTags(dir); got.Fairness != TagUnaudited {
		t.Errorf("fresh default tags = %+v", got)
	}

	write(t, dir, "tags.json", `{"reachability":"invention","fairness":"frontier-hard"}`)
	if err := writeDefaultTags(dir); err != nil {
		t.Fatal(err)
	}
	if got := readTags(dir); got.Fairness != "frontier-hard" {
		t.Errorf("re-generation clobbered a recorded audit: %+v", got)
	}
}

// Tags join per-scenario cells by scenario identity; a run whose scenario is
// gone from disk joins as unaudited rather than blank.
func TestAttachTags(t *testing.T) {
	cells := []ScenarioStats{
		{Scenario: "easy", Tool: "tomo"},
		{Scenario: "easy", Tool: "codex"},
		{Scenario: "gone", Tool: "tomo"},
	}
	attachTags(cells, map[string]Tags{
		"easy": {Reachability: "substitution", Fairness: "fair"},
	})
	if cells[0].Fairness != "fair" || cells[1].Fairness != "fair" {
		t.Errorf("tagged scenario did not join: %+v %+v", cells[0], cells[1])
	}
	if cells[2].Reachability != TagUnaudited || cells[2].Fairness != TagUnaudited {
		t.Errorf("deleted scenario should join as unaudited: %+v", cells[2])
	}
}

// A cap is named at write time: the wall clock, an upstream that starved the
// run, or a burned turn budget. A pass is never capped, whatever the clock did.
func TestStopReason(t *testing.T) {
	if got := stopReason(&Result{Passed: true, ExitCode: 124}, 12, 900); got != "" {
		t.Errorf("a pass is never capped, got %q", got)
	}
	if got := stopReason(&Result{ExitCode: 124}, 12, 900); got != "timeout" {
		t.Errorf("exit 124 = %q, want timeout", got)
	}
	starved := &Result{RateLimit: &RateLimit{Hits: 3}}
	if got := stopReason(starved, 12, 900); got != "rate-limit" {
		t.Errorf("429s with zero tokens = %q, want rate-limit", got)
	}
	quota := &Result{RateLimit: &RateLimit{Hits: 1, QuotaHits: 1}}
	if got := stopReason(quota, 12, 900); got != "quota" {
		t.Errorf("exhausted model balance = %q, want quota", got)
	}
	backoff := &Result{
		Tokens:    Tokens{Total: 500},
		RateLimit: &RateLimit{Hits: 1, MaxRetryAfterS: 54000},
	}
	if got := stopReason(backoff, 12, 900); got != "rate-limit" {
		t.Errorf("retry-after beyond the attempt ceiling = %q, want rate-limit", got)
	}
	turns := &Result{Orchestration: Orchestration{ModelCalls: 12}, Tokens: Tokens{Total: 9000}}
	if got := stopReason(turns, 12, 900); got != "turns" {
		t.Errorf("burned turn budget = %q, want turns", got)
	}
	if got := stopReason(&Result{Tokens: Tokens{Total: 100}}, 12, 900); got != "" {
		t.Errorf("a plain graded fail = %q, want empty", got)
	}
}

// The report reads the recorded verdict and, for rows that predate the field,
// derives only the unambiguous halves.
func TestStopOf(t *testing.T) {
	if got := stopOf(&Result{Stop: "turns"}); got != "turns" {
		t.Errorf("recorded stop = %q, want turns", got)
	}
	if got := stopOf(&Result{ExitCode: 124}); got != "timeout" {
		t.Errorf("historical exit 124 = %q, want timeout", got)
	}
	if got := stopOf(&Result{RateLimit: &RateLimit{Hits: 2}}); got != "rate-limit" {
		t.Errorf("historical starved run = %q, want rate-limit", got)
	}
	// A historical fail with tokens and no verdict stays a graded fail; the turn
	// budget it ran under is unknown, so no turns verdict is invented.
	if got := stopOf(&Result{Tokens: Tokens{Total: 900}, Orchestration: Orchestration{ModelCalls: 40}}); got != "" {
		t.Errorf("historical fail with tokens = %q, want empty", got)
	}
}

func TestCapCell(t *testing.T) {
	if got := capCell(nil); got != "-" {
		t.Errorf("no caps = %q, want -", got)
	}
	if got := capCell(map[string]int{"rate-limit": 2}); got != "2 (429)" {
		t.Errorf("one kind = %q", got)
	}
	if got := capCell(map[string]int{"rate-limit": 2, "timeout": 1}); got != "3 (2 429, 1 timeout)" {
		t.Errorf("mixed kinds = %q", got)
	}
}

// A capped attempt stays out of the graded n and out of every median, and is
// counted where the reader can see it; run health counts every run either way.
func TestSummarizeCapped(t *testing.T) {
	results := []*Result{
		{Tool: "tomo", Scenario: "hard", Passed: true, Attempts: 1, Tokens: Tokens{Prompt: 100, Completion: 10}},
		{Tool: "tomo", Scenario: "hard", ExitCode: 124, Tokens: Tokens{Prompt: 900000, Completion: 90000, Total: 990000}},
		{Tool: "tomo", Scenario: "hard", Stop: "rate-limit", RateLimit: &RateLimit{Hits: 9}},
	}
	sums := summarize(results)
	if len(sums) != 1 {
		t.Fatalf("want one summary, got %d", len(sums))
	}
	s := sums[0]
	if s.Runs != 1 || s.Passed != 1 {
		t.Errorf("graded n = %d passed = %d, want 1/1", s.Runs, s.Passed)
	}
	if s.CapKinds["timeout"] != 1 || s.CapKinds["rate-limit"] != 1 {
		t.Errorf("cap kinds = %v", s.CapKinds)
	}
	if s.RateLimitRuns != 1 {
		t.Errorf("rate-limit health count = %d, want 1", s.RateLimitRuns)
	}
	if s.Fresh.Median != 110 {
		t.Errorf("fresh median = %d, want 110: the runaway's tokens must not pollute the graded median", s.Fresh.Median)
	}

	cells := summarizeScenarios(results)
	if len(cells) != 1 {
		t.Fatalf("want one cell, got %d", len(cells))
	}
	if cells[0].Runs != 1 || cells[0].CapKinds["timeout"] != 1 {
		t.Errorf("cell n = %d caps = %v", cells[0].Runs, cells[0].CapKinds)
	}
}
