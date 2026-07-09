#!/usr/bin/env bash
W="$1"
[ -f "$W/adults.json" ] || { echo "adults.json missing"; exit 1; }
got="$(jq -c '[.[] | {name, age}]' "$W/adults.json" 2>/dev/null)"
want='[{"name":"Alice","age":30},{"name":"Carol","age":22},{"name":"Eve","age":41}]'
if [ "$got" = "$want" ]; then echo "adults filtered and sorted correctly"; exit 0; fi
echo "got: $got"; echo "want: $want"; exit 1
