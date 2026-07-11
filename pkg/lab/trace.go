package lab

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// traceMetrics is what the harness pulls out of a single attempt's trace dir.
// It replaces the awk and jq the shell harness leaned on with real parsing, so
// a stray line or an empty file yields a zero rather than a silent skew.
type traceMetrics struct {
	MaxRSSKB     int
	ElapsedClock string
	Requests     int
	Tokens       Tokens
	CostUSD      float64
	Latency      Latency
	Orch         Orchestration
	RateLimit    *RateLimit
}

// readTrace parses every metric file the proxy and GNU time left in a trace dir.
// Missing or malformed files degrade to zero values, never an error, because a
// tool that crashed still deserves a graded, comparable row.
func readTrace(dir string) traceMetrics {
	m := traceMetrics{}
	m.MaxRSSKB, m.ElapsedClock = readTime(filepath.Join(dir, "time.txt"))
	m.Requests = countLines(filepath.Join(dir, "requests.jsonl"))
	m.Tokens, m.CostUSD = sumTokens(filepath.Join(dir, "usage.jsonl"))
	m.Latency = latencyStats(filepath.Join(dir, "latency.jsonl"))
	m.Orch = orchestration(filepath.Join(dir, "requests.jsonl"))
	m.RateLimit = rateLimitStats(filepath.Join(dir, "latency.jsonl"))
	return m
}

// rateLimitStats scans the latency log for upstream rate-limit rejections. The
// proxy writes one row per call with its status and, on a 429, the Retry-After it
// carried, so this counts the 429s and keeps the longest back-off asked for. It
// returns nil when the run hit no rate limit, so the result omits the field
// entirely rather than recording a zero.
func rateLimitStats(path string) *RateLimit {
	var hits, maxRA int
	forEachJSON(path, func(b []byte) {
		var r struct {
			Status     int `json:"status"`
			RetryAfter int `json:"retry_after_s"`
		}
		if json.Unmarshal(b, &r) != nil || r.Status != 429 {
			return
		}
		hits++
		if r.RetryAfter > maxRA {
			maxRA = r.RetryAfter
		}
	})
	if hits == 0 {
		return nil
	}
	return &RateLimit{Hits: hits, MaxRetryAfterS: maxRA}
}

// planTools are the names a tool calls to write down a plan: a todo or plan
// list, or a plan-mode toggle. Different agents spell it differently, so match
// the union across the wired tools (opencode's todowrite, gemini's plan-mode,
// codex's update_plan), lowercased.
var planTools = map[string]bool{
	"todowrite": true, "todo_write": true, "update_plan": true, "write_todos": true,
	"enter_plan_mode": true, "exit_plan_mode": true, "plan": true, "planning": true,
	// Names the wired tools spell their plan list with: claude-code writes a task
	// list (taskcreate/taskupdate), hermes a bare todo, others a create_plan.
	"todo": true, "taskcreate": true, "taskupdate": true, "create_plan": true, "add_task": true,
}

// subagentTools are the names a tool calls to delegate a scoped piece of work to
// a child agent, the other shape orchestration takes.
var subagentTools = map[string]bool{
	"task": true, "invoke_agent": true, "dispatch_agent": true, "agent": true, "subagent": true,
	// openclaw spawns children as subagents, hermes as delegate_task.
	"subagents": true, "delegate_task": true,
}

