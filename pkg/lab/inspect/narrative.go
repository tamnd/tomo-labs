package inspect

import (
	"fmt"
	"io"
	"strings"
)

// Narrative turns a summary into a few plain sentences a person can read at a
// glance: the verdict and cost first, then what the agent spent its moves on,
// then the notable good and bad signs, then any throttle as the last word.
func Narrative(t *Transcript) []string {
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

	// Leaving the repo is the move worth naming on a task graded only on local
	// files, so it gets its own line with the hosts, not a slot in the did-list.
	if s.Fetches > 0 {
		line := fmt.Sprintf("It went to the network %s", countNoun(s.Fetches, "time", "times"))
		if len(s.FetchHosts) > 0 {
			line += " (" + joinList(s.FetchHosts) + ")"
		}
		lines = append(lines, line+".")
	}

	// Edits made through the shell are real fixes a write-tool count would miss, so
	// name how the agent edited when it went that way.
	if s.ShellEdits > 0 {
		lines = append(lines, fmt.Sprintf("%s of its edits went through the shell (a patch or a redirect) rather than a write tool.", countNoun(s.ShellEdits, "edit", "edits")))
	}
	// The two failure shapes worth calling out at a glance: a run that changed
	// nothing, and a long stretch of digging without touching the code.
	if s.ZeroEdits {
		line := "It changed no file at all; the run was pure investigation and can only fail."
		if s.NoEditStreak > 0 {
			line = fmt.Sprintf("It changed no file at all across %s; the run was pure investigation and can only fail.", countNoun(s.NoEditStreak, "call", "calls"))
		}
		lines = append(lines, line)
	} else if s.NoEditStreak >= 12 {
		lines = append(lines, fmt.Sprintf("Its longest stretch without changing a file was %s, a sign it dug well past the point of deciding.", countNoun(s.NoEditStreak, "call", "calls")))
	}
	// Reading git history is fine once and a runaway when repeated, and grepping
	// every ref for the issue is the answer-shortcut the pruned door denies.
	if s.HistoryReads > 0 {
		line := fmt.Sprintf("It read git history %s", countNoun(s.HistoryReads, "time", "times"))
		if s.HistoryProbes > 0 {
			line += fmt.Sprintf(" (%s grepping every ref for the issue)", countNoun(s.HistoryProbes, "of them", "of them"))
		}
		if s.HistoryReads >= 6 {
			line += ", the archaeology runaway"
		} else {
			line += ", the reflexive first look"
		}
		lines = append(lines, line+".")
	}
	// The convergence guard stepping in is tomo's own signal that the run was going
	// wrong; report which of its four nudges fired.
	if len(s.GuardNudges) > 0 {
		lines = append(lines, fmt.Sprintf("The convergence guard fired: %s.", joinList(s.GuardNudges)))
	}
	// Signs, good and bad, that a plain reader would want flagged.
	if len(s.TestEdits) > 0 {
		lines = append(lines, fmt.Sprintf("It edited %s (%s), which the grader resets before grading, so that change does not count.",
			countNoun(len(s.TestEdits), "test file", "test files"), strings.Join(s.TestEdits, ", ")))
	}
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
	// The throttle is a property of the run's environment, not the agent, so it is
	// the last word: a run the upstream cut short is a floor on what the tool would
	// have done, not a ceiling, and a reader should weigh the verdict with that.
	if t.Throttle != nil && t.Throttle.Hits > 0 {
		line := fmt.Sprintf("The upstream rate-limited it %s and the run was cut short, so read the verdict as a floor, not the tool's best.", countNoun(t.Throttle.Hits, "time", "times"))
		if t.Throttle.MaxRetryAfterS > 0 {
			line = fmt.Sprintf("The upstream rate-limited it %s (longest back-off %ds) and the run was cut short, so read the verdict as a floor, not the tool's best.",
				countNoun(t.Throttle.Hits, "time", "times"), t.Throttle.MaxRetryAfterS)
		}
		lines = append(lines, line)
	}
	return lines
}

