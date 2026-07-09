#!/usr/bin/env bash
# The mean of [4,8,15,16,23,42] is 108/6 = 18. Accept 18 or 18.0.
W="$1"
got="$(cd "$W" && python3 stats.py 2>/dev/null | tr -d '[:space:]')"
if awk -v g="$got" 'BEGIN{ exit !(g+0 == 18) }' 2>/dev/null; then
  echo "stats.py prints the correct mean"; exit 0
fi
echo "stats.py printed '$got', expected 18"; exit 1
