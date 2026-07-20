package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SWE-bench-Live is the actively maintained, continuously updated cousin of
// SWE-bench: it keeps adding fresh GitHub issues drawn from recent commits and
// decontaminates them against training data, so it does not go stale the way the
// original Lite set (frozen in 2023) has. Each instance still pairs a real issue
// with the commit that fixed it and the tests that fixing it made pass, and each
// ships the exact test command and result parser the project uses, so grading does
// not have to guess how a repo runs its suite.
//
// genSWEBenchLive renders each instance into the same lab task shape the rest of
// the tiers use: setup.sh clones the repo at the base commit into the work tree,
// the prompt is the issue, and check.sh installs the project, applies the hidden
// test patch, and runs FAIL_TO_PASS plus PASS_TO_PASS. The fix, the test patch, and
// the two test lists stay under oracle/, a sibling the harness never mounts.
//
// Fidelity trade, stated plainly: upstream SWE-bench-Live grades inside a prebuilt
// per-instance image, while this tier grades on the host in a uv-built venv to stay
// inside the lab's one-tool-image-plus-host-grading model and off x86_64 emulation.
// Because these instances are recent, a plain `uv pip install -e .` on a current
// interpreter provisions far more of them than the era-frozen Lite set ever could,
// but the projects that drive their tests through poetry, hatch, tox, or a native
// toolchain do not provision under a plain venv. So this tier keeps the instances
// whose test command is a plain pytest, and, as everywhere, keeps an instance only
// if its own gold fix makes the hidden tests pass here. The rest are dropped with a
// reason, so the rendered suite is the validated, host-provisionable subset.

const (
	sweLiveDataset = "SWE-bench-Live/SWE-bench-Live"
	// The lite split is the curated, size-bounded cut of the live set, the right
	// default for a host-graded tier; --all reaches for the full split.
	sweLiveSplit     = "lite"
	sweLiveFullSplit = "full"
	// Recent instances target a modern interpreter; uv fetches it on first use.
	// A version mismatch just means the gold fix fails to import and the instance
	// is dropped, so one sane default beats a per-repo table that cannot keep up
	// with a set that grows every month.
	sweLivePython = "3.12"
	// The live set is heavily skewed: a handful of repos supply most instances
	// (conan, cfn-lint, matplotlib, haystack, and so on), so an unbounded pull
	// grades a few projects' quirks over and over instead of a broad sample. Cap
	// how many instances any one repo contributes to the default suite so the
	// rendered set stays diverse. A --langs pull that names repos on purpose is
	// exempt: there the caller wants that repo, however many it takes, and
	// --per-repo overrides the cap (1 renders a one-task-per-repo sample).
	sweLivePerRepoCap = 3
	// A validated pull drops any instance whose gold fix does not pass on this
	// host, so fetching exactly the wanted count would leave the suite short after
	// the drops. Fetch this many candidates per wanted keeper and validate in order
	// until the target is met, so gold drops are backfilled instead of shrinking
	// the suite.
	sweLiveOverfetch = 6
)

// sweLiveRow is one SWE-bench-Live record. FAIL_TO_PASS, PASS_TO_PASS, and
// test_cmds all arrive as real JSON arrays, decoded with sweStrList.
type sweLiveRow struct {
	InstanceID       string          `json:"instance_id"`
	Repo             string          `json:"repo"`
	BaseCommit       string          `json:"base_commit"`
	Patch            string          `json:"patch"`
	TestPatch        string          `json:"test_patch"`
	ProblemStatement string          `json:"problem_statement"`
	CreatedAt        string          `json:"created_at"`
	TestCmds         json.RawMessage `json:"test_cmds"`
	FailToPass       json.RawMessage `json:"FAIL_TO_PASS"`
	PassToPass       json.RawMessage `json:"PASS_TO_PASS"`
}

