package claude

import (
	"regexp"
	"strings"
)

// A benchmark task is only fair if the run solved it from the code in front of it,
// not by fetching the answer from somewhere the harness cannot see. SWE-bench-style
// tasks are checked out at a buggy commit with the fix stripped from local history,
// but a run with network access can still reach the upstream repository and read
// the very pull request that fixed the bug. When it does, a claimed pass is not a
// solve, it is a copy, and it must not be counted as capability.
//
// LeakFetch scans the shell commands a session ran for the shapes of that fetch:
// the GitHub CLI reading a pull request, or a raw fetch of a PR's diff or patch.
// It is deliberately about the tool's own actions, read from the trace, not a
// guess: if a command in the transcript pulled a PR, the run is flagged, and a
// reader can go look at the exact command.

// prLike matches a reference to a specific pull request in a URL or gh invocation:
// a "pull/1225", a "1225.diff"/"1225.patch", or a "gh pr diff|view|checkout 1225".
// The number is captured so a report can name which PR was fetched.
var prLike = regexp.MustCompile(`(?:pull/|gh\s+pr\s+(?:diff|view|checkout|list)\s+#?|/)(\d{2,6})(?:\.diff|\.patch)?\b`)

// fetchVerb marks a command that reaches the network, so a mere mention of a PR
// number in an unrelated command (a grep for it in the local tree) is not flagged.
var fetchVerbs = []string{"gh pr ", "gh api", "curl", "wget", "git fetch", "git clone", "git ls-remote", "api.github.com", "githubusercontent", "raw.github"}

// githubHost marks a command that names GitHub, so a fetch has to be aimed at the
// forge, not at some other host, before it counts as an answer-leak.
var githubRe = regexp.MustCompile(`github|\bgh\s`)

// LeakHit is one command in a session that fetched, or looks like it fetched, an
// answer from the network. It carries the command verbatim so the finding can be
// audited rather than trusted.
type LeakHit struct {
	Command string // the shell command, as the run issued it
	PR      string // the pull request number it referenced, when one was found
}

// LeakFetch returns the commands in a session that reached out to fetch an answer,
// most importantly the upstream pull request that fixed the bug. An empty result
// means the run stayed inside the sandbox; a non-empty result means its outcome
// cannot be trusted as a solve and the commands say exactly why.
func (s *Session) LeakFetch() []LeakHit {
	var hits []LeakHit
	seen := map[string]bool{}
	for _, m := range s.Messages {
		for _, b := range m.Blocks {
			cmd := b.BashCommand()
			if cmd == "" {
				continue
			}
			if h, ok := leakInCommand(cmd); ok && !seen[cmd] {
				seen[cmd] = true
				hits = append(hits, h)
			}
		}
	}
	return hits
}

// leakInCommand reports whether one shell command fetched an answer, and the PR it
// named. A command counts only when it both reaches the network (a fetch verb) and
// aims at GitHub, so a local grep for a PR number is left alone.
func leakInCommand(cmd string) (LeakHit, bool) {
	low := strings.ToLower(cmd)
	fetches := false
	for _, v := range fetchVerbs {
		if strings.Contains(low, v) {
			fetches = true
			break
		}
	}
	if !fetches || !githubRe.MatchString(low) {
		return LeakHit{}, false
	}
	h := LeakHit{Command: cmd}
	if m := prLike.FindStringSubmatch(cmd); m != nil {
		h.PR = m[1]
	}
	return h, true
}
