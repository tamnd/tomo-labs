#!/usr/bin/env bash
W="$1"
[ -f "$W/summary.txt" ] || { echo "summary.txt missing"; exit 1; }
body="$(cat "$W/summary.txt")"
echo "$body" | grep -Eqi 'total:[[:space:]]*525' || { echo "total wrong"; echo "$body"; exit 1; }
echo "$body" | grep -Eqi 'top:[[:space:]]*2026-01-02' || { echo "top day wrong"; echo "$body"; exit 1; }
echo "summary total and top day are correct"
