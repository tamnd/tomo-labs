#!/usr/bin/env bash
# The oracle counts 500s straight from the log the agent was handed, so the
# expected number is never something a human transcribed by hand.
W="$1"
[ -f "$W/count.txt" ] || { echo "count.txt missing"; exit 1; }
want="$(grep -cE '" 500 [0-9]+' "$W/access.log")"
n="$(tr -dc '0-9' < "$W/count.txt")"
if [ "$n" = "$want" ]; then echo "counted the 500s correctly ($want)"; exit 0; fi
echo "count.txt says '$n', expected $want"; exit 1
