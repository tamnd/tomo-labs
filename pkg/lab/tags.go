package lab

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// TagUnaudited is the explicit placeholder for a task whose audit has not run.
// It is a real value, not a blank: a task with no tags file renders as
// unaudited, and an unaudited task stays out of the honest denominator for any
// bar claim until the audit loop works through it.
const TagUnaudited = "unaudited"

// Tags is a task's adoption-time metadata, read from a tags.json beside the
// scenario. The file lives with the task rather than in a central registry, so
// a task and its verdicts move, copy, and delete together.
//
// Reachability is the gold-diff shape: "substitution" when the fix swaps or
// drops lines already there, "invention" when it introduces behavior that was
// not there before. The shape is judged from the gold diff (or the task's own
// deliverable) at adoption, never from run outcomes.
//
// Fairness is the frontier-diagnostic verdict: "fair" (reachable, stays in the
// denominator), "frontier-hard" (a frontier model fails for capability reasons
// a stronger model could overcome; stays in as a hard row), or
// "convention-locked" (frontier and cheap models fail identically because the
// fix demands a project-private convention no capability derives; out of the
// honest denominator).
type Tags struct {
	Reachability string `json:"reachability"`
	Fairness     string `json:"fairness"`
	// Source cites the evidence behind the verdicts, so a tag is auditable
	// back to the run or diff that justified it.
	Source string `json:"source,omitempty"`
	// Audited is the date the verdicts were recorded, so filed-before-run is
	// answerable per task.
	Audited string `json:"audited,omitempty"`
}

// readTags loads a scenario's tags.json. A missing or malformed file yields
// explicit unaudited tags rather than blanks, so an unaudited task can never
// render as anything else.
func readTags(dir string) Tags {
	fallback := Tags{Reachability: TagUnaudited, Fairness: TagUnaudited}
	b, err := os.ReadFile(filepath.Join(dir, "tags.json"))
	if err != nil {
		return fallback
	}
	var t Tags
	if json.Unmarshal(b, &t) != nil {
		return fallback
	}
	if t.Reachability == "" {
		t.Reachability = TagUnaudited
	}
	if t.Fairness == "" {
		t.Fairness = TagUnaudited
	}
	return t
}

// writeDefaultTags writes an explicit unaudited tags.json into a generated
// task dir unless one is already there, so a fresh generation never leaves a
// silent blank and a re-generation never clobbers a recorded audit.
func writeDefaultTags(dir string) error {
	path := filepath.Join(dir, "tags.json")
	if exists(path) {
		return nil
	}
	b, err := json.MarshalIndent(Tags{Reachability: TagUnaudited, Fairness: TagUnaudited}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
