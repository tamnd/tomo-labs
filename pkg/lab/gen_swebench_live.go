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
	target := limit
	if opts.All {
		target = 1500 // enough headroom that the lite split runs out first
	}
	split := sweLiveSplit
	if opts.All {
		split = sweLiveFullSplit
	}

	// --langs narrows the pull to specific repos by short name.
	only := map[string]bool{}
	for _, r := range opts.Langs {
		only[strings.ToLower(strings.TrimSpace(r))] = true
	}

	rows, err := sweLiveFetch(ctx, split, target, only)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("no swebench-live instances matched (split %s)", split)
	}

	kept, dropped := 0, 0
	for _, row := range rows {
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
			fmt.Fprintf(os.Stderr, "  DROP %s: %v\n", name, err)
			os.RemoveAll(task)
			os.RemoveAll(oracle)
			dropped++
			continue
		}
		if ok {
			fmt.Printf("  ok   %s\n", name)
			kept++
		} else {
			fmt.Fprintf(os.Stderr, "  DROP %s: gold fix did not pass on this host\n%s\n", name, trim(out, 400))
			os.RemoveAll(task)
			os.RemoveAll(oracle)
			dropped++
		}
	}
	fmt.Printf("\nswebench-live: kept %d, dropped %d\n", kept, dropped)
	return kept, nil
}

// sweLiveFetch pages the dataset viewer and keeps the instances this tier can grade
// on the host: the ones whose test command is a plain pytest, and, when only is
// non-empty, the named repos. It stops at target keepers or when the split ends.
func sweLiveFetch(ctx context.Context, split string, target int, only map[string]bool) ([]sweLiveRow, error) {
	var out []sweLiveRow
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
			if len(only) > 0 && !only[repoShort(r.Row.Repo)] {
				continue
			}
			if !sweLivePytestRunnable(sweStrList(r.Row.TestCmds)) {
				continue
			}
			out = append(out, r.Row)
			if len(out) >= target {
				break
			}
		}
	}
	return out, nil
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
// tier uses: edit the source in place, do not touch the tests.
func sweLivePrompt(row sweLiveRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are working in the %s repository, already checked out at the commit where a bug was reported.\n", row.Repo)
	b.WriteString("Resolve the issue below by editing the project's source code in place. ")
	b.WriteString("Do not edit or add tests: a hidden test suite grades your change. ")
	b.WriteString("Make the smallest change that fixes the issue without breaking existing behavior.\n\n")
	b.WriteString("Issue:\n\n")
	b.WriteString(strings.TrimSpace(row.ProblemStatement))
	b.WriteString("\n")
	return b.String()
}

// sweLiveCheck grades a recent instance on the host. It builds a venv on a modern
// interpreter with uv, installs the checked-out project (trying the test extra
// first, then a plain editable install, then a non-editable one, so a project's
// layout does not decide the grade), applies the hidden test patch on top of the
// agent's edits, and runs the bug's test files with pytest -rA. Grading matches the
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
for spec in "-e .[test]" "-e .[tests]" "-e .[dev]" "-e ." "."; do
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

# Restore any test files the agent touched to their base state before applying
# the hidden test patch. A tool that edited a test would otherwise break the
# apply, or shift the grade, no matter how good its source fix was. The test
# patch only ever touches test files, so resetting exactly its paths never
# discards the source change. The upstream harness resets the tests the same way.
while IFS= read -r p; do
  [ -n "$p" ] && git -C "$W" checkout -- "$p" >/dev/null 2>&1
done < <(grep '^diff --git ' "$ORACLE/test.diff" | sed -E 's#^diff --git a/.* b/##')

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
