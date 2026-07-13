package codex

import "sort"

// Summary is the shape of a rollout the lab reads at a glance: which models and
// efforts ran, how many turns and tool calls, how many were writes, what the
// run spent, and how it ended. It is derived, so it never adds information the
// trace does not hold.
type Summary struct {
	SessionID  string
	CLIVersion string
	Cwd        string

	// Models is the distinct model+effort pairs the run used, in the order they
	// first appeared. A run that never switched has one entry.
	Models []ModelUse

	Turns     int            // number of turn_context records
	ToolCalls int            // every function_call and custom_tool_call
	Writes    int            // apply_patch calls, the ones that change files
	ByTool    map[string]int // tool call count keyed by tool name

	// Tokens is the session total from the last token_count event, so it is the
	// cumulative usage rather than any single turn.
	Tokens TokenUsage

	Prompt   string // the first user_message, what the session was asked to do
	Aborted  bool   // true if any turn ended in turn_aborted
	Complete bool   // true if any turn reached task_complete
	WallMs   int    // sum of per-turn durations reported by task_complete and turn_aborted
}

// ModelUse is one model+effort the run used.
type ModelUse struct {
	Model  string
	Effort string
}

// Summarize derives a Summary from a parsed rollout.
func (r *Rollout) Summarize() Summary {
	s := Summary{
		SessionID:  r.Meta.SessionID,
		CLIVersion: r.Meta.CLIVersion,
		Cwd:        r.Meta.Cwd,
		Turns:      len(r.Turns),
		ByTool:     map[string]int{},
	}

	seenModel := map[ModelUse]bool{}
	for _, t := range r.Turns {
		mu := ModelUse{Model: t.Model, Effort: t.Effort}
		if t.Model != "" && !seenModel[mu] {
			seenModel[mu] = true
			s.Models = append(s.Models, mu)
		}
	}

	for _, it := range r.Items {
		if it.IsToolCall() {
			s.ToolCalls++
			s.ByTool[it.Name]++
			if it.IsWrite() {
				s.Writes++
			}
		}
	}

	for _, e := range r.Events {
		switch e.Type {
		case "user_message":
			if s.Prompt == "" {
				s.Prompt = e.Message
			}
		case "token_count":
			// The last token_count carries the running total, so keep taking it
			// and the final one wins.
			if e.Tokens != nil {
				s.Tokens = e.Tokens.Total
			}
		case "task_complete":
			s.Complete = true
			s.WallMs += e.DurationMs
		case "turn_aborted":
			s.Aborted = true
			s.WallMs += e.DurationMs
		}
	}
	return s
}

// ToolNames returns the tools the run called, most-used first, so a summary can
// print them in a stable, meaningful order.
func (s Summary) ToolNames() []string {
	names := make([]string, 0, len(s.ByTool))
	for n := range s.ByTool {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if s.ByTool[names[i]] != s.ByTool[names[j]] {
			return s.ByTool[names[i]] > s.ByTool[names[j]]
		}
		return names[i] < names[j]
	})
	return names
}
