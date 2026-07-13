package inspect

import (
	"regexp"
	"strings"
)

// This file holds the shell-command reading that tells a clean run from the two
// failure shapes the swebench runs keep landing in. A wrong fix edits, but never
// the code the bug lives in; a runaway investigates round after round and never
// edits at all. Neither is visible from the plain read/search/edit tallies, so
// the marks are read straight off the commands: how the agent edited (a write
// tool, or a patch smuggled through the shell), whether it went digging in git
// history for the answer, and whether tomo's own convergence guard had to step in.

// gitSub returns the git subcommand a shell command runs, or "" if it is not git.
// It unwraps a leading `cd … &&` the way firstWord does, then skips the option
// flags a caller puts before the verb, so `git -C repo log` reads as "log".
func gitSub(cmd string) string {
	c := strings.TrimSpace(strings.ToLower(cmd))
	for strings.HasPrefix(c, "cd ") {
		i := strings.Index(c, "&&")
		if i < 0 {
			break
		}
		c = strings.TrimSpace(c[i+2:])
	}
	f := strings.Fields(c)
	for i := 0; i < len(f); i++ {
		if f[i] != "git" {
			continue
		}
		for j := i + 1; j < len(f); j++ {
			switch f[j] {
			case "-c", "-C": // these take an argument, so skip the pair
				j++
			default:
				if strings.HasPrefix(f[j], "-") {
					continue
				}
				return f[j]
			}
		}
	}
	return ""
}

// hexRe matches a git object id, used to tell a diff that names a commit (reading
// history) from a diff of the work tree (reviewing your own edits).
var hexRe = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)

// cmdSepRe splits a compound shell command into its segments, so a chain like
// `git status && git log … && git fsck --unreachable` is read as the three
// commands it is rather than the one the first word names. The `||` alternative
// comes before the single `|` so a logical-or is not split as two pipes.
var cmdSepRe = regexp.MustCompile(`&&|\|\||;|\n|\|`)

// splitCmds breaks a compound command into its segments.
func splitCmds(cmd string) []string { return cmdSepRe.Split(cmd, -1) }

// isHistorySeg reports whether one command segment reads git history. fsck and
// show-ref are here alongside log and show because `git fsck --unreachable` and
// `git show-ref` are the moves for digging a pruned fix commit back out, which is
// the archaeology this task's closed door is meant to deny.
func isHistorySeg(seg string) bool {
	switch gitSub(seg) {
	case "log", "show", "blame", "reflog", "rev-list", "whatchanged", "shortlog", "cat-file", "bisect", "fsck", "show-ref":
		return true
	case "diff":
		// A diff of the work tree is the agent reviewing its own edit; a diff that
		// names a commit is reading the past. Only the second is archaeology.
		c := strings.ToLower(seg)
		return strings.Contains(c, "..") || strings.Contains(c, "head~") || strings.Contains(c, "head^") || hexRe.MatchString(c)
	}
	return false
}

// isHistoryCmd reports whether a shell command reads git history in any of its
// segments: the archaeology move an agent reaches for when it would rather find
// the fix in a past commit than derive it from the code. One look is cheap and
// human; the runaway shape is doing it round after round, so the summary counts
// every command that does it.
func isHistoryCmd(cmd string) bool {
	for _, seg := range splitCmds(cmd) {
		if isHistorySeg(seg) {
			return true
		}
	}
	return false
}

// isHistoryProbe reports whether a history read is the answer-shortcut instinct:
// grepping every ref for the issue or PR number, or raking the unreachable
// objects for the pruned fix commit, rather than reading one commit on its
// merits. This is exactly what the swebench setup.sh prune denies, so a run that
// leans on it is spending moves on a door that is shut.
func isHistoryProbe(cmd string) bool {
	for _, seg := range splitCmds(cmd) {
		if !isHistorySeg(seg) {
			continue
		}
		c := strings.ToLower(seg)
		if strings.Contains(c, "--grep") || strings.Contains(c, "--all") || strings.Contains(c, "--unreachable") {
			return true
		}
	}
	return false
}

// shellRedirRe matches a redirect into a file with a source or config extension,
// so `cat > loaders/__init__.py <<EOF` reads as an edit while `2>&1` and
// `> /dev/null` do not. The extension gate keeps a log redirect out of the count.
var shellRedirRe = regexp.MustCompile(`>>?\s*['"]?[^\s|&;'"]+\.(py|go|js|jsx|ts|tsx|mjs|cjs|rb|java|c|h|cc|cpp|cxx|rs|php|cs|kt|swift|scala|toml|ini|cfg|conf|json|yaml|yml)\b`)

// isShellEdit reports whether a shell command changed a file, the way an agent
// with no write tool (or one that prefers the shell) makes its edits. These are
// invisible to a write-tool count, so a run that edited only through the shell
// would otherwise read as if it never touched the code.
func isShellEdit(cmd string) bool {
	c := strings.ToLower(cmd)
	if containsAny(c, "apply_patch", "git apply", "sed -i", "perl -i", "perl -pi") {
		return true
	}
	if t := strings.TrimSpace(c); strings.HasPrefix(t, "patch ") || strings.HasPrefix(t, "tee ") {
		return true
	}
	if containsAny(c, "| patch", "| tee ") {
		return true
	}
	return shellRedirRe.MatchString(c)
}

// guardNudge names the tomo convergence-guard nudge a step carries, or "" if it
// carries none. The guard injects one of four fixed texts when a run stalls,
// stops editing, churns, or sprawls; matching the distinctive phrase of each lets
// the summary report which guard fired without any hook into tomo itself. The
// phrases appear only in tomo runs, so this is a no-op for the other tools.
func guardNudge(text string) string {
	switch {
	case strings.Contains(text, "repeating tool calls you already made"):
		return "stall"
	case strings.Contains(text, "taken many steps without editing any file"):
		return "no-edit"
	case strings.Contains(text, "edited many times without the task converging"):
		return "churn"
	case strings.Contains(text, "edited many different files this turn"):
		return "sprawl"
	}
	return ""
}
