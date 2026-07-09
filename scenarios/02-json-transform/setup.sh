#!/usr/bin/env bash
set -e
W="$1"
cat > "$W/users.json" <<'JSON'
[
  {"name": "Alice", "age": 30},
  {"name": "Bob", "age": 15},
  {"name": "Carol", "age": 22},
  {"name": "Dave", "age": 17},
  {"name": "Eve", "age": 41}
]
JSON