func (l *Lab) genSWEBenchLive(ctx context.Context, opts GenOptions) (int, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return 0, fmt.Errorf("swebench-live needs git on the host to check out each repo: %w", err)
	}
	if _, err := exec.LookPath("uv"); err != nil {
		return 0, fmt.Errorf("swebench-live grades in a per-instance venv; install uv (https://docs.astral.sh/uv): %w", err)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 6
	}
	perRepoCap := sweLivePerRepoCap
	if opts.PerRepo > 0 {
		perRepoCap = opts.PerRepo
	}

	// --langs narrows the pull to specific repos by short name.
	only := map[string]bool{}
	for _, r := range opts.Langs {
		only[strings.ToLower(strings.TrimSpace(r))] = true
	}

	// A validated pull over-fetches so gold drops can be backfilled up to the
	// wanted count; --all and --no-validate take everything they are handed. Draw
	// from the lite split first (curated), then the full split, so a repo-diverse
	// pull has a broad enough pool of repos to reach its target without repeats.
	target := limit * sweLiveOverfetch
	splits := []string{sweLiveSplit, sweLiveFullSplit}
	if opts.All {
		target = 1500 // enough headroom that the full split runs out first
		splits = []string{sweLiveFullSplit}
	}

	rows, err := sweLiveFetch(ctx, splits, target, only, perRepoCap)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("no swebench-live instances matched")
	}

	// Validate in order and stop once the suite has its wanted count of keepers.
	// Each drop is reported with why, so a repo that never provisions on this host
	// is a documented substitution (the next repo's task takes its place), not a
	// silent hole. --all keeps proving every instance it fetched.
	kept, dropped := 0, 0
	for _, row := range rows {
		if !opts.All && !opts.NoValidate && kept >= limit {
			break
		}
		task, oracle, err := l.sweLiveMaterialize(row)
		if err != nil {
			return kept, err
		}
		name := row.InstanceID
		if opts.NoValidate {
			fmt.Printf("  wrote %s\n", name)
			kept++
			continue
		}
		// Gold proof: the instance's own fix, on a clean checkout, must make the
		// hidden tests pass on this host, or the task cannot grade and is dropped.
		goldPath := filepath.Join(oracle, "gold.diff")
		ok, out, err := validateTask(ctx, task, func(work string) error {
			return gitApply(ctx, work, goldPath)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  DROP %s (%s): %v\n", name, repoShort(row.Repo), err)
			os.RemoveAll(task)
			os.RemoveAll(oracle)
			dropped++
			continue
		}
		if ok {
			fmt.Printf("  ok   %s (%s)\n", name, repoShort(row.Repo))
			kept++
		} else {
			fmt.Fprintf(os.Stderr, "  DROP %s (%s): gold fix did not pass on this host\n%s\n", name, repoShort(row.Repo), trim(out, 400))
			os.RemoveAll(task)
			os.RemoveAll(oracle)
			dropped++
		}
	}
	fmt.Printf("\nswebench-live: kept %d, dropped %d (%d distinct repos)\n", kept, dropped, l.sweLiveRepoCount())
	return kept, nil
}

// sweLiveRepoCount counts the distinct repos among the tasks currently on disk,
// so the gen summary can report the diversity it achieved.
func (l *Lab) sweLiveRepoCount() int {
	repos := map[string]bool{}
	oracleDir := filepath.Join(l.suiteDir(), "oracle")
	entries, _ := os.ReadDir(oracleDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(oracleDir, e.Name(), "repo"))
		if err == nil {
			repos[strings.TrimSpace(string(b))] = true
		}
	}
	return len(repos)
}

