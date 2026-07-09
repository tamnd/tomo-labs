#!/usr/bin/env bash
set -e
W="$1"
cat > "$W/sales.csv" <<'CSV'
date,amount
2026-01-01,100
2026-01-02,250
2026-01-03,175
CSV
