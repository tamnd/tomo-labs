package tools

import (
	"fmt"

	"github.com/tamnd/tomo-labs/pkg/lab/inspect"
)

func init() {
	// pi ships just three tools: read, edit, and bash. It has no dedicated search,
	// so everything it looks up runs through bash; the analysis reclassifies those
	// commands into searches, and piNotes reads what the shell was really doing.
	inspect.Register("pi", inspect.ToolReader{
		Profile: inspect.ToolProfile{Lexicon: map[string]string{
			"read": "read",
			"edit": "edit", "write": "edit",
			"bash": "shell",
		}},
		Notes: piNotes,
	})
}

// piNotes reads a pi run for what its bash-centric style hides. pi drives every
// lookup through the shell, so the analysis has already split its bash calls into
// the greps and finds (searches) and the rest (real shell); this counts how much
// of pi's work was searching-by-shell, and flags the same waste tomo is judged on
// so the two read on the same terms: re-reading a file it just edited, and
// re-running a shell command whose output it already had.
func piNotes(prof inspect.ToolProfile, steps []inspect.Step) []string {
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
			if f := inspect.ArgPath(st.Text); f != "" {
				wrote[f] = true
			}
		case "read":
			if f := inspect.ArgPath(st.Text); f != "" && wrote[f] {
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
		notes = append(notes, fmt.Sprintf("did its searching through the shell (%s), since it ships no dedicated search tool", inspect.CountNoun(shellSearch, "command", "commands")))
	}
	if readAfterWrite > 0 {
		notes = append(notes, fmt.Sprintf("re-read a file it had just edited %s; the content was already in hand", inspect.CountNoun(readAfterWrite, "time", "times")))
	}
	if repeatShell > 0 {
		notes = append(notes, fmt.Sprintf("re-ran an identical shell command %s; the output was already in the transcript", inspect.CountNoun(repeatShell, "time", "times")))
	}
	return notes
}
