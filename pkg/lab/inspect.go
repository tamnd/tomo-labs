package lab

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// A result table says a run passed and cost N tokens, but not what the agent
// actually did to get there: which files it read, what it ran, where it went in
// circles. That story lives in the request tap, but reading raw requests.jsonl by
// hand is tedious. Inspect reconstructs the run and, more than dumping it, reads
// it: it classifies every move the agent made and writes a plain-language summary
// of how the run went, then the step-by-step transcript beneath it.
//
// It works the same for every tool because the proxy normalizes all traffic to
// chat-completions: whatever dialect an agent speaks, its history lands as a
// messages array. The fullest request a run made carries the whole conversation
// up to that point, system prompt through the last tool result, so one record is
// the entire action sequence in order.

// Transcript is one run's actions, reconstructed from its request tap, plus the
// verdict and cost read back from its result and a read of how the run went.
type Transcript struct {
	Tool     string      `json:"tool"`
	Scenario string      `json:"scenario"`
	Run      string      `json:"run"` // the run's timestamp
	Passed   *bool       `json:"passed,omitempty"`
	Requests int         `json:"requests,omitempty"`
	Tokens   int         `json:"tokens,omitempty"`
	Wall     int         `json:"wall_seconds,omitempty"`
	Check    string      `json:"check,omitempty"`
	Summary  *RunSummary `json:"summary"`
	Steps    []Step      `json:"steps"`
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
	Reads     int      `json:"reads"`
	Searches  int      `json:"searches"`
	Edits     int      `json:"edits"`
	Shells    int      `json:"shells"`
	Plans     int      `json:"plans"`
	Other     int      `json:"other"`
	Thoughts  int      `json:"thoughts"`
	FilesRead []string `json:"files_read,omitempty"`
	FilesEdit []string `json:"files_edited,omitempty"`
	Repeated  int      `json:"repeated_calls"`  // calls that repeated an earlier call verbatim
	Installs  int      `json:"install_rounds"`  // shell commands that install dependencies
	Errors    int      `json:"error_results"`   // tool results that carried an error
	Verified  bool     `json:"verified"`        // ran a test or a syntax check after editing
	Notes     []string `json:"notes,omitempty"` // tool-specific observations about how the run went
}

// Inspect finds the newest run for a tool (optionally narrowed to one scenario)
// and returns its transcript, summary, and verdict. Scenario is empty to take the
// newest run the tool has across all scenarios.
func (l *Lab) Inspect(tool, scenario string) (*Transcript, error) {
	if tool == "" {
		return nil, fmt.Errorf("usage: lab inspect <tool> [scenario] [--full] [--json]")
	}
	base := filepath.Join(l.resultsDir(), tool)
	if _, err := os.Stat(base); err != nil {
		return nil, fmt.Errorf("no runs for %q: run `lab run %s` first", tool, tool)
	}
	scenarios := []string{scenario}
	if scenario == "" {
		names, err := subdirs(base)
		if err != nil {
			return nil, err
		}
		scenarios = names
	}

	// Timestamps sort the same lexically as chronologically, so the largest ts
	// with a real request trace is the newest run worth showing.
	var bestReqs, bestScenario, bestTS string
	for _, sc := range scenarios {
		stamps, err := subdirs(filepath.Join(base, sc))
		if err != nil {
			continue
		}
		for _, ts := range stamps {
			reqs := traceRequestFiles(filepath.Join(base, sc, ts))
			if len(reqs) == 0 {
				continue
			}
			if ts > bestTS {
				bestTS, bestScenario, bestReqs = ts, sc, reqs[len(reqs)-1]
			}
		}
	}
	if bestReqs == "" {
		return nil, fmt.Errorf("no captured requests for %q yet", tool)
	}

	steps, err := transcribe(bestReqs)
	if err != nil {
		return nil, err
	}
	t := &Transcript{Tool: tool, Scenario: bestScenario, Run: bestTS, Steps: steps, Summary: analyze(tool, l.loadProfile(tool), steps)}
	readVerdict(t, filepath.Join(base, bestScenario, bestTS, "result.json"))
	return t, nil
}

