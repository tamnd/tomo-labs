#!/usr/bin/env bash
# A small stats helper with a real bug: it divides the total by one more than
# the count, so every average comes out too low.
set -e
W="$1"
cat > "$W/stats.py" <<'PY'
# Prints the mean of the readings.
readings = [4, 8, 15, 16, 23, 42]

total = 0
for r in readings:
    total += r

# BUG: dividing by len(readings) + 1 instead of len(readings).
mean = total / (len(readings) + 1)
print(mean)
PY
