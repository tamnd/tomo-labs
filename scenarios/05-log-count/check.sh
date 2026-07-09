#!/usr/bin/env bash
W="$1"
[ -f "$W/count.txt" ] || { echo "count.txt missing"; exit 1; }
n="$(tr -dc '0-9' < "$W/count.txt")"
if [ "$n" = "3" ]; then echo "counted the 500s correctly"; exit 0; fi
echo "count.txt says '$n', expected 3"; exit 1
