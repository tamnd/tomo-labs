package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SWE-bench Lite is a held-out set of real GitHub issues, each paired with the
// commit that fixed it and the tests that fixing it made pass. A task hands the
// agent the project checked out at the buggy commit and the issue text, and asks
// for a source change that resolves it. The agent never sees the fix or the tests.
//
// genSWEBench renders each instance into the lab task shape. setup.sh clones the
// repository at the instance's base commit into the work tree, so the agent edits
// the real project in place. Grading applies the instance's hidden test patch on
// top of whatever the agent left, then runs the two named test sets: FAIL_TO_PASS,
// the tests the fix is supposed to turn green, and PASS_TO_PASS, the tests that
// must stay green so a fix does not break the rest. A task passes only if every
// test in both sets passes.
//
// The fix, the test patch, and the two test lists encode the answer, so they never
// reach the agent: they live under oracle/, a sibling of tasks/ the harness never
// mounts, and check.sh reads them from there at grading time. These instances are
// years old, and their packages neither build nor import on a current host Python,
// so check.sh builds a throwaway virtualenv per grade on the interpreter the
// instance's era targets, using uv to fetch that interpreter, installs the era's
// pinned dependencies and then the checked-out project into it, and tears it down
// after.
//
// Unlike the upstream SWE-bench harness, which grades inside a per-instance x86_64
// image, this tier grades on the host in a uv-built venv, staying inside the lab's
// one-tool-image-plus-host-grading model and off the slow x86_64 emulation an
// arm64 host would need for those images. The dependency pins and interpreter
// versions are transcribed from SWE-bench's own per-repo specs, so the environment
// matches the era rather than drifting to today's versions. It is honest about the
// cost: an instance is kept only if its own gold fix, applied to a clean checkout,
// actually makes the tests pass here. An instance whose environment does not
// provision on this host, or whose tests do not run under a plain pytest, fails
// that proof and is dropped with a reason, so the rendered suite is a validated
// subset rather than a set of tasks that cannot grade. The django and sympy
// instances run their own test runners rather than pytest, so they are skipped up
// front.

const (
	sweRowsAPI = "https://datasets-server.huggingface.co/rows"
	sweDataset = "princeton-nlp/SWE-bench_Lite"
	// The private test lists and patches are inlined per row, so a page of 100 is
	// already a few megabytes; the viewer caps a page at 100 regardless.
	swePageSize = 100
)

// sweGradeable is the set of repositories, keyed by the short name after the
// owner, whose test suites run under a plain `pytest <node id>` and whose package
// installs without a compiler on the host. The default sample draws only from
// these so a first generation does not stall building a C extension that has no
// wheel for the checked-out source. `--all` ignores the allowlist and lets the
// gold-fix proof decide, so a caller can still reach for the heavier repos.
var sweGradeable = map[string]bool{
	"requests": true,
	"flask":    true,
	"pylint":   true,
	"pytest":   true,
	"sphinx":   true,
	"xarray":   true,
}

// sweSkip is the set of repositories that do not grade under a plain
// `pytest <node id>`, so they are dropped before any network work. django ships
// its own test runner, and sympy's FAIL_TO_PASS ids are dotted module paths for
// its bin/test runner rather than pytest node ids.
var sweSkip = map[string]bool{"django": true, "sympy": true}

// sweSpec is the install recipe for one instance, transcribed from SWE-bench's
// own MAP_REPO_VERSION_TO_SPECS. pip is the set of exact dependency pins that
// belong to the instance's era; without them a plain install pulls today's
// versions and even the gold fix fails to import. install is the pip arguments
// used to install the checked-out project itself, run with the work tree as cwd.
type sweSpec struct {
	python  string   // interpreter the instance's era targets, e.g. "3.9"
	pip     []string // exact dependency pins installed before the project
	install string   // e.g. "-e .", ".", "-e .[test]"
}

