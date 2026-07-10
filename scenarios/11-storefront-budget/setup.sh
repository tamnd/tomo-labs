#!/usr/bin/env bash
# The page is served by the web sidecar from the repo's webroot. The only local
# fixture is the budget the agent has to read off disk and join against it.
set -e
W="$1"
echo '$40.00' > "$W/budget.txt"
