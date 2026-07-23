#!/usr/bin/env bash
# One (task, codex) attempt on the ChatGPT subscription via bridge+proxy, on the
# same SWE-bench-Live-faithful flow as the other tools:
#   codex (swelive-int, no egress)
#     -> usage proxy  (swelive-int)         captures Responses token usage
#     -> codex bridge (swelive-int+egress)  injects the subscription token
#     -> chatgpt.com/backend-api/codex/responses
# codex can reach nothing but the proxy; only the bridge has egress, and only to
# the codex backend. Grading is offline in a fresh instance container.
set -uo pipefail
ROOT="$HOME/swelive"
SLUG="$1"; IMAGE="$2"; TEST_CMD="$3"
MODEL="${LAB_MODEL:-gpt-5.6-sol}"; EFFORT="${LAB_EFFORT:-high}"
OVR="${OVERLAY_IMAGE:-tomolab-inst-agents:$SLUG}"
TOOL="codex"

# --- networks: internal (no egress) + egress ---
docker network inspect swelive-int   >/dev/null 2>&1 || docker network create --internal swelive-int >/dev/null
docker network inspect swelive-egress >/dev/null 2>&1 || docker network create swelive-egress >/dev/null

# --- images ---
docker build -q -t swelive-bridge -f "$ROOT/net/Dockerfile.bridge" "$ROOT" >/dev/null
docker build -q -t swelive-usageproxy -f "$ROOT/net/Dockerfile.proxy" "$ROOT/net" >/dev/null
if ! docker image inspect "$OVR" >/dev/null 2>&1; then
  echo "[build] $OVR FROM $IMAGE"
  docker build --platform linux/amd64 --build-arg BASE="$IMAGE" \
    -t "$OVR" -f "$ROOT/overlay/Dockerfile.overlay" "$ROOT/overlay" 2>&1 | tail -8
fi

TDIR="$ROOT/runs/$SLUG/$TOOL"; rm -rf "$TDIR"; mkdir -p "$TDIR/trace"
mkdir -p "$ROOT/proxytrace" "$ROOT/bridgetrace"; : > "$ROOT/proxytrace/usage.jsonl"; rm -f "$ROOT/bridgetrace"/*.json 2>/dev/null || true

# --- bridge (subscription token holder, has egress) ---
docker rm -f swelive-bridge >/dev/null 2>&1 || true
docker run -d --name swelive-bridge --network swelive-egress \
  -e LAB_RUNTIME=noop \
  -v "$ROOT/codexauth":/auth \
  -v "$ROOT/bridgetrace":/bridgetrace \
  swelive-bridge --port 8790 --model "$MODEL" --effort "$EFFORT" \
  --auth /auth/auth.json --trace /bridgetrace >/dev/null
docker network connect swelive-int swelive-bridge

# --- usage proxy (captures tokens, forwards to bridge) ---
docker rm -f swelive-proxy >/dev/null 2>&1 || true
docker run -d --name swelive-proxy --network swelive-int \
  -e UPSTREAM="http://swelive-bridge:8790" \
  -e USAGE_LOG=/trace/usage.jsonl \
  -v "$ROOT/proxytrace":/trace \
  swelive-usageproxy >/dev/null
sleep 2
echo "[net] bridge+proxy up; health: $(docker run --rm --network swelive-int curlimages/curl:latest -s -m 5 http://swelive-bridge:8790/healthz 2>/dev/null || echo '?')"

echo "[run] codex on $SLUG (model=$MODEL effort=$EFFORT, subscription via bridge, gateway-only egress)"
timeout "${ATTEMPT_TIMEOUT:-1800}" docker run --rm --platform linux/amd64 \
  --network swelive-int \
  -e LAB_BASE_URL="http://swelive-proxy:8080/v1" \
  -e LAB_MODEL="$MODEL" \
  -e OPENCODE_API_KEY="unused-bridge-injects-token" \
  -v "$ROOT/scenario":/scenario:ro \
  -v "$TDIR/trace":/trace \
  "$OVR" codex
rc=$?
cp "$ROOT/proxytrace/usage.jsonl" "$TDIR/trace/usage.jsonl" 2>/dev/null || true
cp -r "$ROOT/bridgetrace" "$TDIR/trace/bridgetrace" 2>/dev/null || true
docker rm -f swelive-proxy swelive-bridge >/dev/null 2>&1 || true
echo "[agent done] rc=$rc exit_code=$(cat "$TDIR/trace/exit_code" 2>/dev/null) patch_lines=$(cat "$TDIR/trace/model.patch.lines" 2>/dev/null)"

echo "[grade] offline, fresh instance container, upstream flow"
bash "$ROOT/eval_instance.sh" "$IMAGE" "$TEST_CMD" \
  "$TDIR/trace/model.patch" "$ROOT/dyn/test.patch" "$ROOT/dyn/f2p.json" "$ROOT/dyn/p2p.json" \
  "$TDIR/grade" 2>&1 | tee "$TDIR/grade.log" | tail -14