// WriteTranscript renders a run as a person would want to read it: the verdict
// and cost, a plain-language summary of how it went, then a step-by-step
// walkthrough of how the agent actually solved the task, grouped into the phases
// a fix moves through, with full stops clipping the long arguments and outputs.
func WriteTranscript(w io.Writer, t *Transcript, full bool) {
	if t.Summary == nil {
		t.Summary = Analyze(t.Tool, BuiltinProfile(t.Tool), t.Steps)
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
	for _, line := range Narrative(t) {
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
	seen := map[string]bool{}
	for i, s := range steps {
		// A harness event is something the tool did to the model, not the model to
		// the repo: the convergence guard injecting a nudge is the one that matters,
		// and it rides on a user or system step the walkthrough used to drop. Surface
		// it inline so the step-by-step shows the harness stepping in, right where it
		// happened, rather than only in the summary.
		if s.Kind == "system" || s.Kind == "user" {
			if lbl := guardNudge(s.Text); lbl != "" {
				if ph := phaseAt(i); ph != cur {
					cur = ph
					fmt.Fprintf(w, "  %s\n", ph)
				}
				fmt.Fprintf(w, "    ⚑ harness: %s guard nudged the model\n", lbl)
			}
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
		// A call: summarize the move, tag what kind of move it really was, then the
		// outcome from the next result step. The tags are what a reader would
		// otherwise have to squint at the command to see: an edit smuggled through
		// the shell, a dig in git history, a reach for the network, a verbatim repeat.
		step++
		key := s.Name + s.Text
		line := moveLine(s)
		if tags := moveTags(s, seen[key]); tags != "" {
			line += "  " + tags
		}
		seen[key] = true
		fmt.Fprintf(w, "    %2d. %s\n", step, clipLine(line, full, 200))
		if i+1 < len(steps) && steps[i+1].Kind == "result" {
			out := steps[i+1].Text
			mark := ""
			if resultIsError(out) {
				mark = " [error]"
			}
			fmt.Fprintf(w, "        → %s%s\n", clipLine(resultLine(out), full, 160), mark)
		}
	}
}

// moveLine turns a tool call into a short human sentence: the verb the action
// bucket implies, and the file or the command it acted on, so a reader sees what
// the agent did without decoding JSON arguments.
func moveLine(s Step) string {
	// A network move is named for where it went, whichever bucket it fell in: a web
	// tool reads as "other" and a curl reads as "shell", but both are fetches, and
	// the host is the useful thing to show.
	if isNetworkTool(s.Name) || (s.Act == "shell" && isNetworkCmd(argField(s.Text, "command", "cmd", "script"))) {
		if u := fetchURL(s); u != "" {
			return "fetched " + u
		}
		return "went to the network"
	}
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
		// The multi-agent verbs are shared across harnesses, so name them plainly
		// rather than dumping their JSON arguments into the walkthrough.
		switch s.Name {
		case "Agent", "Task":
			return "dispatched a subagent"
		case "SendMessage":
			return "sent a subagent instructions"
		case "TaskOutput":
			return "collected a subagent's output"
		}
		if s.Name != "" {
			return s.Name
		}
		return strings.TrimSpace(s.Text)
	}
}

// moveTags names, in a bracketed shorthand, what a call really was beyond its
// action bucket: the marks that separate a productive move from a runaway one.
// repeated is whether this exact call was seen earlier, which the walkthrough
// tracks as it goes. An empty string means the move needs no flag.
func moveTags(s Step, repeated bool) string {
	var tags []string
	cmd := argField(s.Text, "command", "cmd", "script")
	switch {
	case isHistoryProbe(cmd):
		tags = append(tags, "history-probe") // grepping every ref for the pruned fix
	case isHistoryCmd(cmd):
		tags = append(tags, "git-history") // reading the past, fine once
	}
	if s.Act == "edit" && isShellEdit(cmd) {
		tags = append(tags, "shell-edit") // a fix made through the shell, not a write tool
	}
	if isNetworkTool(s.Name) || (s.Act == "shell" && isNetworkCmd(cmd)) {
		tags = append(tags, "network") // left the repo, denied on a closed-door task
	}
	if isInstall(s.Text) {
		tags = append(tags, "setup") // bootstrapping the env the prepared image should cover
	}
	if repeated {
		tags = append(tags, "repeat") // a verbatim rerun, a sign of spinning
	}
	if len(tags) == 0 {
		return ""
	}
	return "[" + strings.Join(tags, ", ") + "]"
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

// clipLine collapses whitespace and, unless full is set, clips to n runes so the
// walkthrough stays scannable.
func clipLine(s string, full bool, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if !full && len(s) > n {
		return s[:n] + " …"
	}
	return s
}