// Era pins, verbatim from SWE-bench's SPECS_* tables for the repositories this
// tier grades. flask splits by version; the rest share one recipe across every
// version present in Lite.
var (
	sweFlask20 = []string{
		"setuptools==70.0.0", "Werkzeug==2.3.7", "Jinja2==3.0.1",
		"itsdangerous==2.1.2", "click==8.0.1", "MarkupSafe==2.1.3",
	}
	sweFlask2x = []string{
		"setuptools==70.0.0", "Werkzeug==2.3.7", "Jinja2==3.1.2",
		"itsdangerous==2.1.2", "click==8.1.3", "MarkupSafe==2.1.1",
	}
	sweXarray = []string{
		"numpy==1.23.0", "packaging==23.1", "pandas==1.5.3", "pytest==7.4.0",
		"python-dateutil==2.8.2", "pytz==2023.3", "six==1.16.0", "scipy==1.11.1",
		"setuptools==68.0.0", "dask==2022.8.1",
	}
)

// sweSpecFor resolves the install recipe for a repo at a version. Anything not
// covered falls back to a plain editable install, which the gold-fix proof then
// keeps or drops on its own.
func sweSpecFor(repo, version string) sweSpec {
	switch repoShort(repo) {
	case "requests":
		// Pure stdlib deps; upstream installs non-editable and adds pytest.
		return sweSpec{python: "3.9", install: "."}
	case "flask":
		if version == "2.0" {
			return sweSpec{python: "3.9", pip: sweFlask20, install: "-e ."}
		}
		return sweSpec{python: "3.11", pip: sweFlask2x, install: "-e ."}
	case "xarray":
		return sweSpec{python: "3.10", pip: sweXarray, install: "-e ."}
	case "sphinx":
		// Upstream drives the tests through tox; this tier runs the same pytest
		// node ids directly, so it installs the test extra and pins Jinja2.
		return sweSpec{python: "3.9", pip: []string{"Jinja2==3.0.3"}, install: "-e .[test]"}
	case "pytest":
		// Old pytest imports pkg_resources, which setuptools 81 dropped; hold
		// setuptools below that so the checked-out pytest still starts up.
		return sweSpec{python: "3.9", pip: []string{"setuptools<81"}, install: "-e ."}
	default: // pylint and any --all repo without an entry
		return sweSpec{python: "3.9", install: "-e ."}
	}
}

// sweRow is one dataset record. The two test lists arrive as JSON-encoded strings
// (a stringified array), so they are kept raw and decoded with sweStrList.
type sweRow struct {
	InstanceID       string          `json:"instance_id"`
	Repo             string          `json:"repo"`
	BaseCommit       string          `json:"base_commit"`
	Patch            string          `json:"patch"`
	TestPatch        string          `json:"test_patch"`
	ProblemStatement string          `json:"problem_statement"`
	Version          string          `json:"version"`
	FailToPass       json.RawMessage `json:"FAIL_TO_PASS"`
	PassToPass       json.RawMessage `json:"PASS_TO_PASS"`
}

func (l *Lab) genSWEBench(ctx context.Context, opts GenOptions) (int, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return 0, fmt.Errorf("swebench needs git on the host to check out each repo: %w", err)
	}
	if _, err := exec.LookPath("uv"); err != nil {
		return 0, fmt.Errorf("swebench grades in a per-instance venv built on the era's Python; install uv (https://docs.astral.sh/uv): %w", err)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 6
	}
	target := limit
	if opts.All {
		target = 300 // SWE-bench Lite's test split is 300 instances.
	}

	// --langs narrows the pull to specific repos by short name, e.g.
	// --langs pytest,pylint, which keeps a targeted generation off the slower repos.
	only := map[string]bool{}
	for _, r := range opts.Langs {
		only[strings.ToLower(strings.TrimSpace(r))] = true
	}

	rows, err := sweFetch(ctx, target, opts.All, only)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("no swebench instances matched (allowlist: %s)", strings.Join(sweRepoNames(), ", "))
	}

	kept, dropped := 0, 0
	for _, row := range rows {
		task, oracle, err := l.sweMaterialize(row)
		if err != nil {
			return kept, err
		}
		name := row.InstanceID
		if opts.NoValidate {
			fmt.Printf("  wrote %s\n", name)
			kept++
			continue
		}
		// Gold proof: the instance's own fix, applied to a clean checkout, must make
		// the hidden tests pass on this host. If it does not, the environment did not
		// provision or the tests do not run here, so the task cannot grade and is
		// dropped rather than shipped broken.
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
	fmt.Printf("\nswebench-lite: kept %d, dropped %d\n", kept, dropped)
	return kept, nil
}

