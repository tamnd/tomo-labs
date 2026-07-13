// Package tools holds each rival's own reading of its transcript: the call
// lexicon that buckets its moves and the notes function that reads the habits
// specific to how that agent works. Each file registers one tool from init(), so
// adding a tool is dropping a file here, never editing the inspect package. The
// lab package blank-imports this package to populate the registry.
package tools

import (
	"fmt"

	"github.com/tamnd/tomo-labs/pkg/lab/inspect"
)

func init() {
	// tomo is kept in sync with its shipped inspect.json, folding its historical
	// call names (read_file, shell, write_file, edit_file) in with its current ones
	// (read, bash, write, edit), so an older run still reads correctly.
	inspect.Register("tomo", inspect.ToolReader{
		Profile: inspect.ToolProfile{Lexicon: map[string]string{
			"read": "read", "read_file": "read",
			"grep": "search", "glob": "search", "ls": "search",
			"edit": "edit", "edit_file": "edit", "write": "edit", "write_file": "edit",
			"bash": "shell", "shell": "shell",
			"plan": "plan", "fetch": "other", "time": "other",
		}},
		Notes: tomoNotes,
	})
}

// tomoNotes reads a tomo run for the habits tomo is specifically tuned to avoid:
// re-reading a file it just wrote (redundant, since it already holds the content),
// and re-running a bash command whose output it already has (the spinning the
// loop governor exists to bound). Catching these here turns a passing-but-wasteful
// run into something visibly worth tightening, even when the token count alone
// looks fine.
func tomoNotes(prof inspect.ToolProfile, steps []inspect.Step) []string {
	var notes []string
	wrote := map[string]bool{}
	ranBash := map[string]int{}
	readAfterWrite := 0
	for _, st := range steps {
		if st.Kind != "call" {
			continue
		}
		switch prof.Classify(st.Name) {
		case "edit":
			if f := inspect.ArgPath(st.Text); f != "" {
				wrote[f] = true
			}
		case "read":
			if f := inspect.ArgPath(st.Text); f != "" && wrote[f] {
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
		notes = append(notes, fmt.Sprintf("re-read a file it had just written %s; the content was already in hand", inspect.CountNoun(readAfterWrite, "time", "times")))
	}
	if repeatBash > 0 {
		notes = append(notes, fmt.Sprintf("re-ran an identical shell command %s; the output was already in the transcript", inspect.CountNoun(repeatBash, "time", "times")))
	}
	return notes
}
