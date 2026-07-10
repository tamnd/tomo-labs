#!/usr/bin/env bash
# The catalog is served by the web sidecar from the repo's webroot. The orders
# are the local fixture the agent joins against it.
set -e
W="$1"
cat > "$W/orders.csv" <<'CSV'
product,qty
widget,10
gadget,3
sprocket,8
cog,20
flange,5
CSV
