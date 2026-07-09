#!/usr/bin/env bash
# Derive the true total and best day from the CSV the agent was given, then
# require the summary to match both. Amounts are matched loosely so a stray
# thousands separator or trailing text does not fail a correct answer.
W="$1"
[ -f "$W/summary.txt" ] || { echo "summary.txt missing"; exit 1; }
want_total="$(awk -F, 'NR>1{s+=$2} END{print s}' "$W/sales.csv")"
want_top="$(awk -F, 'NR>1{if($2+0>m){m=$2+0;d=$1}} END{print d}' "$W/sales.csv")"
body="$(cat "$W/summary.txt")"
echo "$body" | grep -Eqi "total:[[:space:]]*${want_total}\b" || { echo "total wrong (want $want_total)"; echo "$body"; exit 1; }
echo "$body" | grep -Eqi "top:[[:space:]]*${want_top}\b" || { echo "top day wrong (want $want_top)"; echo "$body"; exit 1; }
echo "summary total ($want_total) and top day ($want_top) are correct"
