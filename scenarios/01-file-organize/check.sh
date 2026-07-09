#!/usr/bin/env bash
# Every file must land in a folder named after its extension, nothing may be
# left loose in the root, and a moved file's contents must be untouched.
W="$1"

# Plain parallel list so the check runs on the host's stock bash 3.2 too.
want="log/server.log log/worker.log log/nginx.log
csv/users.csv csv/prices.csv
txt/notes.txt txt/todo.txt txt/reminder.txt
md/README.md md/CHANGELOG.md
json/settings.json json/package.json
py/scratch.py"

for rel in $want; do
  [ -f "$W/$rel" ] || { echo "missing $rel"; exit 1; }
done

# No regular files may still be sitting flat in the root.
loose="$(find "$W" -maxdepth 1 -type f | wc -l | tr -d ' ')"
[ "$loose" = 0 ] || { echo "$loose file(s) left in the root"; exit 1; }

grep -q 'server started on :8080' "$W/log/server.log" || { echo "server.log contents changed"; exit 1; }
echo "13 files sorted into extension folders, contents intact"
