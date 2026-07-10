package lab

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

	Check string `json:"check"`

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
}

// Latency is the average model-call latency over a run, in milliseconds, with
// the number of calls the average is over.
type Latency struct {
	AvgTTFB  int `json:"avg_ttfb"`
	AvgTotal int `json:"avg_total"`
	Calls    int `json:"calls"`
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
