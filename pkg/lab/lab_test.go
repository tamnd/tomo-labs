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

// summarize rolls per-run results into per-tool rows: pass count and the pass@1
// (first-try) count the table ranks on.
func TestSummarize(t *testing.T) {
	results := []*Result{
		{Tool: "tomo", Passed: true, Attempts: 1, AttemptsMax: 3, Tokens: Tokens{Total: 100, Cached: 20}, CostUSD: 0.01, MaxRSSKB: 1024 * 30, WallSeconds: 10, Latency: Latency{AvgTTFB: 200, Calls: 2}, InstallKB: 1024 * 40},
		{Tool: "tomo", Passed: true, Attempts: 2, AttemptsMax: 3, Tokens: Tokens{Total: 200, Cached: 30}, CostUSD: 0.02, MaxRSSKB: 1024 * 50, WallSeconds: 20, Latency: Latency{AvgTTFB: 400, Calls: 3}, InstallKB: 1024 * 40},
		{Tool: "openclaw", Passed: false, Attempts: 3, AttemptsMax: 3, Tokens: Tokens{Total: 300}, MaxRSSKB: 1024 * 200, WallSeconds: 30, Latency: Latency{}, InstallKB: 1024 * 300},
	}
	sums := summarize(results)
	if len(sums) != 2 {
		t.Fatalf("got %d tools, want 2", len(sums))
	}
	// Ranked on pass@1 first: tomo solved one task on the first attempt, openclaw
	// solved none, so tomo leads regardless of cost.
	tomo, oc := sums[0], sums[1]
	if tomo.Tool != "tomo" || oc.Tool != "openclaw" {
		t.Fatalf("order = %s,%s want tomo,openclaw", sums[0].Tool, sums[1].Tool)
	}
	if tomo.Runs != 2 || tomo.Passed != 2 {
		t.Errorf("tomo runs/passed = %d/%d, want 2/2", tomo.Runs, tomo.Passed)
	}
	if tomo.FirstTry != 1 || tomo.Retried != 1 {
		t.Errorf("tomo first_try/retried = %d/%d, want 1/1", tomo.FirstTry, tomo.Retried)
	}
	if tomo.TotalTokens != 300 {
		t.Errorf("tomo total tokens = %d, want 300", tomo.TotalTokens)
	}
	if tomo.AvgRSSMB != 40 {
		t.Errorf("tomo avg_rss_mb = %d, want 40", tomo.AvgRSSMB)
	}
	if tomo.AvgTTFBMS != 300 {
		t.Errorf("tomo avg_ttfb = %d, want 300", tomo.AvgTTFBMS)
	}
	if tomo.InstallMB != 40 {
		t.Errorf("tomo install mb = %d, want 40", tomo.InstallMB)
	}
	if tomo.CachedTokens != 50 {
		t.Errorf("tomo cached tokens = %d, want 50", tomo.CachedTokens)
	}
	if tomo.TotalCostUSD < 0.0299 || tomo.TotalCostUSD > 0.0301 {
		t.Errorf("tomo total cost = %v, want ~0.03", tomo.TotalCostUSD)
	}
	// openclaw never produced a timed completion, so its latency stays zero
	// instead of dividing by zero.
	if oc.AvgTTFBMS != 0 {
		t.Errorf("openclaw avg_ttfb = %d, want 0", oc.AvgTTFBMS)
	}
	if oc.Passed != 0 || oc.Retried != 1 || oc.FirstTry != 0 {
		t.Errorf("openclaw passed/retried/first = %d/%d/%d, want 0/1/0", oc.Passed, oc.Retried, oc.FirstTry)
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
