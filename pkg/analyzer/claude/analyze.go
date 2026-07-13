package claude

import (
	"sort"

	"github.com/tamnd/tomo-labs/pkg/pricing"
)

// Summary is the shape of a session the lab reads at a glance: which model ran,
// how many turns and tool calls, how many were writes, what it spent across the
// cache tiers, whether it reached outside the sandbox, and what it was asked to
// do. It is derived, so it never adds information the transcript does not hold.
type Summary struct {
	SessionID string
	Version   string
	Cwd       string
	GitBranch string

	// Models is the distinct models the run used, in first-seen order. A session
	// that never switched has one entry, e.g. claude-opus-4-8.
	Models []string

	Turns     int            // assistant messages
	ToolCalls int            // every tool_use block
	Writes    int            // Write/Edit/MultiEdit/NotebookEdit calls
	Files     int            // distinct files those writes touched
	ByTool    map[string]int // tool call count keyed by tool name

	// Tokens is the session total, summed across every assistant turn, since
	// Claude reports usage per call rather than as a running total.
	Tokens Usage

	Prompt string    // the first user message, what the session was asked to do
	Leaks  []LeakHit // commands that fetched an answer from the network, empty when clean
}

// Clean reports whether the run stayed inside the sandbox: no command fetched an
// answer. A run that is not clean cannot be counted as a capability result, since
// its outcome may be a copied answer rather than a solved one.
func (s Summary) Clean() bool { return len(s.Leaks) == 0 }

// Summarize derives a Summary from a parsed session.
func (s *Session) Summarize() Summary {
	sum := Summary{
		SessionID: s.SessionID,
		Version:   s.Version,
		Cwd:       s.Cwd,
		GitBranch: s.GitBranch,
		ByTool:    map[string]int{},
		Leaks:     s.LeakFetch(),
	}
	seenModel := map[string]bool{}
	files := map[string]bool{}
	for _, m := range s.Messages {
		if m.Role == "assistant" {
			sum.Turns++
			if m.Model != "" && !seenModel[m.Model] {
				seenModel[m.Model] = true
				sum.Models = append(sum.Models, m.Model)
			}
			sum.Tokens.InputTokens += m.Usage.InputTokens
			sum.Tokens.CacheCreationTokens += m.Usage.CacheCreationTokens
			sum.Tokens.CacheReadTokens += m.Usage.CacheReadTokens
			sum.Tokens.OutputTokens += m.Usage.OutputTokens
			sum.Tokens.Ephemeral5mTokens += m.Usage.Ephemeral5mTokens
			sum.Tokens.Ephemeral1hTokens += m.Usage.Ephemeral1hTokens
		}
		for _, b := range m.Blocks {
			if b.IsToolCall() {
				sum.ToolCalls++
				sum.ByTool[b.Name]++
				if b.IsWrite() {
					sum.Writes++
					if p := b.WrittenPath(); p != "" {
						files[p] = true
					}
				}
			}
			if m.Role == "user" && b.Type == "text" && sum.Prompt == "" {
				sum.Prompt = b.Text
			}
		}
	}
	sum.Files = len(files)
	return sum
}

// ToolNames returns the tools the run called, most-used first, so a summary prints
// them in a stable, meaningful order.
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

// Cost prices the session at the rate of the model it ran, using the shared
// pricing table so a Claude run reads in the same dollars as a gpt-5.x or deepseek
// run. Claude reports the three input kinds apart already (fresh, cache read, cache
// write), so they map straight onto the disjoint pricing.Usage. ok is false when
// the table has no rate for the model, so a caller can say "tokens only" rather
// than invent a number.
func (s Summary) Cost(table pricing.Table) (rate pricing.Model, cost pricing.Cost, ok bool) {
	model := ""
	if n := len(s.Models); n > 0 {
		model = s.Models[n-1] // the last model the run used, where it settled
	}
	rate, ok = lookupModel(table, model)
	if !ok {
		return pricing.Model{}, pricing.Cost{}, false
	}
	cost = rate.Cost(pricing.Usage{
		InputTokens:       s.Tokens.InputTokens,
		CachedInputTokens: s.Tokens.CacheReadTokens,
		CacheWriteTokens:  s.Tokens.CacheCreationTokens,
		OutputTokens:      s.Tokens.OutputTokens,
	})
	return rate, cost, true
}

// lookupModel resolves a Claude model name against the pricing table, trying the
// name as reported and then with a trailing -YYYYMMDD date snapshot trimmed, since
// the CLI may report either "claude-haiku-4-5" or "claude-haiku-4-5-20251001" for
// the same rate.
func lookupModel(table pricing.Table, name string) (pricing.Model, bool) {
	if m, ok := table.Lookup(name); ok {
		return m, true
	}
	if trimmed := trimDateSuffix(name); trimmed != name {
		if m, ok := table.Lookup(trimmed); ok {
			return m, true
		}
	}
	return pricing.Model{}, false
}

// trimDateSuffix drops a trailing -YYYYMMDD from a model name, so a dated snapshot
// resolves to the same rate as its bare alias.
func trimDateSuffix(name string) string {
	if len(name) < 9 {
		return name
	}
	tail := name[len(name)-9:]
	if tail[0] != '-' {
		return name
	}
	for _, c := range tail[1:] {
		if c < '0' || c > '9' {
			return name
		}
	}
	return name[:len(name)-9]
}
