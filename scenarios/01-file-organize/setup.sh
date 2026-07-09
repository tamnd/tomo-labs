#!/usr/bin/env bash
# A messy download folder: a real spread of extensions and names the agent has
# to sort, not a handful of toy files.
set -e
W="$1"

printf '2026-07-01 08:14:22 INFO  server started on :8080\n2026-07-01 08:14:23 WARN  cache miss for key=user:42\n' > "$W/server.log"
printf '2026-07-01 09:02:10 ERROR db connection reset\n2026-07-01 09:02:11 INFO  reconnected after 1 retry\n' > "$W/worker.log"
printf 'nginx: 2026/07/01 accept4() failed (24: too many open files)\n' > "$W/nginx.log"

printf 'id,name,email\n1,Alice Tran,alice@example.com\n2,Bob Ng,bob@example.com\n' > "$W/users.csv"
printf 'sku,price\nA-100,12.50\nA-101,9.99\n' > "$W/prices.csv"

printf 'Meeting notes\n- ship the release\n- fix the flaky test\n' > "$W/notes.txt"
printf 'Grocery list: eggs, milk, coffee\n' > "$W/todo.txt"
printf 'Reminder: renew the domain\n' > "$W/reminder.txt"

printf '# Project\n\nA short readme.\n' > "$W/README.md"
printf '# Changelog\n\n- 0.1.0 first cut\n' > "$W/CHANGELOG.md"

printf '{"theme":"dark","fontSize":14}\n' > "$W/settings.json"
printf '{"version":"0.1.0","private":true}\n' > "$W/package.json"

printf 'print("hello")\n' > "$W/scratch.py"
