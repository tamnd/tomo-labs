package tools

import (
	"fmt"

	"github.com/tamnd/tomo-labs/pkg/lab/inspect"
)

func init() {
	// opencode ships a flat toolset (read, grep, glob, edit, write, bash) plus a
	// planner (todowrite) and delegation (task, skill). It has a dedicated search,
	// so its greps do not need the shell-search reclassification pi and claude-code
	// lean on. opencodeNotes reads the one failure its style is prone to: reading
	// the bug without ever editing the fix.
	inspect.Register("opencode", inspect.ToolReader{
		Profile: inspect.ToolProfile{Lexicon: map[string]string{
			"read": "read",
			"grep": "search", "glob": "search",
			"edit": "edit", "write": "edit",
			"bash":      "shell",
			"todowrite": "plan",
			"task":      "other", "skill": "other", "webfetch": "other",
		}},
		Notes: opencodeNotes,
	})
}

// opencodeNotes reads an opencode run for the failure its transcript on the
// SWE-bench tasks kept showing: it investigates thoroughly (grep, then read the
// source, then read the tests) and then never edits. A coding fix that reads the
// buggy source and stops has done the hard part of localizing the bug and skipped
// the only part that is graded. Surfacing "read the bug, never wrote the fix"
// turns a cheap-looking run (fewest tokens of the four, because it gave up early)
// into the analysis-paralysis it actually was. It also flags the softer miss of
// editing a test file when the task said to leave tests alone, and reuses the
// same re-read and repeated-command checks the other tools get.
func opencodeNotes(prof inspect.ToolProfile, steps []inspect.Step) []string {
	var notes []string
	reads, searches, edits := 0, 0, 0
	editedTest := false
	wrote := map[string]bool{}
	readAfterWrite := 0
	ranShell := map[string]int{}
	for _, st := range steps {
		if st.Kind != "call" {
			continue
		}
		switch prof.Classify(st.Name) {
		case "read":
			reads++
			if f := inspect.ArgPath(st.Text); f != "" && wrote[f] {
				readAfterWrite++
			}
		case "search":
			searches++
		case "edit":
			edits++
			if f := inspect.ArgPath(st.Text); f != "" {
				wrote[f] = true
				if inspect.IsTestPath(f) {
					editedTest = true
				}
			}
		case "shell":
			ranShell[st.Text]++
		}
	}
	// The headline: it looked but never touched. Only meaningful when it did look,
	// so a run that legitimately did nothing is not scolded for finding nothing.
	if edits == 0 && reads+searches > 0 {
		notes = append(notes, fmt.Sprintf("read %s and searched %s but never edited a file; it localized the bug and stopped short of writing the fix, so the source went in unchanged",
			inspect.CountNoun(reads, "file", "files"), inspect.CountNoun(searches, "time", "times")))
	}
	if editedTest {
		notes = append(notes, "edited a test file, which the task said to leave alone; the fix belongs in the source, not the tests")
	}
	if readAfterWrite > 0 {
		notes = append(notes, fmt.Sprintf("re-read a file it had just edited %s; the content was already in hand", inspect.CountNoun(readAfterWrite, "time", "times")))
	}
	repeatShell := 0
	for _, n := range ranShell {
		if n > 1 {
			repeatShell += n - 1
		}
	}
	if repeatShell > 0 {
		notes = append(notes, fmt.Sprintf("re-ran an identical shell command %s; the output was already in the transcript", inspect.CountNoun(repeatShell, "time", "times")))
	}
	return notes
}
