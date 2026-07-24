#!/usr/bin/env bash
set -e
ROOT="$HOME/swelive"
docker image inspect swelive-usageproxy >/dev/null 2>&1 || \
  docker build -t swelive-usageproxy -f "$ROOT/net/Dockerfile.proxy" "$ROOT/net" >/dev/null
docker network inspect swelive-int    >/dev/null 2>&1 || docker network create --internal swelive-int >/dev/null
docker network inspect swelive-egress >/dev/null 2>&1 || docker network create swelive-egress >/dev/null
mkdir -p "$ROOT/proxytrace"
# A proxy left behind by run_sub.sh points UPSTREAM at the codex bridge, and a
# zen call through it dies on the bridge's 404. Recreate unless the running one
# already targets the zen gateway.
UP="$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' swelive-proxy 2>/dev/null | sed -n 's/^UPSTREAM=//p')"
if ! docker ps --format '{{.Names}}' | grep -qx swelive-proxy || [ "$UP" != "https://opencode.ai" ]; then
  docker rm -f swelive-proxy >/dev/null 2>&1 || true
  docker run -d --name swelive-proxy --network swelive-egress \
    -e UPSTREAM=https://opencode.ai -e USAGE_LOG=/trace/usage.jsonl \
    -v "$ROOT/proxytrace":/trace swelive-usageproxy >/dev/null
  docker network connect swelive-int swelive-proxy
fi
echo "usage-proxy up"