// sweRepoNames lists the allowlisted repo short names for an error message.
func sweRepoNames() []string {
	var out []string
	for r := range sweGradeable {
		out = append(out, r)
	}
	return out
}

// sweFetch pulls instances from the dataset viewer, paging at 100, and keeps only
// the ones this tier can grade: pytest repos, and unless all is set, the
// allowlisted ones. A non-empty only set narrows the pull further to those repo
// short names. It stops once it has target keepers or the split runs out.
func sweFetch(ctx context.Context, target int, all bool, only map[string]bool) ([]sweRow, error) {
	var out []sweRow
	for offset := 0; len(out) < target; offset += swePageSize {
		q := url.Values{
			"dataset": {sweDataset}, "config": {"default"}, "split": {"test"},
			"offset": {fmt.Sprint(offset)}, "length": {fmt.Sprint(swePageSize)},
		}
		var page struct {
			Rows []struct {
				Row sweRow `json:"row"`
			} `json:"rows"`
		}
		if err := httpGetJSON(ctx, sweRowsAPI+"?"+q.Encode(), &page); err != nil {
			return nil, err
		}
		if len(page.Rows) == 0 {
			break
		}
		for _, r := range page.Rows {
			short := repoShort(r.Row.Repo)
			if sweSkip[short] {
				continue
			}
			if len(only) > 0 && !only[short] {
				continue
			}
			if !all && !sweGradeable[short] {
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

// repoShort returns the part of an owner/name repo after the slash, lowercased,
// which is what the allow and skip lists key on.
func repoShort(repo string) string {
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		repo = repo[i+1:]
	}
	return strings.ToLower(repo)
}

// sweStrList decodes a test list that the dataset ships as a JSON-encoded string
// holding an array (the common case) or, defensively, as a real JSON array.
func sweStrList(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		_ = json.Unmarshal([]byte(s), &arr)
	}
	return arr
}

var sweSlugRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sweSlug sanitizes an instance id into a directory-safe task name. Instance ids
// are already of the form owner__name-1234, so this only guards the odd stray
// character.
func sweSlug(id string) string {
	return strings.Trim(sweSlugRe.ReplaceAllString(id, "-"), "-")
}

// gitApply applies a unified diff to a checked-out work tree, used both to lay the
// gold fix down during validation and, from check.sh, to lay the hidden test patch
// down at grading time.
func gitApply(ctx context.Context, work, diff string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", work, "apply", diff)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git apply %s: %v: %s", filepath.Base(diff), err, trim(string(out), 200))
	}
	return nil
}

func (l *Lab) sweMaterialize(row sweRow) (task, oracle string, err error) {
	name := sweSlug(row.InstanceID)
	task = filepath.Join(l.tasksDir(), name)
	oracle = filepath.Join(l.suiteDir(), "oracle", name)
	os.RemoveAll(task)
	os.RemoveAll(oracle)

	// The hidden answer stays under oracle/, never mounted: the gold fix, the test
	// patch, and the two test lists check.sh grades against, plus the repo and
	// commit setup.sh reads to clone, and the era pins plus install args check.sh
	// builds the venv from.
	spec := sweSpecFor(row.Repo, row.Version)
	files := map[string]string{
		"gold.diff":        row.Patch,
		"test.diff":        row.TestPatch,
		"fail_to_pass.txt": strings.Join(sweStrList(row.FailToPass), "\n") + "\n",
		"pass_to_pass.txt": strings.Join(sweStrList(row.PassToPass), "\n") + "\n",
		"repo":             row.Repo + "\n",
		"base_commit":      row.BaseCommit + "\n",
		"pins.txt":         strings.Join(spec.pip, "\n") + "\n",
		"install":          spec.install + "\n",
		"python":           spec.python + "\n",
	}
	for fn, body := range files {
		if err = writeFile(filepath.Join(oracle, fn), []byte(body), 0o644); err != nil {
			return
		}
	}

	if err = writeFile(filepath.Join(task, "prompt.txt"), []byte(swePrompt(row)), 0o644); err != nil {
		return
	}
	desc := fmt.Sprintf("swebench-lite: %s (%s)\n", row.InstanceID, row.Repo)
	if err = writeFile(filepath.Join(task, "desc"), []byte(desc), 0o644); err != nil {
		return
	}
	if err = writeDefaultTags(task); err != nil {
		return
	}
	if err = writeFile(filepath.Join(task, "setup.sh"), []byte(sweSetup), 0o755); err != nil {
		return
	}
	if err = writeFile(filepath.Join(task, "check.sh"), []byte(sweCheck), 0o755); err != nil {
		return
	}
	return task, oracle, nil
}

