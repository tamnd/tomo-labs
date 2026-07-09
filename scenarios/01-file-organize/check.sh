#!/usr/bin/env bash
W="$1"
for f in log/a.log log/b.log csv/data.csv txt/notes.txt txt/report.txt; do
  [ -f "$W/$f" ] || { echo "missing $f"; exit 1; }
done
# The originals must not still be sitting flat in the root.
for f in a.log b.log data.csv notes.txt report.txt; do
  [ -f "$W/$f" ] && { echo "$f was not moved out of the root"; exit 1; }
done
grep -q 'boot ok' "$W/log/a.log" || { echo "a.log contents changed"; exit 1; }
echo "files sorted into extension folders"
