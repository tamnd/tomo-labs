#!/usr/bin/env bash
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
