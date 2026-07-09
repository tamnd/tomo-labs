#!/usr/bin/env bash
# clamp() must survive exactly once and the whole suite must still pass.
W="$1"
out="$(cd "$W" && node test.js 2>/dev/null)"
[ "$out" = "PASS" ] || { echo "test did not print PASS (got '$out')"; exit 1; }
defs="$(grep -c 'function clamp' "$W/util.js")"
if [ "$defs" -le 1 ]; then echo "duplicate removed and tests still pass"; exit 0; fi
echo "clamp is still defined $defs times"; exit 1