// sweLiveFetch pages the dataset viewer across the given splits in order and
// keeps the instances this tier can grade on the host: the ones whose test
// command is a plain pytest, and, when only is non-empty, the named repos. It
// stops at target candidates or when every split ends. Paging lite before full
// draws the curated instances first and reaches into the larger split only for
// the breadth a repo-diverse pull needs. The per-repo tally and the seen-id set
// are shared across splits, so full (a superset of lite) neither repeats an
// instance nor overshoots a repo's cap.
func sweLiveFetch(ctx context.Context, splits []string, target int, only map[string]bool, perRepoCap int) ([]sweLiveRow, error) {
	// Enforce the per-repo cap only on an untargeted pull; a --langs pull that
	// names repos wants exactly those, however many instances that takes.
	perRepo := map[string]int{}
	seen := map[string]bool{}
	capped := len(only) == 0
	var out []sweLiveRow
	for _, split := range splits {
		for offset := 0; len(out) < target; offset += swePageSize {
			q := url.Values{
				"dataset": {sweLiveDataset}, "config": {"default"}, "split": {split},
				"offset": {fmt.Sprint(offset)}, "length": {fmt.Sprint(swePageSize)},
			}
			var page struct {
				Rows []struct {
					Row sweLiveRow `json:"row"`
				} `json:"rows"`
			}
			if err := httpGetJSON(ctx, sweRowsAPI+"?"+q.Encode(), &page); err != nil {
				return nil, err
			}
			if len(page.Rows) == 0 {
				break
			}
			for _, r := range page.Rows {
				if seen[r.Row.InstanceID] {
					continue
				}
				if !sweLiveKeep(r.Row, only, perRepo, capped, perRepoCap) {
					continue
				}
				seen[r.Row.InstanceID] = true
				out = append(out, r.Row)
				if len(out) >= target {
					break
				}
			}
		}
	}
	return out, nil
}

// sweLiveKeep decides whether to keep one row, given the optional repo filter
// and the per-repo cap, and updates the running per-repo tally when it keeps
// one. It drops a row whose repo is not in a non-empty only set, whose test
// command this tier cannot run on the host, or whose repo has already hit the
// cap on a capped (untargeted) pull.
func sweLiveKeep(row sweLiveRow, only map[string]bool, perRepo map[string]int, capped bool, perRepoCap int) bool {
	short := repoShort(row.Repo)
	if len(only) > 0 && !only[short] {
		return false
	}
	if !sweLivePytestRunnable(sweStrList(row.TestCmds)) {
		return false
	}
	if capped && perRepo[short] >= perRepoCap {
		return false
	}
	perRepo[short]++
	return true
}

// sweLivePytestRunnable reports whether an instance's test command is a plain
// pytest invocation this tier can reproduce in a uv venv. Commands that route
// through a project's own environment manager (poetry, hatch, tox, nox, make) need
// that toolchain and its lockfile, which a plain venv does not reconstruct, so they
// are left for the upstream image-based harness rather than graded wrong here.
func sweLivePytestRunnable(cmds []string) bool {
	if len(cmds) != 1 {
		return false // a single test command keeps grading unambiguous
	}
	c := strings.TrimSpace(cmds[0])
	return strings.HasPrefix(c, "pytest") ||
		strings.HasPrefix(c, "python -m pytest") ||
		strings.HasPrefix(c, "python3 -m pytest")
}

// sweTestFile returns the test file a pytest node id points at: everything before
// the first "::". Live's ids are truncated at the first space inside a param, but
// the truncation lands after the "::", so the file part is always intact.
func sweTestFile(id string) string {
	if i := strings.Index(id, "::"); i >= 0 {
		return id[:i]
	}
	return id
}

// sweInFiles keeps the ids whose test file is in the given set, preserving order.
func sweInFiles(ids []string, files map[string]bool) []string {
	var out []string
	for _, id := range ids {
		if files[sweTestFile(id)] {
			out = append(out, id)
		}
	}
	return out
}

