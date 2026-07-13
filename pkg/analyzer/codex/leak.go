package codex

import (
	"regexp"
	"strings"
)

// A SWE-bench-style task is only a fair measure of capability if the run solved
// it from the buggy code in front of it, not by reaching the fix from somewhere
// the checker cannot see. There are three such doors, and a real gpt-5.x run has
// been observed taking each:
//
//   - the network door: with an open network the tool fetches the upstream pull
//     request that fixed the bug (gh pr diff, a curl of the .diff, the API).
//   - the git-history door: if the checkout still carries the commits that came
//     after the base, the tool finds the gold commit in local history and diffs
//     or extracts it (git log --all --grep, git show <gold>, git archive <gold>,
//     a compare against origin/master), then reproduces it. No network needed.
//   - the package-cache door: the fixed release of the same project is often
//     already installed or cached on the host (pip site-packages, a uv or pip
//     archive under ~/.cache), so the tool finds that copy and diffs the buggy
//     checkout against it to lift the patch (diff work against
//     ~/.cache/uv/archive-v0/.../pkg, rg the archive for the fixed symbol). No
//     network and no git history needed. This is the door the winning offline
//     host-fs runs actually took, so it is the one that most distorts a result.
//
// setup.sh strips the future history and the container mounts /work with no .git,
// so the second door is closed for the containerised tools; the internal-network
// isolation closes the first. The third stays open whenever a run can see the
// host filesystem, which is why the offline codex-real runs leaked and the
// container runs did not. LeakScan checks all three, because it audits past runs
// too and confirms on each new run that no door was taken. It reads the run's own
// shell commands, so every finding names the exact command and can be checked by
// hand.

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

// pkgCacheRe marks a path to a cached or installed copy of the project's source
// that a checker cannot see: a uv or pip archive under ~/.cache, or an installed
// site-packages tree. The fixed release lives at one of these paths, so a command
// that names one is reaching outside the checkout.
var pkgCacheRe = regexp.MustCompile(`\.cache/(?:uv|pip)|archive-v0|site-packages|dist-packages`)

// pkgReadVerbRe marks the verbs that read or compare files. A cache path only
// leaks the answer when a run reads from it or diffs the checkout against it; a
// path that merely appears in an install log or a PYTHONPATH is not a read, so the
// verb is required to keep an editable-install run that imports from site-packages
// during pytest from being flagged.
var pkgReadVerbRe = regexp.MustCompile(`\b(?:diff|cat|sed|rg|grep|find|cp|less|head|tail|comm|vimdiff|colordiff)\b`)

// LeakDoor names which door a finding took.
type LeakDoor string

const (
	DoorNetwork LeakDoor = "network" // fetched the answer over the network
	DoorHistory LeakDoor = "history" // read the answer from post-base git history
	DoorPackage LeakDoor = "package" // diffed against a cached/installed fixed release
)

// LeakHit is one command that reached, or looks like it reached, the answer. It
// carries the command verbatim and which door it took, so a finding is audited
// rather than trusted.
type LeakHit struct {
	Command string   // the shell command, as the run issued it
	Door    LeakDoor // network, history, or package
	PR      string   // the pull request number it referenced, when one was found
}

// LeakScan returns the commands in a rollout that reached for the answer through
// any door. An empty result means the run stayed inside the task; a non-empty
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

// Clean reports whether a rollout took no door.
func (r *Rollout) Clean() bool { return len(r.LeakScan()) == 0 }

// leakInCommand classifies one shell command. A network hit needs both a fetch
// verb and a GitHub mention, so a local grep for a PR number is not flagged. A
// history hit needs a git command that reaches a remote-tracking ref or mines all
// history by grep, which a correctly-pruned checkout would make impossible. A
// package hit needs both a cache or site-packages path and a read verb aimed at
// it, so importing from site-packages during pytest is not flagged but a diff of
// the checkout against the cached fixed release is.
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

	if pkgCacheRe.MatchString(low) && pkgReadVerbRe.MatchString(low) {
		h := LeakHit{Command: cmd, Door: DoorPackage}
		if m := prLike.FindStringSubmatch(cmd); m != nil {
			h.PR = m[1]
		}
		return h, true
	}

	return LeakHit{}, false
}
