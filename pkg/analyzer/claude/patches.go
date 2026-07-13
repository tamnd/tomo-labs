package claude

import "encoding/json"

// Edit is one change a run made through a builtin write tool: the file it touched,
// the tool that made the change, and the payload of that change. It is the part of
// a session worth reading next to what tomo did on the same task, the way a Codex
// apply_patch is.
//
// Unlike Codex, which writes every change through apply_patch, Claude can also edit
// by shelling out (a heredoc, a git apply). Those do not show up here, since they
// carry no structured file target; a run that edits entirely through Bash yields no
// Edits, and its Bash commands are what the leak detector and a reader look at
// instead. So Edits is the structured, tool-driven subset of a run's changes.
type Edit struct {
	Tool    string // Write, Edit, MultiEdit, or NotebookEdit
	Path    string // the file the change targeted
	OldText string // Edit/MultiEdit: the text replaced (first hunk for MultiEdit)
	NewText string // Write: the full contents; Edit: the replacement text
}

// Edits returns every builtin write-tool change a run made, in order.
func (s *Session) Edits() []Edit {
	var out []Edit
	for _, m := range s.Messages {
		for _, b := range m.Blocks {
			if !b.IsWrite() {
				continue
			}
			out = append(out, editFromBlock(b))
		}
	}
	return out
}

func editFromBlock(b Block) Edit {
	e := Edit{Tool: b.Name, Path: b.WrittenPath()}
	var v struct {
		Content   string `json:"content"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
		Edits     []struct {
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		} `json:"edits"`
	}
	if json.Unmarshal(b.Input, &v) != nil {
		return e
	}
	switch b.Name {
	case "Write":
		e.NewText = v.Content
	case "Edit":
		e.OldText = v.OldString
		e.NewText = v.NewString
	case "MultiEdit":
		if len(v.Edits) > 0 {
			e.OldText = v.Edits[0].OldString
			e.NewText = v.Edits[0].NewString
		}
	}
	return e
}