// swePrompt renders the task for the agent from the embedded prompts/swebench.md
// template: the issue text framed the way the benchmark's own prompt frames it,
// with no instruction about tests (see prompts.go).
func swePrompt(row sweRow) string {
	return renderPrompt("swebench.md", row.Repo, row.ProblemStatement)
}

// sweSetup clones the instance's repository at its base commit into the work tree
// so the agent edits the real project. It keeps a bare mirror under the data root
// ($LAB_DATA, default $HOME/data/tomo-labs) rather than in the repo, so a second
// task on the same repo, or a rerun, clones from local disk instead of the
// network, and the multi-gigabyte mirrors never sit inside the checkout. The work
// tree is the harness's, created empty just before this runs, so the clone lands
// at its root and the agent's cwd is the repo.
//
// The clone is --no-hardlinks, not --shared. A --shared clone writes a
// .git/objects/info/alternates that points back at the host cache dir; once the
// work tree is mounted into the run container that host path is gone, so every
// git command the agent runs dies with "bad object HEAD". --no-hardlinks copies
// the objects into the work tree so its git is self-contained and works in the
// container, at the cost of a little disk the benchmark can spare.
const sweSetup = `#!/usr/bin/env bash
# Check out the repository at the instance's base commit into the work tree, then
# strip every commit, branch, and tag that came after it, so the instance's own
# gold fix is not sitting in the work tree's git history for a tool to find and
# cherry-pick instead of solving the bug.
set -e
W="$1"
D="$(cd "$(dirname "$0")" && pwd)"
SUITE="$(cd "$D/../.." && pwd)"
NAME="$(basename "$D")"
ORACLE="$SUITE/oracle/$NAME"
REPO="$(cat "$ORACLE/repo")"
SHA="$(cat "$ORACLE/base_commit")"
DATA="${LAB_DATA:-$HOME/data/tomo-labs}"
CACHE="$DATA/cache/$(basename "$SUITE")/$(echo "$REPO" | tr '/' '_').git"
if [ ! -d "$CACHE" ]; then
  mkdir -p "$(dirname "$CACHE")"
  git clone --bare --quiet "https://github.com/$REPO.git" "$CACHE"
fi
git clone --quiet --no-hardlinks "$CACHE" "$W"
` + sweStripFuture

// sweStripFuture removes every commit, branch, and tag not reachable at or before
// the base commit from the checked-out work tree, so the instance's gold fix is
// not in the work tree's git history. Without it a clever tool can skip the bug
// and just git-log the answer: a real run had gpt-5.6-sol pass a task by diffing
// the base against the upstream fix commit and applying it, which the full clone
// left reachable.
//
// It pins the base under one branch, drops the remote and every other branch,
// deletes any tag that is not an ancestor of the base (a later release could
// otherwise carry the fix), then expires the reflog and prunes so the future
// objects are actually gone rather than merely unreferenced. Ancestor tags stay,
// so a project that reads its version from git (setuptools_scm and the like)
// still installs. It expects $W (the work tree) and $SHA (the base commit) set,
// and is shared with its test so both prune exactly the same way.
const sweStripFuture = `git -C "$W" checkout --quiet -B __base "$SHA"
git -C "$W" remote remove origin 2>/dev/null || true
for ref in $(git -C "$W" for-each-ref --format='%(refname)' refs/heads); do
  [ "$ref" = "refs/heads/__base" ] || git -C "$W" update-ref -d "$ref"
done
for tag in $(git -C "$W" tag); do
  git -C "$W" merge-base --is-ancestor "refs/tags/$tag" __base 2>/dev/null || git -C "$W" tag -d "$tag" >/dev/null
done
git -C "$W" reflog expire --expire=now --all >/dev/null 2>&1 || true
git -C "$W" gc --prune=now --quiet 2>/dev/null || true
`

