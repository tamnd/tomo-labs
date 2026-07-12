#!/usr/bin/env bash
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
