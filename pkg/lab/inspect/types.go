// Package inspect reads a run's reconstructed action list into a plain-language
// account of how the run went. It is the analysis half of `lab inspect`: given
// the ordered steps of one run (built by the lab package, which owns the trace
// files), it classifies every move, adds up the summary, and renders the
// narrative and walkthrough a person reads.
//
// It deliberately depends on nothing in the lab package, so the reading of a run
// is testable on its own and a new tool is added by dropping one file into the
// tools sub-package rather than editing a switch here. The lab package glues the
// filesystem and the run's result.json onto these types; this package never
// touches disk.
package inspect

// Transcript is one run's actions plus the verdict and cost the lab package read
// back from its result, and this package's reading of how the run went.
type Transcript struct {
	Tool     string      `json:"tool"`
	Scenario string      `json:"scenario"`
	Run      string      `json:"run"` // the run's timestamp
	Passed   *bool       `json:"passed,omitempty"`
	Requests int         `json:"requests,omitempty"`
	Tokens   int         `json:"tokens,omitempty"`
	Wall     int         `json:"wall_seconds,omitempty"`
	Check    string      `json:"check,omitempty"`
	Throttle *Throttle   `json:"throttle,omitempty"` // upstream 429s the run hit, so a cut-short run is not read as a plain agent failure
	Summary  *RunSummary `json:"summary"`
	Steps    []Step      `json:"steps"`
}

// Throttle is the run's rate-limit summary, carried onto the transcript so a run
// the upstream cut short reads as a floor, not a plain agent failure. It mirrors
// the lab package's own RateLimit, kept as a separate type here so this package
// stays free of any lab dependency.
type Throttle struct {
	Hits           int `json:"hits"`
	MaxRetryAfterS int `json:"max_retry_after_s,omitempty"`
}

// Step is one turn in the transcript: a system prompt, the task, a line of the
// agent's own reasoning, a tool call, or a tool result.
type Step struct {
	Kind string `json:"kind"`           // system | user | assistant | call | result
	Name string `json:"name,omitempty"` // tool name, for a call
	Act  string `json:"act,omitempty"`  // action bucket for a call: read|search|edit|shell|plan|other
	Text string `json:"text"`           // content, arguments, or output
}

// RunSummary is what the transcript adds up to: how many moves of each kind the
// agent made, which files it touched, and the tell-tale signs of a run that went
// smoothly or fought its environment. It is derived, not measured, so it stands
// beside the hard numbers in result.json rather than replacing them.
type RunSummary struct {
	Reads      int      `json:"reads"`
	Searches   int      `json:"searches"`
	Edits      int      `json:"edits"`
	Shells     int      `json:"shells"`
	Plans      int      `json:"plans"`
	Other      int      `json:"other"`
	Thoughts   int      `json:"thoughts"`
	Fetches    int      `json:"fetches"`               // calls that went to the network (a web tool or a curl/wget in the shell)
	FetchHosts []string `json:"fetch_hosts,omitempty"` // the distinct hosts a run reached, so a run that left the repo is visible
	FilesRead  []string `json:"files_read,omitempty"`
	FilesEdit  []string `json:"files_edited,omitempty"`
	TestEdits  []string `json:"test_edits,omitempty"` // edited files that live in a test tree, split out from the source fix
	Repeated   int      `json:"repeated_calls"`       // calls that repeated an earlier call verbatim
	Installs   int      `json:"install_rounds"`       // shell commands that install dependencies
	Errors     int      `json:"error_results"`        // tool results that carried an error
	Verified   bool     `json:"verified"`             // ran a test or a syntax check after editing
	Notes      []string `json:"notes,omitempty"`      // tool-specific observations about how the run went

	// The forensic marks that separate a clean solve from the two failure shapes
	// these runs keep landing in: a wrong fix that never touches the code the bug
	// lives in, and a pure-investigation runaway that never edits at all. These are
	// counted here so the reading of a run is automatic, not hand-derived from the
	// raw trace.
	ShellEdits    int      `json:"shell_edits,omitempty"`    // edits an agent made through the shell (apply_patch, sed -i, a redirect into a source file) rather than a write tool, which a write-only count misses
	HistoryReads  int      `json:"history_reads,omitempty"`  // git commands that read past history (log, show, blame, a diff of a commit): archaeology, fine once and a runaway when repeated
	HistoryProbes int      `json:"history_probes,omitempty"` // history reads that grep every ref for the issue or PR (--grep, --all): the answer-shortcut instinct the pruned door denies
	NoEditStreak  int      `json:"no_edit_streak,omitempty"` // the longest run of consecutive calls that changed no file, i.e. the size of an investigation runaway
	ZeroEdits     bool     `json:"zero_edits,omitempty"`     // the run changed no file at all, by a write tool or the shell
	GuardNudges   []string `json:"guard_nudges,omitempty"`   // convergence-guard nudges that fired in the transcript (stall, no-edit, churn, sprawl)
}

// ToolProfile is how a tool tells the inspector to read its transcript. The
// lexicon maps the tool's exact call names to action buckets (read, search, edit,
// shell, plan, other), which is more faithful than guessing from the name. A tool
// ships one as tools/<tool>/inspect.json; the lab package falls back to the
// built-in profile a tool registers here so a tool with no file still reads well.
type ToolProfile struct {
	Lexicon map[string]string `json:"lexicon"`
}

// ToolReader is a tool's registered reading of itself: the profile that buckets
// its calls, and the notes function that reads the habits specific to how that
// agent works. A new tool registers one of these from its own file in the tools
// sub-package, so adding a tool never means editing this package.
type ToolReader struct {
	Profile ToolProfile
	Notes   func(ToolProfile, []Step) []string
}

// registry holds the tool readers registered at init time by the tools
// sub-package. It is keyed by the tool name the harness uses.
var registry = map[string]ToolReader{}

// Register records a tool's reader. Tools call it from init(), so importing the
// tools sub-package is what populates the registry.
func Register(name string, r ToolReader) { registry[name] = r }

// BuiltinProfile returns the profile a tool registered, or an empty profile if
// the tool registered none. The lab package uses it as the fallback when a tool
// ships no tools/<tool>/inspect.json on disk.
func BuiltinProfile(name string) ToolProfile { return registry[name].Profile }

// notesFor returns the tool's notes function, or nil if it registered none.
func notesFor(name string) func(ToolProfile, []Step) []string { return registry[name].Notes }
