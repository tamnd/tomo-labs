#!/usr/bin/env bash
W="$1"
[ -f "$W/cheapest.txt" ] || { echo "cheapest.txt missing"; exit 1; }
if grep -qi 'gadget' "$W/cheapest.txt"; then echo "cheapest product identified"; exit 0; fi
echo "cheapest.txt did not name the cheapest product"; cat "$W/cheapest.txt"; exit 1
