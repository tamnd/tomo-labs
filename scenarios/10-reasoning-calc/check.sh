#!/usr/bin/env bash
# (3 + 5) pallets * 24 boxes * 12 units + 100 loose = 2304 + 100 = 2404.
W="$1"
[ -f "$W/result.txt" ] || { echo "result.txt missing"; exit 1; }
n="$(tr -dc '0-9' < "$W/result.txt")"
if [ "$n" = "2404" ]; then echo "final number is correct"; exit 0; fi
echo "result.txt says '$n', expected 2404"; exit 1
