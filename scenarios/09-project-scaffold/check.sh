#!/usr/bin/env bash
W="$1"
[ -s "$W/README.md" ] || { echo "README.md missing or empty"; exit 1; }
[ -f "$W/main.py" ] || { echo "main.py missing"; exit 1; }
[ -f "$W/Makefile" ] || { echo "Makefile missing"; exit 1; }
out="$(cd "$W" && make run 2>/dev/null)"
if echo "$out" | grep -q 'hello from tomo'; then echo "project scaffolded and make run works"; exit 0; fi
echo "make run did not print the message"; echo "$out"; exit 1