func (l *Lab) sweLiveMaterialize(row sweLiveRow) (task, oracle string, err error) {
	name := sweSlug(row.InstanceID)
	task = filepath.Join(l.tasksDir(), name)
	oracle = filepath.Join(l.suiteDir(), "oracle", name)
	os.RemoveAll(task)
	os.RemoveAll(oracle)

	// The grade runs only the test files the bug's own tests live in, then checks
	// per-test outcomes from the log. That keeps a grade to a handful of files
	// instead of a project's whole multi-thousand-test suite, and it scopes the
	// regression guard (PASS_TO_PASS) to those same files: local regressions the
	// fix could plausibly cause, not the entire repo. Files stay in first-seen
	// order so the pytest command is stable across regenerations.
	f2p := sweStrList(row.FailToPass)
	fileSet := map[string]bool{}
	var testFiles []string
	for _, id := range f2p {
		f := sweTestFile(id)
		if !fileSet[f] {
			fileSet[f] = true
			testFiles = append(testFiles, f)
		}
	}
	p2p := sweInFiles(sweStrList(row.PassToPass), fileSet)

	files := map[string]string{
		"gold.diff":        row.Patch,
		"test.diff":        row.TestPatch,
		"fail_to_pass.txt": strings.Join(f2p, "\n") + "\n",
		"pass_to_pass.txt": strings.Join(p2p, "\n") + "\n",
		"test_files.txt":   strings.Join(testFiles, "\n") + "\n",
		"repo":             row.Repo + "\n",
		"base_commit":      row.BaseCommit + "\n",
		"python":           sweLivePython + "\n",
	}
	for fn, body := range files {
		if err = writeFile(filepath.Join(oracle, fn), []byte(body), 0o644); err != nil {
			return
		}
	}

	if err = writeFile(filepath.Join(task, "prompt.txt"), []byte(sweLivePrompt(row)), 0o644); err != nil {
		return
	}
	desc := fmt.Sprintf("swebench-live: %s (%s, %s)\n", row.InstanceID, row.Repo, row.CreatedAt)
	if err = writeFile(filepath.Join(task, "desc"), []byte(desc), 0o644); err != nil {
		return
	}
	if err = writeDefaultTags(task); err != nil {
		return
	}
	// setup.sh is the dataset-agnostic clone-at-base-commit step shared with the
	// Lite tier; check.sh is the host-provisioning grader for recent projects.
	if err = writeFile(filepath.Join(task, "setup.sh"), []byte(sweSetup), 0o755); err != nil {
		return
	}
	if err = writeFile(filepath.Join(task, "check.sh"), []byte(sweLiveCheck), 0o755); err != nil {
		return
	}
	return task, oracle, nil
}

// sweLivePrompt renders the issue for the agent with the same ground rules the Lite
// tier uses: edit the source in place, do not touch the tests. It also states the
// two closed doors this harness holds shut, because a tool that is not told them
// spends its opening rounds discovering them: the network is blackholed and the git
// history is truncated to the base commit (see sweStripFuture), so a run that reaches
// for the referenced PR online or mines the log for the fix only learns, a round
// later, that both are gone. Every real subscription harness announces its sandbox
// this way; stating the same truth here levels the presentation across tools and
// keeps a run reading the code the bug points at instead of the doors. It is not a
// hint at the fix: it names what is absent, never what to change.
func sweLivePrompt(row sweLiveRow) string {
	return renderPrompt("swebench_live.md", row.Repo, row.ProblemStatement)
}

