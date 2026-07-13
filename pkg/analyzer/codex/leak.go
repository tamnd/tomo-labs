package codex

import (
	"regexp"
	"strings"
)

// A SWE-bench-style task is only a fair measure of capability if the run solved
// it from the buggy code in front of it, not by reaching the fix from somewhere
// the checker cannot see. There are two such doors, and a real gpt-5.x run has
// been observed taking each:
//
//   - the network door: with an open network the tool fetches the upstream pull
//     request that fixed the bug (gh pr diff, a curl of the .diff, the API).
//   - the git-history door: if the checkout still carries the commits that came
//     after the base, the tool finds the gold commit in local history and diffs
//     or extracts it (git log --all --grep, git show <gold>, git archive <gold>,
//     a compare against origin/master), then reproduces it. No network needed.
//
// setup.sh now strips the future history, so the second door is closed for new
// runs, and the internal-network isolation closes the first for the containerised
// tools. LeakScan still checks both, because it audits past runs too and confirms
// on each new run that neither door was taken. It reads the run's own shell
// commands, so every finding names the exact command and can be checked by hand.

// prLike matches a reference to a specific pull request: a "pull/1225", a
// "1225.diff"/"1225.patch", or a "gh pr diff|view|checkout 1225". The number is
// captured so a report can say which PR was reached.
var prLike = regexp.MustCompile(`(?:pull/|gh\s+pr\s+(?:diff|view|checkout|list)\s+#?|/)(\d{2,6})(?:\.diff|\.patch)?\b`)

// networkFetchVerbs mark a command that reaches the network. A bare mention of a
// PR number in a local command is not one of these, so it is left alone.
var networkFetchVerbs = []string{"gh pr ", "gh api", "curl", "wget", "git fetch", "git clone", "git ls-remote", "api.github.com", "githubusercontent", "raw.github"}

// githubRe marks a command that names GitHub, so a network fetch has to aim at the
// forge before it counts.
var githubRe = regexp.MustCompile(`github|\bgh\s`)

// remoteRefRe marks a git command that reaches a ref which should not exist after
// a correct prune: a remote-tracking ref or an explicit origin/ branch. In a
// pruned SWE checkout only refs/heads/__base and ancestor tags survive, so a
// command that names origin/ or refs/remotes/ is both evidence the tree was not
// pruned and evidence the tool went looking through post-base history.
var remoteRefRe = regexp.MustCompile(`\borigin/|refs/remotes/`)

// historyMineRe marks the shape of mining local history for the fix: a git log
// over all refs filtered by a grep, which is how a run hunts for the commit whose
// message names the bug or PR.
var historyMineRe = regexp.MustCompile(`git\s+log\b[^|&;]*--all\b[^|&;]*--grep`)

// LeakDoor names which door a finding took.
type LeakDoor string

const (
	DoorNetwork LeakDoor = "network" // fetched the answer over the network
	DoorHistory LeakDoor = "history" // read the answer from post-base git history
)

// LeakHit is one command that reached, or looks like it reached, the answer. It
// carries the command verbatim and which door it took, so a finding is audited
// rather than trusted.
type LeakHit struct {
	Command string   // the shell command, as the run issued it
	Door    LeakDoor // network or history
	PR      string   // the pull request number it referenced, when one was found
}

// LeakScan returns the commands in a rollout that reached for the answer through
// either door. An empty result means the run stayed inside the task; a non-empty
// result means its outcome cannot be trusted as a solve, and each hit says which
// door and shows the exact command.
func (r *Rollout) LeakScan() []LeakHit {
	var hits []LeakHit
	seen := map[string]bool{}
	for _, cmd := range r.Commands() {
		if seen[cmd] {
			continue
		}
		if h, ok := leakInCommand(cmd); ok {
			seen[cmd] = true
			hits = append(hits, h)
		}
	}
	return hits
}

// Clean reports whether a rollout took neither door.
func (r *Rollout) Clean() bool { return len(r.LeakScan()) == 0 }

// leakInCommand classifies one shell command. A network hit needs both a fetch
// verb and a GitHub mention, so a local grep for a PR number is not flagged. A
// history hit needs a git command that reaches a remote-tracking ref or mines all
// history by grep, which a correctly-pruned checkout would make impossible.
func leakInCommand(cmd string) (LeakHit, bool) {
	low := strings.ToLower(cmd)

	fetches := false
	for _, v := range networkFetchVerbs {
		if strings.Contains(low, v) {
			fetches = true
			break
		}
	}
	if fetches && githubRe.MatchString(low) {
		h := LeakHit{Command: cmd, Door: DoorNetwork}
		if m := prLike.FindStringSubmatch(cmd); m != nil {
			h.PR = m[1]
		}
		return h, true
	}

	if strings.Contains(low, "git ") && (remoteRefRe.MatchString(low) || historyMineRe.MatchString(low)) {
		h := LeakHit{Command: cmd, Door: DoorHistory}
		if m := prLike.FindStringSubmatch(cmd); m != nil {
			h.PR = m[1]
		}
		return h, true
	}

	return LeakHit{}, false
}