// sweCheck grades the work tree: it builds a throwaway virtualenv on the instance's
// era Python, installs the era pins and the checked-out project into it, applies
// the hidden test patch on top of the agent's edits, and runs the two test sets.
// FAIL_TO_PASS must turn green and PASS_TO_PASS must stay green for the task to
// pass. The venv is built with uv on the interpreter the instance targets
// (oracle/python), because these instances are years old and their packages do not
// build or import on a current host Python; uv downloads that interpreter on first
// use. The era pins (oracle/pins.txt) go in before the project so a plain install
// cannot drag in today's incompatible dependency versions; the install args
// (oracle/install) are the project install recipe for this repo and version. The
// oracle sits two levels up under the suite's oracle/, which the harness never
// mounts. It avoids bash 4 features (mapfile) so it runs under the macOS system bash.
const sweCheck = `#!/usr/bin/env bash
# Grade against the hidden SWE-bench tests: build the era's Python, pin the era's
# deps, install the project, apply the test patch, then run FAIL_TO_PASS (must
# pass) and PASS_TO_PASS (must stay passing).
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

# The era pins first, so the project install below cannot upgrade them away.
PINS=(); while IFS= read -r line; do [ -n "$line" ] && PINS+=("$line"); done < "$ORACLE/pins.txt"
if [ "${#PINS[@]}" -gt 0 ]; then
  if ! uv pip install --python "$PY" -q "${PINS[@]}" >/dev/null 2>&1; then
    echo "FAIL: could not install era pins"; exit 1
  fi
fi

# The project itself, using this instance's install recipe (-e ., ., -e .[test]).
read -ra INSTALL < "$ORACLE/install"
if ! ( cd "$W" && uv pip install --python "$PY" -q "${INSTALL[@]}" >/dev/null 2>&1 ); then
  echo "FAIL: could not install project"; exit 1
fi
# Make sure a test runner is present without clobbering a pinned or self-provided one.
"$PY" -c "import pytest" >/dev/null 2>&1 || uv pip install --python "$PY" -q pytest >/dev/null 2>&1

if ! git -C "$W" apply "$ORACLE/test.diff" >/dev/null 2>&1; then
  echo "FAIL: test patch did not apply"; exit 1
fi

# Read node ids into arrays without mapfile, so lines with spaces stay intact.
F2P=(); P2P=()
while IFS= read -r line; do [ -n "$line" ] && F2P+=("$line"); done < "$ORACLE/fail_to_pass.txt"
while IFS= read -r line; do [ -n "$line" ] && P2P+=("$line"); done < "$ORACLE/pass_to_pass.txt"

cd "$W"
rc1=0; rc2=0
if [ "${#F2P[@]}" -gt 0 ]; then
  "$PY" -m pytest -q -p no:cacheprovider "${F2P[@]}" >"$VENV/f2p.log" 2>&1 || rc1=$?
fi
if [ "${#P2P[@]}" -gt 0 ]; then
  "$PY" -m pytest -q -p no:cacheprovider "${P2P[@]}" >"$VENV/p2p.log" 2>&1 || rc2=$?
fi

if [ "$rc1" -eq 0 ] && [ "$rc2" -eq 0 ]; then
  echo "PASS: FAIL_TO_PASS and PASS_TO_PASS green"
  exit 0
fi
echo "FAIL: fail_to_pass rc=$rc1 pass_to_pass rc=$rc2"
tail -n 3 "$VENV/f2p.log" 2>/dev/null
exit 1
`
