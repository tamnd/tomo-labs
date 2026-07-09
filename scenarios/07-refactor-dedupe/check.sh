#!/usr/bin/env bash
W="$1"
out="$(cd "$W" && node test.js 2>/dev/null)"
[ "$out" = "PASS" ] || { echo "test did not print PASS (got '$out')"; exit 1; }
defs="$(grep -c 'function add' "$W/util.js")"
if [ "$defs" -le 1 ]; then echo "duplicate removed and test passes"; exit 0; fi
echo "function is still defined $defs times"; exit 1