// readVerdict folds the run's own result.json (verdict, cost, checker note) onto
// the transcript so the summary can lead with what the run actually cost and
// whether it passed. A missing or malformed result leaves the fields at zero: the
// transcript still stands on its own.
func readVerdict(t *Transcript, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var r Result
	if json.Unmarshal(b, &r) != nil {
		return
	}
	passed := r.Passed
	t.Passed = &passed
	t.Requests = r.Requests
	t.Tokens = r.Tokens.Total
	t.Wall = r.WallSeconds
	t.Check = r.Check
}

// transcribe reads one request tap and walks the fullest conversation it holds
// into an ordered list of steps. The fullest request is the one with the most
// messages: an agent resends its growing history every call, so the last full
// call subsumes all the earlier ones.
func transcribe(reqFile string) ([]Step, error) {
	data, err := os.ReadFile(reqFile)
	if err != nil {
		return nil, err
	}
	type message struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls []struct {
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	var fullest []message
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Body struct {
				Messages []message `json:"messages"`
			} `json:"body"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if len(rec.Body.Messages) > len(fullest) {
			fullest = rec.Body.Messages
		}
	}
	if len(fullest) == 0 {
		return nil, fmt.Errorf("no messages captured in %s", reqFile)
	}

	var steps []Step
	for _, m := range fullest {
		switch m.Role {
		case "system":
			steps = append(steps, Step{Kind: "system", Text: contentText(m.Content)})
		case "user":
			steps = append(steps, Step{Kind: "user", Text: contentText(m.Content)})
		case "assistant":
			if t := contentText(m.Content); t != "" {
				steps = append(steps, Step{Kind: "assistant", Text: t})
			}
			for _, tc := range m.ToolCalls {
				steps = append(steps, Step{Kind: "call", Name: tc.Function.Name, Text: strings.TrimSpace(tc.Function.Arguments)})
			}
		case "tool":
			steps = append(steps, Step{Kind: "result", Text: contentText(m.Content)})
		}
	}
	return steps, nil
}

// analyze reads the step list into a summary: what kind of move each call was,
// which files were touched, and the marks a run leaves when it repeats itself,
// bootstraps its environment, hits errors, or checks its own work. Every judgment
// is keyword-based on the call name and arguments, which keeps it tool-agnostic:
// one tool's `read` and another's `view_file` land in the same bucket.
func analyze(tool string, prof toolProfile, steps []Step) *RunSummary {
	s := &RunSummary{}
	seen := map[string]bool{}
	edited := false
	for i := range steps {
		st := &steps[i]
		switch st.Kind {
		case "assistant":
			s.Thoughts++
			continue
		case "result":
			if resultIsError(st.Text) {
				s.Errors++
			}
			continue
		case "call":
			// handled below
		default:
			continue
		}
		st.Act = prof.classify(st.Name)
		switch st.Act {
		case "read":
			s.Reads++
			if f := argPath(st.Text); f != "" {
				s.FilesRead = appendUnique(s.FilesRead, f)
			}
		case "search":
			s.Searches++
		case "edit":
			s.Edits++
			edited = true
			if f := argPath(st.Text); f != "" {
				s.FilesEdit = appendUnique(s.FilesEdit, f)
			}
		case "shell":
			// A tool with no dedicated search runs its greps and finds through the
			// shell; count those as the searches they are, and keep only the real
			// commands as shell, so a bash-only agent's summary is not one big
			// undifferentiated pile of shell calls.
			if cmd := argField(st.Text, "command", "cmd", "script"); isSearchCmd(cmd) {
				st.Act = "search"
				s.Searches++
				break
			}
			s.Shells++
			if isInstall(st.Text) {
				s.Installs++
			}
			if edited && isVerify(st.Text) {
				s.Verified = true
			}
		case "plan":
			s.Plans++
		default:
			s.Other++
		}
		if seen[st.Name+st.Text] {
			s.Repeated++
		}
		seen[st.Name+st.Text] = true
	}
	// A tool that ships a deeper read of its own behavior gets the last word.
	if notes := toolNotes[tool]; notes != nil {
		s.Notes = notes(prof, steps)
	}
	return s
}

// toolProfile is how a tool tells the inspector to read its transcript. The
// lexicon maps the tool's exact call names to action buckets (read, search, edit,
// shell, plan, other), which is more faithful than guessing from the name. A tool
// ships one as tools/<tool>/inspect.json; loadProfile falls back to a built-in
// default so a tool with no file still reads sensibly.
type toolProfile struct {
	Lexicon map[string]string `json:"lexicon"`
}

// classify buckets one call name: the tool's own lexicon first, then the verb
// keywords agents tend to share, so a name the lexicon does not list (or an
// untuned tool) still lands somewhere reasonable.
func (p toolProfile) classify(name string) string {
	if b, ok := p.Lexicon[name]; ok {
		return b
	}
	n := strings.ToLower(name)
	switch {
	case containsAny(n, "read", "view", "open", "cat", "show", "fetch_file"):
		return "read"
	case containsAny(n, "grep", "search", "find", "glob", "ls", "list"):
		return "search"
	case containsAny(n, "edit", "write", "patch", "apply", "replace", "create", "insert", "modify"):
		return "edit"
	case containsAny(n, "bash", "shell", "exec", "run", "command", "terminal", "sh"):
		return "shell"
	case containsAny(n, "plan", "todo", "task"):
		return "plan"
	default:
		return "other"
	}
}

// loadProfile reads a tool's inspect.json profile if it ships one, else falls
// back to a built-in default. The file lives beside the tool's Dockerfile and
// adapter, so a tool's behavioral vocabulary is tuned where the tool is defined.
func (l *Lab) loadProfile(tool string) toolProfile {
	path := filepath.Join(l.cfg.Root, "tools", tool, "inspect.json")
	if b, err := os.ReadFile(path); err == nil {
		var p toolProfile
		if json.Unmarshal(b, &p) == nil && len(p.Lexicon) > 0 {
			return p
		}
	}
	if p, ok := builtinProfiles[tool]; ok {
		return p
	}
	return toolProfile{}
}

// builtinProfiles are the fallbacks compiled in, so inspect works in a checkout
// that has no tools/<tool>/inspect.json yet. tomo is kept here in sync with its
// shipped file, folding its historical call names (read_file, shell, write_file,
// edit_file) in with its current ones (read, bash, write, edit).
var builtinProfiles = map[string]toolProfile{
	"tomo": {Lexicon: map[string]string{
		"read": "read", "read_file": "read",
		"grep": "search", "glob": "search", "ls": "search",
		"edit": "edit", "edit_file": "edit", "write": "edit", "write_file": "edit",
		"bash": "shell", "shell": "shell",
		"plan": "plan", "fetch": "other", "time": "other",
	}},
	// pi ships just three tools: read, edit, and bash. It has no dedicated search,
	// so everything it looks up runs through bash; analyze reclassifies those
	// commands into searches, and piNotes reads what the shell was really doing.
	"pi": {Lexicon: map[string]string{
		"read": "read",
		"edit": "edit", "write": "edit",
		"bash": "shell",
	}},
}

// toolNotes holds each tool's deeper read of its own transcript: the anti-patterns
// and habits that are specific to how that agent works and would be lost in the
// generic counts. tomo is analyzed first and most closely, since it is the tool
// under study; other tools can register their own reading as they are tuned.
var toolNotes = map[string]func(toolProfile, []Step) []string{
	"tomo": tomoNotes,
	"pi":   piNotes,
}

// tomoNotes reads a tomo run for the habits tomo is specifically tuned to avoid:
// re-reading a file it just wrote (redundant, since it already holds the content),
// and re-running a bash command whose output it already has (the spinning the
// loop governor exists to bound). Catching these here turns a passing-but-wasteful
// run into something visibly worth tightening, even when the token count alone
// looks fine.
func tomoNotes(prof toolProfile, steps []Step) []string {
	var notes []string
	wrote := map[string]bool{}
	ranBash := map[string]int{}
	readAfterWrite := 0
	for _, st := range steps {
		if st.Kind != "call" {
			continue
		}
		switch prof.classify(st.Name) {
		case "edit":
			if f := argPath(st.Text); f != "" {
				wrote[f] = true
			}
		case "read":
			if f := argPath(st.Text); f != "" && wrote[f] {
				readAfterWrite++
			}
		case "shell":
			ranBash[st.Text]++
		}
	}
	repeatBash := 0
	for _, n := range ranBash {
		if n > 1 {
			repeatBash += n - 1
		}
	}
	if readAfterWrite > 0 {
		notes = append(notes, fmt.Sprintf("re-read a file it had just written %s; the content was already in hand", countNoun(readAfterWrite, "time", "times")))
	}
	if repeatBash > 0 {
		notes = append(notes, fmt.Sprintf("re-ran an identical shell command %s; the output was already in the transcript", countNoun(repeatBash, "time", "times")))
	}
	return notes
}

// piNotes reads a pi run for what its bash-centric style hides. pi drives every
// lookup through the shell, so analyze has already split its bash calls into the
// greps and finds (searches) and the rest (real shell); this counts how much of
// pi's work was searching-by-shell, and flags the same waste tomo is judged on so
// the two read on the same terms: re-reading a file it just edited, and re-running
// a shell command whose output it already had.
func piNotes(prof toolProfile, steps []Step) []string {
	var notes []string
	shellSearch := 0
	ranShell := map[string]int{}
	wrote := map[string]bool{}
	readAfterWrite := 0
	for _, st := range steps {
		if st.Kind != "call" {
			continue
		}
		switch st.Act {
		case "search":
			if st.Name == "bash" {
				shellSearch++
				ranShell[st.Text]++
			}
		case "shell":
			ranShell[st.Text]++
		case "edit":
			if f := argPath(st.Text); f != "" {
				wrote[f] = true
			}
		case "read":
			if f := argPath(st.Text); f != "" && wrote[f] {
				readAfterWrite++
			}
		}
	}
	repeatShell := 0
	for _, n := range ranShell {
		if n > 1 {
			repeatShell += n - 1
		}
	}
	if shellSearch > 0 {
		notes = append(notes, fmt.Sprintf("did its searching through the shell (%s), since it ships no dedicated search tool", countNoun(shellSearch, "command", "commands")))
	}
	if readAfterWrite > 0 {
		notes = append(notes, fmt.Sprintf("re-read a file it had just edited %s; the content was already in hand", countNoun(readAfterWrite, "time", "times")))
	}
	if repeatShell > 0 {
		notes = append(notes, fmt.Sprintf("re-ran an identical shell command %s; the output was already in the transcript", countNoun(repeatShell, "time", "times")))
	}
	return notes
}

// argPath pulls a file path out of a call's JSON arguments, trying the field
// names different tools use, so the summary can name the files a run touched.
func argPath(args string) string {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return ""
	}
	for _, k := range []string{"path", "file", "file_path", "filename", "filepath", "target_file"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// isInstall reports whether a shell command is bootstrapping the environment
// rather than doing the task, so a run that pays that tax is called out. With the
// prepared env in place this should be rare, and a spike is worth seeing.
func isInstall(cmd string) bool {
	return containsAny(strings.ToLower(cmd),
		"pip install", "pip3 install", "uv pip", "apt-get install", "apt install",
		"npm install", "npm i ", "yarn add", "poetry install", "conda install",
		"break-system-packages")
}

// isVerify reports whether a command is the agent checking its own work: running
// the tests, or at least confirming the edited file still parses.
func isVerify(cmd string) bool {
	return containsAny(strings.ToLower(cmd),
		"pytest", "unittest", "python -m test", "go test", "npm test", "npm run test",
		"ast.parse", "py_compile", "tox", "make test", "./run_tests")
}

// isSearchCmd reports whether a shell command is really a search: a grep, find,
// or listing whose point is to locate code, not to change or test it. A bash-only
// agent has no other way to search, so recognizing these keeps its summary honest
// about how much of its work was orienting itself. It strips leading `cd … &&`
// prefixes and matches on the first real command word.
func isSearchCmd(cmd string) bool {
	c := strings.TrimSpace(strings.ToLower(cmd))
	for strings.HasPrefix(c, "cd ") {
		i := strings.Index(c, "&&")
		if i < 0 {
			break
		}
		c = strings.TrimSpace(c[i+2:])
	}
	tok := c
	if i := strings.IndexAny(c, " \t"); i >= 0 {
		tok = c[:i]
	}
	switch tok {
	case "grep", "egrep", "fgrep", "rg", "ag", "ack", "find", "fd", "ls", "locate", "tree", "glob":
		return true
	}
	return false
}

// narrative turns a summary into a few plain sentences a person can read at a
// glance: the verdict and cost first, then what the agent spent its moves on,
// then the notable good and bad signs.
func narrative(t *Transcript) []string {
	s := t.Summary
	var lines []string

	verdict := "ran"
	if t.Passed != nil {
		if *t.Passed {
			verdict = "solved"
		} else {
			verdict = "did not solve"
		}
	}
	head := fmt.Sprintf("%s %s %s", t.Tool, verdict, t.Scenario)
	if t.Requests > 0 {
		head += fmt.Sprintf(" in %d requests", t.Requests)
	}
	if t.Tokens > 0 {
		head += fmt.Sprintf(" and %s tokens", comma(t.Tokens))
	}
	lines = append(lines, head+".")

	var did []string
	if s.Reads > 0 {
		did = append(did, fmt.Sprintf("read %s", countNoun(s.Reads, "file", "files")))
	}
	if s.Searches > 0 {
		did = append(did, fmt.Sprintf("searched %s", countNoun(s.Searches, "time", "times")))
	}
	if s.Edits > 0 {
		e := fmt.Sprintf("made %s", countNoun(s.Edits, "edit", "edits"))
		if len(s.FilesEdit) > 0 {
			e += " to " + strings.Join(s.FilesEdit, ", ")
		}
		did = append(did, e)
	}
	if s.Shells > 0 {
		did = append(did, fmt.Sprintf("ran %s", countNoun(s.Shells, "shell command", "shell commands")))
	}
	if s.Plans > 0 {
		did = append(did, fmt.Sprintf("wrote %s", countNoun(s.Plans, "plan", "plans")))
	}
	if s.Other > 0 {
		did = append(did, fmt.Sprintf("made %s", countNoun(s.Other, "other call", "other calls")))
	}
	if len(did) == 0 {
		did = append(did, "made no structured tool calls")
	}
	lines = append(lines, "It "+joinList(did)+".")

	// Signs, good and bad, that a plain reader would want flagged.
	if s.Installs > 0 {
		lines = append(lines, fmt.Sprintf("It spent %s bootstrapping the environment; the prepared env should have covered this.", countNoun(s.Installs, "command", "commands")))
	}
	if s.Repeated > 0 {
		lines = append(lines, fmt.Sprintf("%s repeated an earlier call verbatim, a sign of spinning.", countNoun(s.Repeated, "call", "calls")))
	}
	if s.Errors > 0 {
		lines = append(lines, fmt.Sprintf("It hit %s along the way.", countNoun(s.Errors, "tool error", "tool errors")))
	}
	if s.Edits > 0 {
		if s.Verified {
			lines = append(lines, "It checked its own work before finishing.")
		} else {
			lines = append(lines, "It finished without running a test or a syntax check on the edit.")
		}
	}
	if s.Installs == 0 && s.Repeated == 0 {
		lines = append(lines, "No wasted repetition and no time lost to setup.")
	}
	return lines
}

// WriteTranscript renders a run as a person would want to read it: the verdict
// and cost, a plain-language summary of how it went, then a step-by-step
// walkthrough of how the agent actually solved the task, grouped into the phases
// a fix moves through. full stops clipping the long arguments and outputs.
func WriteTranscript(w io.Writer, t *Transcript, full bool) {
	if t.Summary == nil {
		t.Summary = analyze(t.Tool, builtinProfiles[t.Tool], t.Steps)
	}
	fmt.Fprintf(w, "%s  %s  run %s\n", t.Tool, t.Scenario, t.Run)
	if t.Passed != nil {
		verdict := "PASS"
		if !*t.Passed {
			verdict = "FAIL"
		}
		fmt.Fprintf(w, "%s  %d requests  %s tokens  %ds\n", verdict, t.Requests, comma(t.Tokens), t.Wall)
		if t.Check != "" {
			fmt.Fprintf(w, "check: %s\n", t.Check)
		}
	}
	fmt.Fprintln(w, "\nSummary")
	for _, line := range narrative(t) {
		fmt.Fprintf(w, "  %s\n", line)
	}
	// The per-tool notes are this tool's own reading of the run: surface them under
	// the generic narrative, prefixed with the tool name so it is clear whose habit
	// is being called out.
	for _, note := range t.Summary.Notes {
		fmt.Fprintf(w, "  %s %s.\n", t.Tool, note)
	}

	writeWalkthrough(w, t.Steps, full)
}

// writeWalkthrough tells the story of the run in order, under the phases a fix
// moves through: everything before the first edit is the agent orienting itself,
// the edits are the fix, and what follows is it checking the fix. A run that never
// edits is shown as one plain sequence. Each line is one move summarized to a
// short human sentence, with the agent's own reasoning kept as the connective
// tissue between actions.
func writeWalkthrough(w io.Writer, steps []Step, full bool) {
	firstEdit, lastEdit := -1, -1
	for i, s := range steps {
		if s.Kind == "call" && s.Act == "edit" {
			if firstEdit < 0 {
				firstEdit = i
			}
			lastEdit = i
		}
	}
	phaseAt := func(i int) string {
		switch {
		case firstEdit < 0:
			return "Work"
		case i < firstEdit:
			return "Investigate"
		case i <= lastEdit:
			return "Fix"
		default:
			return "Verify"
		}
	}

	fmt.Fprintln(w, "\nWalkthrough")
	step := 0
	cur := ""
	for i, s := range steps {
		if s.Kind == "system" || s.Kind == "user" {
			continue
		}
		if s.Kind == "result" {
			continue // folded into the call it answers, below
		}
		if ph := phaseAt(i); ph != cur {
			cur = ph
			fmt.Fprintf(w, "  %s\n", ph)
		}
		if s.Kind == "assistant" {
			fmt.Fprintf(w, "    %s\n", clipLine("· "+s.Text, full, 200))
			continue
		}
		// A call: summarize the move, then the outcome from the next result step.
		step++
		fmt.Fprintf(w, "    %2d. %s\n", step, clipLine(moveLine(s), full, 200))
		if i+1 < len(steps) && steps[i+1].Kind == "result" {
			fmt.Fprintf(w, "        → %s\n", clipLine(resultLine(steps[i+1].Text), full, 160))
		}
	}
}

// moveLine turns a tool call into a short human sentence: the verb the action
// bucket implies, and the file or the command it acted on, so a reader sees what
// the agent did without decoding JSON arguments.
func moveLine(s Step) string {
	switch s.Act {
	case "read":
		if f := argPath(s.Text); f != "" {
			return "read " + f
		}
		return "read a file"
	case "search":
		if q := argField(s.Text, "pattern", "query", "regex"); q != "" {
			return "searched for " + q
		}
		// A search that ran through the shell carries the command, not a pattern.
		if c := argField(s.Text, "command", "cmd", "script"); c != "" {
			return "searched: " + c
		}
		return "searched the tree"
	case "edit":
		if f := argPath(s.Text); f != "" {
			return "edited " + f
		}
		return "edited a file"
	case "shell":
		if c := argField(s.Text, "command", "cmd", "script"); c != "" {
			return "ran " + c
		}
		return "ran a shell command"
	case "plan":
		return "wrote a plan"
	default:
		return s.Name + " " + s.Text
	}
}

// resultLine reduces a tool result to its first meaningful line, since the point
// in a walkthrough is what came back, not the whole payload.
func resultLine(out string) string {
	for _, ln := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(ln); s != "" {
			return s
		}
	}
	return strings.TrimSpace(out)
}

// argField pulls the first present string field out of a call's JSON arguments,
// used to name the pattern a search ran or the command a shell executed.
func argField(args string, keys ...string) string {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// clipLine collapses whitespace and, unless full is set, clips to n runes so the
// walkthrough stays scannable.
func clipLine(s string, full bool, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if !full && len(s) > n {
		return s[:n] + " …"
	}
	return s
}

// resultIsError reports whether a tool result carried a failure the agent had to
// work around, so a run that fought errors reads differently from one that did
// not. It looks for the markers tools and languages print on failure.
func resultIsError(out string) bool {
	return containsAny(out, "Traceback (most recent call last)", "fatal:", "ERROR:", "command not found", "No such file", "Error:")
}

// -- small text helpers, kept local so the summary reads cleanly ----------------

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func appendUnique(xs []string, x string) []string {
	for _, e := range xs {
		if e == x {
			return xs
		}
	}
	return append(xs, x)
}

func countNoun(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// joinList renders a list as prose: "a", "a and b", "a, b, and c".
func joinList(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	case 2:
		return xs[0] + " and " + xs[1]
	default:
		return strings.Join(xs[:len(xs)-1], ", ") + ", and " + xs[len(xs)-1]
	}
}

// comma groups an integer with thousands separators for readability.
func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return "-" + comma(-n)
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
