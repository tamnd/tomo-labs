package tools

import (
	"fmt"

	"github.com/tamnd/tomo-labs/pkg/lab/inspect"
)

func init() {
	// claude-code works through subagents: it dispatches them (Agent), instructs
	// them (SendMessage), and polls their output (TaskOutput). Those land in the
	// other bucket here, and claudeNotes reads them as the coordination tax they
	// are. It searches through Bash like pi, so the shell-search reclassification
	// covers its greps too.
	inspect.Register("claude-code", inspect.ToolReader{
		Profile: inspect.ToolProfile{Lexicon: map[string]string{
			"Read": "read",
			"Grep": "search", "Glob": "search", "LS": "search",
			"Edit": "edit", "Write": "edit", "MultiEdit": "edit", "NotebookEdit": "edit",
			"Bash":      "shell",
			"TodoWrite": "plan",
			"Agent":     "other", "Task": "other", "SendMessage": "other", "TaskOutput": "other",
			"WebFetch": "other", "WebSearch": "other",
		}},
		Notes: claudeNotes,
	})
}

// claudeNotes reads a claude-code run for the habit that dominates its cost: it
// solves through subagents, so a large share of its calls are not looking at the
// code but coordinating other agents that do (Agent to dispatch, SendMessage to
// instruct, TaskOutput to poll). That orchestration is invisible in the plain
// read/edit/shell counts, so it is called out here as the tax it is, next to the
// same re-read and repeated-command checks the other tools get.
func claudeNotes(prof inspect.ToolProfile, steps []inspect.Step) []string {
	var notes []string
	orchestration, total := 0, 0
	subagentStalled := false
	wrote := map[string]bool{}
	readAfterWrite := 0
	ranShell := map[string]int{}
	for i := range steps {
		st := steps[i]
		if st.Kind != "call" {
			continue
		}
		total++
		switch st.Name {
		case "Agent", "Task", "SendMessage", "TaskOutput":
			orchestration++
			// The output a poll came back with tells whether the subagent detour
			// paid off or stalled; a timeout or failure means those coordinating
			// calls bought nothing and the agent had to fall back.
			if i+1 < len(steps) && steps[i+1].Kind == "result" {
				if inspect.ContainsAny(steps[i+1].Text, "retrieval_status>timeout", "\"timeout\"", "agent failed", "did not complete") {
					subagentStalled = true
				}
			}
		}
		switch st.Act {
		case "edit":
			if f := inspect.ArgPath(st.Text); f != "" {
				wrote[f] = true
			}
		case "read":
			if f := inspect.ArgPath(st.Text); f != "" && wrote[f] {
				readAfterWrite++
			}
		case "shell":
			ranShell[st.Text]++
		}
	}
	repeatShell := 0
	for _, n := range ranShell {
		if n > 1 {
			repeatShell += n - 1
		}
	}
	if orchestration > 0 && total > 0 {
		notes = append(notes, fmt.Sprintf("spent %s coordinating subagents (dispatch, message, and poll), %d%% of its %s",
			inspect.CountNoun(orchestration, "call", "calls"), orchestration*100/total, inspect.CountNoun(total, "tool call", "tool calls")))
	}
	if subagentStalled {
		notes = append(notes, "dispatched a subagent that timed out, then fell back to doing the work itself; the detour cost calls and bought nothing")
	}
	if readAfterWrite > 0 {
		notes = append(notes, fmt.Sprintf("re-read a file it had just edited %s; the content was already in hand", inspect.CountNoun(readAfterWrite, "time", "times")))
	}
	if repeatShell > 0 {
		notes = append(notes, fmt.Sprintf("re-ran an identical shell command %s; the output was already in the transcript", inspect.CountNoun(repeatShell, "time", "times")))
	}
	return notes
}
