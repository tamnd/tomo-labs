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

	DiskBeforeKB int `json:"disk_before_kb"`
	DiskAfterKB  int `json:"disk_after_kb"`
	DiskDeltaKB  int `json:"disk_delta_kb"`

	InstallKB int `json:"install_kb"`
	ImageKB   int `json:"image_kb"`

	Check string `json:"check"`
}

// Tokens is the model's token accounting summed over a run's completions.
type Tokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// Latency is the average model-call latency over a run, in milliseconds, with
// the number of calls the average is over.
type Latency struct {
	AvgTTFB  int `json:"avg_ttfb"`
	AvgTotal int `json:"avg_total"`
	Calls    int `json:"calls"`
}
