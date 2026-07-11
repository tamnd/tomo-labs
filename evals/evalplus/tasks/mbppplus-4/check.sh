#!/usr/bin/env bash
# Grade against the hidden EvalPlus test suite.
W="$1"
D="$(cd "$(dirname "$0")" && pwd)"
SUITE="$(cd "$D/../.." && pwd)"
NAME="$(basename "$D")"
ORACLE="$SUITE/oracle/$NAME/test.py"
VENV="$SUITE/.venv"
if [ ! -x "$VENV/bin/python3" ]; then
  python3 -m venv "$VENV" >/dev/null 2>&1 && "$VENV/bin/pip" install -q numpy >/dev/null 2>&1
fi
runner="$(mktemp)"
cat "$W/solution.py" "$ORACLE" > "$runner"
cd "$W" && "$VENV/bin/python3" "$runner"
rc=$?
rm -f "$runner"
exit $rc
