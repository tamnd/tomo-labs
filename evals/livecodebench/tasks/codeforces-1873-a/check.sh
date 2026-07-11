#!/usr/bin/env bash
# Grade against the hidden LiveCodeBench tests with the vendored runner.
W="$1"
D="$(cd "$(dirname "$0")" && pwd)"
SUITE="$(cd "$D/../.." && pwd)"
NAME="$(basename "$D")"
ORACLE="$SUITE/oracle/$NAME"
LCB="$SUITE/oracle/_lcb"
VENV="$SUITE/.venv"
if [ ! -x "$VENV/bin/python3" ]; then
  python3 -m venv "$VENV" >/dev/null 2>&1 && "$VENV/bin/pip" install -q numpy >/dev/null 2>&1
fi
PYTHONPATH="$LCB" "$VENV/bin/python3" "$LCB/grade.py" "$W/solution.py" "$ORACLE"
