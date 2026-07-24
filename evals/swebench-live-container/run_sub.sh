#!/usr/bin/env bash
# One (task, tool) attempt on the ChatGPT subscription via bridge+proxy, on the
# same SWE-bench-Live-faithful flow as run_task.sh, but the model is served by
# the subscription bridge instead of a plain OpenAI-dialect gateway:
#   agent (swelive-int, no egress)
#     -> usage proxy  (swelive-int)         captures token usage
#     -> codex bridge (swelive-int+egress)  chat/completions -> Responses, injects
#                                           the subscription token, pins model
#     -> chatgpt.com/backend-api/codex/responses
# tomo/oi speak /v1/chat/completions; the bridge translates to the Responses wire.
# Grading is offline in a fresh instance container (--network none).
# Usage: run_sub.sh SLUG IMAGE TOOL TEST_CMD     (TOMO_ENGINE optional: agent|oi)
set -uo pipefail
ROOT="$HOME/swelive"
SLUG="$1"; IMAGE="$2"; TOOL="$3"; TEST_CMD="$4"
MODEL="${LAB_MODEL:-gpt-5.6-sol}"; EFFORT="${LAB_EFFORT:-high}"
ENGINE="${TOMO_ENGINE:-agent}"
OVR="${OVERLAY_IMAGE:-tomolab-inst-agents:$SLUG}"
LABEL="${RUN_LABEL:-${TOOL}-${ENGINE}}"

docker network inspect swelive-int    >/dev/null 2>&1 || docker network create --internal swelive-int >/dev/null
docker network inspect swelive-egress >/dev/null 2>&1 || docker network create swelive-egress >/dev/null

docker build -q -t swelive-bridge     -f "$ROOT/net/Dockerfile.bridge" "$ROOT"     >/dev/null
docker build -q -t swelive-usageproxy -f "$ROOT/net/Dockerfile.proxy"  "$ROOT/net" >/dev/null
if ! docker image inspect "$OVR" >/dev/null 2>&1; then
  echo "[build] $OVR FROM $IMAGE"
  docker build --platform linux/amd64 --build-arg BASE="$IMAGE" \
    -t "$OVR" -f "$ROOT/overlay/Dockerfile.overlay" "$ROOT/overlay" 2>&1 | tail -8
fi

TDIR="$ROOT/runs/$SLUG/$LABEL"; rm -rf "$TDIR"; mkdir -p "$TDIR/trace"
mkdir -p "$ROOT/proxytrace" "$ROOT/bridgetrace"; : > "$ROOT/proxytrace/usage.jsonl"; rm -f "$ROOT/bridgetrace"/*.json 2>/dev/null || true

# --- bridge (subscription token holder, has egress). LAB_RUNTIME=noop lets the
#     bridge subcommand start without a container runtime it never uses. ---
docker rm -f swelive-bridge >/dev/null 2>&1 || true
docker run -d --name swelive-bridge --network swelive-egress \
  -e LAB_RUNTIME=noop \
  -v "$ROOT/codexauth":/auth \
  -v "$ROOT/bridgetrace":/bridgetrace \
  swelive-bridge --port 8790 --model "$MODEL" --effort "$EFFORT" \
  --auth /auth/auth.json --trace /bridgetrace >/dev/null
docker network connect swelive-int swelive-bridge

docker rm -f swelive-proxy >/dev/null 2>&1 || true
docker run -d --name swelive-proxy --network swelive-int \
  -e UPSTREAM="http://swelive-bridge:8790" \
  -e USAGE_LOG=/trace/usage.jsonl \
  -v "$ROOT/proxytrace":/trace \
  swelive-usageproxy >/dev/null
sleep 2
echo "[net] bridge+proxy up; health: $(docker run --rm --network swelive-int curlimages/curl:latest -s -m 5 http://swelive-bridge:8790/healthz 2>/dev/null || echo '?')"

echo "[run] $TOOL/$ENGINE on $SLUG (model=$MODEL effort=$EFFORT, subscription via bridge, gateway-only egress)"
timeout "${ATTEMPT_TIMEOUT:-2400}" docker run --rm --platform linux/amd64 \
  --network swelive-int \
  -e LAB_BASE_URL="http://swelive-proxy:8080/v1" \
  -e LAB_MODEL="$MODEL" \
  -e TOMO_ENGINE="$ENGINE" \
  -e TOMO_LSP="${TOMO_LSP:-}" \
  -e TOMO_MAX_ROUNDS="${TOMO_MAX_ROUNDS:-}" \
  -e OPENCODE_API_KEY="unused-bridge-injects-token" \
  -v "$ROOT/scenario":/scenario:ro \
  -v "$TDIR/trace":/trace \
  "$OVR" "$TOOL"
rc=$?
cp "$ROOT/proxytrace/usage.jsonl" "$TDIR/trace/usage.jsonl" 2>/dev/null || true
cp -r "$ROOT/bridgetrace" "$TDIR/trace/bridgetrace" 2>/dev/null || true
docker rm -f swelive-proxy swelive-bridge >/dev/null 2>&1 || true
echo "[agent done] rc=$rc exit_code=$(cat "$TDIR/trace/exit_code" 2>/dev/null) patch_lines=$(cat "$TDIR/trace/model.patch.lines" 2>/dev/null)"

# abort-is-not-fail: a provider/gateway failure (bridge/proxy 400/401/429/5xx) is
# an infra abort, not a model result; classify and skip grading so it is pruned or
# retried rather than scored resolved=False.
if ! bash "$ROOT/classify_run.sh" "$TDIR/trace"; then
  echo "[grade] skipped: run aborted on infra, not a model result (prune or retry)"
  exit 0
fi
echo "[grade] offline, fresh instance container, upstream flow"
bash "$ROOT/eval_instance.sh" "$IMAGE" "$TEST_CMD" \
  "$TDIR/trace/model.patch" "$ROOT/dyn/test.patch" "$ROOT/dyn/f2p.json" "$ROOT/dyn/p2p.json" \
  "$TDIR/grade" 2>&1 | tee "$TDIR/grade.log" | tail -14

# --- self-publish: fold this run into the labs data layout (result.json + the
#     canonical codex-style date-tree session.jsonl) and best-effort mirror it to
#     the Hub, so the standard artifact exists the moment the run finishes and no
#     manual backfill is needed. Skipped cleanly when no lab binary is present.
#     INGEST_TOOL defaults to the run label (tomo-oi, tomo-agent), the dataset's
#     tool name for this bridge path. ---
LABBIN=""
for cand in "$ROOT/_labbin" "$ROOT/net/_labbin" "$(command -v labbin 2>/dev/null)"; do
  if [ -n "$cand" ] && [ -x "$cand" ]; then LABBIN="$cand"; break; fi
done
if [ -n "$LABBIN" ]; then
  echo "[publish] ingest $LABEL/$SLUG via $LABBIN"
  "$LABBIN" ingest-swelive --src "$TDIR" --tool "${INGEST_TOOL:-$LABEL}" \
    --scenario "$SLUG" --model "$MODEL" --runtime docker \
    || echo "[publish] ingest failed (non-fatal); $TDIR kept for backfill"
else
  echo "[publish] no lab binary found; skipping ingest ($TDIR kept for backfill)"
fi
