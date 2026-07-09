#!/usr/bin/env bash
set -e
W="$1"
printf 'boot ok\n' > "$W/a.log"
printf 'shutdown\n' > "$W/b.log"
printf 'id,name\n1,x\n' > "$W/data.csv"
printf 'hello notes\n' > "$W/notes.txt"
printf 'quarterly\n' > "$W/report.txt"
