package main

import (
	"fmt"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/analyzer/claude"
	"github.com/tamnd/tomo-labs/pkg/pricing"
)

// cmdClaude reads the session transcripts the Claude Code CLI wrote to disk under
// ~/.claude, the same way cmdCodex reads Codex rollouts. It is a tap on a strong
// model running under the user's own subscription, not a benchmark on the shared
// free model, so it needs no container and no key and dispatches before lab.New.
//
// Its point beyond token and cost accounting is fairness: a SWE-bench-style task is
// only a capability result if the run solved it from the code in front of it, not
// by fetching the upstream pull request that fixed the bug. analyze flags any such
// fetch from the trace, so a claimed pass can be told from a copied one.
//
//	lab claude sessions [cwd]                     list session files, newest first
//	lab claude analyze [session] [--patch] [--json] summarize a session (latest if omitted)
func cmdClaude(args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "sessions":
		return claudeSessions(arg(args, 1), hasFlag(args, "--json"))
	case "analyze", "":
		return claudeAnalyze(arg(args, 1), hasFlag(args, "--json"), hasFlag(args, "--patch"))
	default:
		return fmt.Errorf("usage: lab claude {sessions [cwd]|analyze [session] [--patch]} [--json]")
	}
}

// claudeSessions lists the session files under the Claude home, newest first. With
// a cwd it lists only that run directory's sessions, resolving its projects slug.
func claudeSessions(cwd string, asJSON bool) error {
	home := claude.Home()
	var (
		paths []string
		err   error
	)
	if cwd != "" {
		paths, err = claude.FindSessionsForCwd(home, cwd)
	} else {
		paths, err = claude.FindSessions(home)
	}
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(paths)
	}
	if len(paths) == 0 {
		fmt.Printf("no sessions under %s\n", claude.ProjectsDir(home))
		return nil
	}
	for _, p := range paths {
		fmt.Println(p)
	}
	return nil
}

// claudeAnalyze summarizes one session: the model that ran, the turns and tool
// calls, the token cost across the cache tiers, the dollar cost at the model's
// published rate, and, first when present, any command that fetched an answer from
// the network. A leak makes the outcome untrustworthy as a solve, so it prints
// loud and above the rest.
func claudeAnalyze(path string, asJSON, showPatch bool) error {
	if path == "" {
		p, ok, err := claude.LatestSession(claude.Home())
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no sessions under %s", claude.Home())
		}
		path = p
	}
	s, err := claude.ParseSessionFile(path)
	if err != nil {
		return err
	}
	sum := s.Summarize()
	if asJSON {
		return writeJSON(sum)
	}

	fmt.Printf("session: %s\n", path)
	fmt.Printf("id:      %s  cli %s\n", sum.SessionID, sum.Version)
	fmt.Printf("models:  %s\n", strings.Join(sum.Models, ", "))
	fmt.Printf("cwd:     %s\n", sum.Cwd)
	fmt.Printf("turns=%d tool_calls=%d writes=%d files=%d\n", sum.Turns, sum.ToolCalls, sum.Writes, sum.Files)
	if len(sum.ByTool) > 0 {
		fmt.Printf("by tool: %s\n", claudeByTool(sum))
	}

	// The fairness verdict comes first: if the run fetched the answer, its outcome
	// is not a capability result, and the reader should see that before any number.
	if !sum.Clean() {
		fmt.Printf("LEAK:    run fetched from the network, outcome is NOT a solve (%d command(s)):\n", len(sum.Leaks))
		for _, h := range sum.Leaks {
			pr := ""
			if h.PR != "" {
				pr = fmt.Sprintf("  [PR #%s]", h.PR)
			}
			fmt.Printf("  $ %s%s\n", firstLine(h.Command, 120), pr)
		}
	} else {
		fmt.Printf("fair:    no network fetch of an answer found in the trace\n")
	}

	t := sum.Tokens
	fmt.Println("tokens:")
	fmt.Printf("  input        %9d  (fresh, cache-miss)\n", t.InputTokens)
	fmt.Printf("  cache read   %9d  (served from cache, %s of input)\n", t.CacheReadTokens, hitRate(t.CacheReadTokens, t.InputTokens+t.CacheReadTokens))
	fmt.Printf("  cache write  %9d  (written into cache)\n", t.CacheCreationTokens)
	fmt.Printf("  output       %9d\n", t.OutputTokens)
	fmt.Printf("  total        %9d\n", t.InputTokens+t.CacheReadTokens+t.CacheCreationTokens+t.OutputTokens)

	printClaudeCost(sum)

	if sum.Prompt != "" {
		fmt.Printf("prompt:  %s\n", firstLine(sum.Prompt, 100))
	}

	if showPatch {
		edits := s.Edits()
		if len(edits) == 0 {
			fmt.Println("edits:   (none through builtin write tools; a Bash-only run shows none)")
		}
		for i, e := range edits {
			fmt.Printf("--- edit %d: %s %s ---\n", i+1, e.Tool, e.Path)
			if e.OldText != "" {
				fmt.Printf("- %s\n", firstLine(e.OldText, 200))
			}
			if e.NewText != "" {
				fmt.Printf("+ %s\n", firstLine(e.NewText, 200))
			}
		}
	}
	return nil
}

// printClaudeCost turns the session tokens into a dollar figure at the model's
// published rate, using the shared pricing table so a Claude run reads in the same
// dollars as a gpt-5.x or deepseek run. Claude reports the three input kinds apart
// (fresh, cache read, cache write), so Summary.Cost maps them straight onto the
// disjoint pricing.Usage with no subtraction.
func printClaudeCost(sum claude.Summary) {
	model := ""
	if n := len(sum.Models); n > 0 {
		model = sum.Models[n-1]
	}
	_, c, ok := sum.Cost(pricing.Default())
	if !ok {
		fmt.Printf("  cost       (no published rate for %q, tokens only)\n", model)
		return
	}
	fmt.Printf("cost (%s API list price, a subscription run is not metered per token):\n", model)
	fmt.Printf("  input        %s  (fresh)\n", usd(c.InputUSD))
	fmt.Printf("  cache read   %s  (billed at the discounted read rate)\n", usd(c.CachedUSD))
	fmt.Printf("  cache write  %s  (billed at the write rate)\n", usd(c.CacheWriteUSD))
	fmt.Printf("  output       %s\n", usd(c.OutputUSD))
	fmt.Printf("  total        %s\n", usd(c.TotalUSD))
}

// claudeByTool renders the tool-call histogram most-used first, matching the codex
// summary's one-line "by tool" form.
func claudeByTool(sum claude.Summary) string {
	var b strings.Builder
	for i, n := range sum.ToolNames() {
		if i > 0 {
			b.WriteString("  ")
		}
		fmt.Fprintf(&b, "%s=%d", n, sum.ByTool[n])
	}
	return b.String()
}
