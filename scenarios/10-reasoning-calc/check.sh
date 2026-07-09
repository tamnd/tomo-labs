#!/usr/bin/env bash
W="$1"
[ -f "$W/result.txt" ] || { echo "result.txt missing"; exit 1; }
n="$(tr -dc '0-9' < "$W/result.txt")"
if [ "$n" = "84" ]; then echo "final number is correct"; exit 0; fi
echo "result.txt says '$n', expected 84"; exit 1