// sweLiveCheck grades a recent instance on the host. It builds a venv on a modern
// interpreter with uv, installs the checked-out project (trying a plain editable
// install first, then a non-editable one, with test extras only as fallbacks, so a
// project's optional integration stack does not decide the grade), applies the
// hidden test patch on top of the agent's edits, and runs the bug's test files with
// pytest -rA. Grading matches the
// dataset's node ids against the -rA outcome lines by prefix, because Live truncates
// a parametrized id at its first space, so the stored id is a prefix of the real
// one and only a log match can line them up. FAIL_TO_PASS must be green and the
// in-file PASS_TO_PASS must stay green. It avoids bash 4 features so it runs under
// the macOS system bash.
const sweLiveCheck = `#!/usr/bin/env bash
# Grade against the hidden SWE-bench-Live tests: build a modern venv, install the
# project, apply the test patch, run the bug's test files, and read per-test
# outcomes from the pytest -rA summary.
W="$1"
D="$(cd "$(dirname "$0")" && pwd)"
SUITE="$(cd "$D/../.." && pwd)"
NAME="$(basename "$D")"
ORACLE="$SUITE/oracle/$NAME"

VENV="$(mktemp -d)"
trap 'rm -rf "$VENV"' EXIT
PYVER="$(cat "$ORACLE/python")"
if ! uv venv --python "$PYVER" "$VENV" >/dev/null 2>&1; then
  echo "FAIL: could not build a Python $PYVER venv"; exit 1
fi
PY="$VENV/bin/python"

# Install the project. Disable globbing so the [extra] specs stay literal, and take
# the first recipe that succeeds, so a project without a test extra still installs.
set -f
installed=0
for spec in "-e ." "." "-e .[test]" "-e .[tests]" "-e .[dev]"; do
  if ( cd "$W" && uv pip install --python "$PY" -q $spec ) >/dev/null 2>&1; then
    installed=1; break
  fi
done
set +f
if [ "$installed" -ne 1 ]; then
  echo "FAIL: could not install project"; exit 1
fi
# Ensure a test runner is present without clobbering a self-provided one.
"$PY" -c "import pytest" >/dev/null 2>&1 || uv pip install --python "$PY" -q pytest >/dev/null 2>&1

TESTPATHS="$(grep '^diff --git ' "$ORACLE/test.diff" | sed -E 's#^diff --git a/.* b/##')"

# Catch, then restore. First note any hidden test file the agent changed in the
# work tree: a tool leaning on the tests instead of fixing the source. This is
# observability across every tool, printed for the harness to record on the
# result, not a penalty, because the restore right after makes the grade fair no
# matter who edited what.
edited=""
while IFS= read -r p; do
  [ -z "$p" ] && continue
  if [ -n "$(git -C "$W" status --porcelain -- "$p" 2>/dev/null)" ]; then
    edited="$edited $p"
  fi
done <<EOF
$TESTPATHS
EOF
[ -n "$edited" ] && echo "EDITED_TESTS:$edited"

# Restore any test files the agent touched to their base state before applying
# the hidden test patch. A tool that edited a test would otherwise break the
# apply, or shift the grade, no matter how good its source fix was. The test
# patch only ever touches test files, so resetting exactly its paths never
# discards the source change. The upstream harness resets the tests the same way.
while IFS= read -r p; do
  [ -n "$p" ] && git -C "$W" checkout -- "$p" >/dev/null 2>&1
done <<EOF
$TESTPATHS
EOF

if ! git -C "$W" apply "$ORACLE/test.diff" >/dev/null 2>&1; then
  echo "FAIL: test patch did not apply"; exit 1
fi

# Read the bug's test files into an array without mapfile, keeping any spaces.
FILES=()
while IFS= read -r line; do [ -n "$line" ] && FILES+=("$line"); done < "$ORACLE/test_files.txt"
if [ "${#FILES[@]}" -eq 0 ]; then
  echo "FAIL: no test files recorded"; exit 1
fi

cd "$W"
"$PY" -m pytest -rA -p no:cacheprovider "${FILES[@]}" >"$VENV/out.log" 2>&1

# Match dataset ids against the -rA outcome lines by prefix, then require every
# FAIL_TO_PASS id green and no in-file PASS_TO_PASS id regressed.
"$PY" - "$VENV/out.log" "$ORACLE/fail_to_pass.txt" "$ORACLE/pass_to_pass.txt" <<'PYEOF'
import sys
log, f2p_file, p2p_file = sys.argv[1], sys.argv[2], sys.argv[3]
outcomes = []
with open(log, errors="replace") as fh:
    for line in fh:
        line = line.rstrip("\n")
        for kind in ("PASSED", "FAILED", "ERROR"):
            if line.startswith(kind + " "):
                outcomes.append((kind, line[len(kind) + 1:]))
                break

def load(path):
    with open(path) as fh:
        return [ln.strip() for ln in fh if ln.strip()]

def kinds_for(stored):
    # A stored id is a prefix of the real (possibly space-truncated) node id.
    return [k for (k, nid) in outcomes if nid.startswith(stored)]

bad = []
for t in load(f2p_file):
    ks = kinds_for(t)
    if not ks:
        bad.append("FAIL_TO_PASS did not run: " + t)
    elif any(k != "PASSED" for k in ks):
        bad.append("FAIL_TO_PASS not green: " + t)
for t in load(p2p_file):
    ks = kinds_for(t)
    # Ignore a PASS_TO_PASS id that did not run; only a real regression fails.
    if ks and any(k != "PASSED" for k in ks):
        bad.append("PASS_TO_PASS regressed: " + t)

if bad:
    print("FAIL: hidden tests not satisfied")
    for b in bad[:8]:
        print("  " + b)
    sys.exit(1)
print("PASS: fail_to_pass green, in-file pass_to_pass stable")
PYEOF
rc=$?
if [ "$rc" -ne 0 ]; then
  tail -n 3 "$VENV/out.log" 2>/dev/null
fi
exit $rc
`
