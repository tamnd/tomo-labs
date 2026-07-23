package result

// Result is one tool's outcome on one scenario, written as result.json at the
// run root and read back by the report. The JSON shape is stable: other tools
// and the docs read these files, so field names and nesting are part of the
// contract.
type Result struct {
	Tool     string `json:"tool"`
	Scenario string `json:"scenario"`
	Time     string `json:"timestamp"`
	Model    string `json:"model"`
	Runtime  string `json:"runtime"`

	Passed   bool `json:"passed"`
	ExitCode int  `json:"exit_code"`
	// Attempts is how many tries the tool took; AttemptsMax is the cap it ran
	// under. Attempts > 1 marks a scenario that only passed on a retry, which is
	// the harness's honest signal of run-to-run flakiness.
	Attempts    int `json:"attempts"`
	AttemptsMax int `json:"attempts_max"`

	WallSeconds  int    `json:"wall_seconds"`
	ElapsedClock string `json:"elapsed_clock"`

	MaxRSSKB int `json:"max_rss_kb"`
	Requests int `json:"requests"`

	Tokens  Tokens  `json:"tokens"`
	Latency Latency `json:"latency_ms"`

	// Orchestration is how the tool went about the task, read back from its own
	// captured calls: how many model round-trips it took, how many tool calls it
	// made, and whether it planned or spawned subagents to get there.
	Orchestration Orchestration `json:"orchestration"`

	// CostUSD is the dollar cost the provider billed for the run, summed over
	// every completion. It is omitted when the provider does not report a cost,
	// so a zero here means unknown, not free.
	CostUSD float64 `json:"cost_usd,omitempty"`

	DiskBeforeKB int `json:"disk_before_kb"`
	DiskAfterKB  int `json:"disk_after_kb"`
	DiskDeltaKB  int `json:"disk_delta_kb"`

	// InstallKB is the tool's own bytes on top of the shared base image, the
	// honest size axis. Whole-image size is dropped on purpose: it is dominated
	// by the base every tool shares, so it measures the base, not the tool.
	InstallKB int `json:"install_kb"`

	// RateLimit is set only when the upstream throttled the run, so its presence
	// marks a result whose slowness or failure was the free tier rejecting calls,
	// not the agent. It is omitted on a run that hit no rate limit.
	RateLimit *RateLimit `json:"rate_limit,omitempty"`

	// StreamFail is set only when the upstream dropped a completion mid-stream, so
	// its presence marks a result whose failure or retry the gateway caused, not the
	// agent. It is omitted on a run whose streams all completed cleanly.
	StreamFail *StreamFail `json:"stream_fail,omitempty"`

	// Stop names the cap that ended a failed attempt when the tool did not stop
	// on its own: "timeout" when the wall-clock ceiling killed the container,
	// "turns" when the run burned its whole turn budget without passing, or
	// "rate-limit" when the upstream starved the run (every call rejected, or a
	// back-off longer than the attempt is allowed to live). Empty means the tool
	// ended its own turn. A fail with a Stop is a capped fail, not a graded one,
	// and the report keeps the two apart per the measurement law.
	Stop string `json:"stop,omitempty"`

	Check string `json:"check"`

	// EditedTests names the hidden test files the tool changed in the work tree
	// before grading, detected by the checker and reset before the hidden patch
	// applies. It is observability, not a penalty: the grade already resets these
	// files so a tool cannot pass by rewriting them, and this field just records
	// which tools leaned on the tests instead of fixing the source. Omitted when
	// the tool touched no test file, so its presence alone is the signal.
	EditedTests []string `json:"edited_tests,omitempty"`

	// Ungraded marks a run with no checker, which is what an ad-hoc prompt run
	// (lab -p) produces: there is no pass or fail, only the answer and the metrics.
	// The scenario report skips these so a prompt run never counts as a failure.
	Ungraded bool `json:"ungraded,omitempty"`

	// Answer is the tool's final stdout, captured only for an ungraded prompt run
	// so the comparison can show what each tool actually produced. It is trimmed to
	// a sane length; the full stream stays in the trace.
	Answer string `json:"answer,omitempty"`
}

// Tokens is the model's token accounting summed over a run's completions. Cached
// and CacheWrite are the prompt tokens the provider served from, or wrote to, a
// prompt cache; both are omitted when the provider does not report caching, so a
// zero means unreported rather than none.
type Tokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
	Cached     int `json:"cached,omitempty"`
	CacheWrite int `json:"cache_write,omitempty"`
	// Reasoning is the subset of output tokens consumed by hidden reasoning.
	// It remains included in Completion and Total, matching provider accounting.
	Reasoning int `json:"reasoning,omitempty"`
}

// Latency is the model-call latency over a run, in milliseconds. AvgTTFB and
// AvgTotal are per-call averages over Calls model round-trips; SumTotal is the
// wall time the run spent waiting on the model, the model half of the run's total
// time, with the rest (tool execution and agent glue) being wall minus this.
type Latency struct {
	AvgTTFB  int `json:"avg_ttfb"`
	AvgTotal int `json:"avg_total"`
	SumTotal int `json:"sum_total"`
	Calls    int `json:"calls"`
}

// RateLimit summarizes upstream capacity rejections recovered from the proxy's
// latency log. Hits includes HTTP 429s and explicit model-account quota errors;
// QuotaHits keeps the latter distinct. MaxRetryAfterS is the longest back-off a
// 429 requested. These responses leave no answer, so without this a starved run
// looks like a plain capability failure.
type RateLimit struct {
	Hits           int `json:"hits"`
	QuotaHits      int `json:"quota_hits,omitempty"`
	MaxRetryAfterS int `json:"max_retry_after_s,omitempty"`
}

// StreamFail summarizes the completions the upstream returned as HTTP 200 and
// then dropped mid-stream, recovered from the proxy's latency log. A dropped
// stream leaves no usage and can leave the agent's work half-done, so without
// this a gateway fault looks like the agent failing the task. Calls is how many
// model calls in the recorded (winning or last) attempt broke this way, and
// Sample is the upstream's error text when it sent one. RetriedAttempts is how
// many whole attempts the harness threw out and re-ran because a dropped stream,
// not the agent, sank them; those retries do not count against the tool.
type StreamFail struct {
	Calls           int    `json:"calls,omitempty"`
	RetriedAttempts int    `json:"retried_attempts,omitempty"`
	Sample          string `json:"sample,omitempty"`
}

// Orchestration is what a run reveals about how the tool approached the task,
// recovered from the calls it actually made rather than from what it says it
// did. ModelCalls is the number of model round-trips, the honest turn count.
// ToolCalls is every tool the agent invoked. PlanCalls counts writes to a
// planning primitive (a todo or plan list, a plan-mode toggle); Subagents counts
// delegations to a child agent. Planned is true when the tool did either, so a
// tool that judged the task simple and ran a flat loop reads as unplanned, which
// is a real difference between approaches rather than a missing measurement.
type Orchestration struct {
	ModelCalls int  `json:"model_calls"`
	ToolCalls  int  `json:"tool_calls"`
	PlanCalls  int  `json:"plan_calls"`
	Subagents  int  `json:"subagents"`
	Planned    bool `json:"planned"`
}