// orchestration recovers how a tool worked from the proxy's request tap. Every
// completion request carries the whole transcript so far, so the request with
// the most messages holds every tool call the agent made; counting names in it
// yields the plan-list writes and subagent spawns without the tool cooperating.
// ModelCalls counts the completion requests themselves, the round-trip count.
//
// Planning shows up two different ways across the wired tools, and both count.
// A tool with a plan primitive calls it as a tool (opencode's todowrite, codex's
// update_plan), which lands in the transcript's tool calls. A tool that plans in
// a dedicated pass instead makes a separate completion under a planner system
// prompt (tomo asks its planner to turn a job into steps); that call has no plan
// tool to see, so it is recognized by its system prompt instead.
func orchestration(path string) Orchestration {
	var o Orchestration
	var best []string
	bestN := -1
	forEachJSON(path, func(b []byte) {
		var rec struct {
			Method string          `json:"method"`
			Path   string          `json:"path"`
			Body   json.RawMessage `json:"body"`
		}
		if json.Unmarshal(b, &rec) != nil {
			return
		}
		if rec.Method != "POST" || !strings.Contains(rec.Path, "chat/completions") {
			return
		}
		o.ModelCalls++
		if len(rec.Body) == 0 {
			return
		}
		var body struct {
			Messages []struct {
				Role      string          `json:"role"`
				Content   json.RawMessage `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if json.Unmarshal(rec.Body, &body) != nil {
			return
		}
		// A dedicated planner pass is one completion; count each such request once.
		if len(body.Messages) > 0 && isPlannerSystem(body.Messages[0].Role, body.Messages[0].Content) {
			o.PlanCalls++
		}
		if len(body.Messages) <= bestN {
			return
		}
		bestN = len(body.Messages)
		best = best[:0]
		for _, m := range body.Messages {
			for _, tc := range m.ToolCalls {
				best = append(best, strings.ToLower(tc.Function.Name))
			}
		}
	})
	for _, name := range best {
		o.ToolCalls++
		if planTools[name] {
			o.PlanCalls++
		}
		if subagentTools[name] {
			o.Subagents++
		}
	}
	o.Planned = o.PlanCalls > 0 || o.Subagents > 0
	return o
}

// isPlannerSystem reports whether a message is a system prompt that puts the
// model in a dedicated planning role, the signature of a plan-in-a-separate-pass
// tool. It reads the word "planner" in the system content, which the plan pass
// carries and an ordinary agent turn does not.
func isPlannerSystem(role string, content json.RawMessage) bool {
	if role != "system" || len(content) == 0 {
		return false
	}
	var s string
	if json.Unmarshal(content, &s) != nil {
		s = string(content) // content may be an array of parts; scan it raw
	}
	return strings.Contains(strings.ToLower(s), "planner")
}

// readTime pulls the max resident set size (in kbytes) and the wall-clock
// elapsed string out of GNU time's -v report.
func readTime(path string) (rssKB int, elapsed string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "Maximum resident set size"):
			rssKB = lastInt(line)
		case strings.HasPrefix(line, "Elapsed (wall clock) time"):
			if i := strings.LastIndex(line, ": "); i >= 0 {
				elapsed = strings.TrimSpace(line[i+2:])
			}
		}
	}
	return rssKB, elapsed
}

// countLines counts non-empty lines, which is the request count in a jsonl tap.
func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			n++
		}
	}
	return n
}

// sumTokens adds up the usage rows the proxy recorded, one per reply, and the
// dollar cost alongside them. Cached and cache-write tokens and cost only appear
// when the provider reported them, so they stay zero for a provider that does
// not, which the report renders as blank rather than as a real zero.
func sumTokens(path string) (Tokens, float64) {
	var t Tokens
	var cost float64
	forEachJSON(path, func(b []byte) {
		var r struct {
			Prompt     int     `json:"prompt_tokens"`
			Completion int     `json:"completion_tokens"`
			Total      int     `json:"total_tokens"`
			Cached     int     `json:"cached_tokens"`
			CacheWrite int     `json:"cache_write_tokens"`
			Cost       float64 `json:"cost_usd"`
		}
		if json.Unmarshal(b, &r) == nil {
			t.Prompt += r.Prompt
			t.Completion += r.Completion
			t.Total += r.Total
			t.Cached += r.Cached
			t.CacheWrite += r.CacheWrite
			cost += r.Cost
		}
	})
	return t, cost
}

// latencyStats averages ttfb and total over the model calls the proxy timed. It
// counts only the completions endpoint with a 200, so a readiness probe or a
// rejected request never skews the numbers.
func latencyStats(path string) Latency {
	var ttfb, total, n int
	forEachJSON(path, func(b []byte) {
		var r struct {
			Status  int    `json:"status"`
			Path    string `json:"path"`
			TTFBMS  int    `json:"ttfb_ms"`
			TotalMS int    `json:"total_ms"`
		}
		if json.Unmarshal(b, &r) != nil || r.Status != 200 {
			return
		}
		if !strings.Contains(r.Path, "chat/completions") && !strings.Contains(r.Path, "/messages") {
			return
		}
		ttfb += r.TTFBMS
		total += r.TotalMS
		n++
	})
	if n == 0 {
		return Latency{}
	}
	return Latency{AvgTTFB: ttfb / n, AvgTotal: total / n, Calls: n}
}

// forEachJSON calls fn with each non-empty line of a jsonl file, skipping a
// missing file so callers stay one-liners.
func forEachJSON(path string, fn func([]byte)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		b := sc.Bytes()
		if len(strings.TrimSpace(string(b))) == 0 {
			continue
		}
		fn(b)
	}
}

// dirSizeKB is the on-disk footprint of a work tree in kbytes, summed over every
// regular file. It stands in for du -sk without shelling out.
func dirSizeKB(root string) int {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return int(total / 1024)
}

func lastInt(s string) int {
	fields := strings.Fields(s)
	for i := len(fields) - 1; i >= 0; i-- {
		if n, err := strconv.Atoi(strings.Trim(fields[i], ":")); err == nil {
			return n
		}
	}
	return 0
}
