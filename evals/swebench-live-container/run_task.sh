#!/usr/bin/env bash
# One (task,tool) attempt, SWE-bench-Live-faithful:
#  1. overlay tool CLIs on the prebuilt instance image (setup phase, has network)
#  2. run the agent INSIDE that image at /testbed on an INTERNAL docker net whose
#     only reachable host is the gateway proxy -> no github/pypi/remote leak
#  3. grade offline in a FRESH instance container (--network none), upstream flow
set -uo pipefail
ROOT="$HOME/swelive"
SLUG="$1"; IMAGE="$2"; TOOL="$3"; TEST_CMD="$4"
OVR="tomolab-inst-agents:$SLUG"
bash "$ROOT/net/net_up.sh" >/dev/null
if ! docker image inspect "$OVR" >/dev/null 2>&1; then
  echo "[build] $OVR FROM $IMAGE"
  docker build --platform linux/amd64 --build-arg BASE="$IMAGE" \
    -t "$OVR" -f "$ROOT/overlay/Dockerfile.overlay" "$ROOT/overlay" 2>&1 | tail -8
fi
TDIR="$ROOT/runs/$SLUG/$TOOL"; rm -rf "$TDIR"; mkdir -p "$TDIR/trace"
: > "$ROOT/proxytrace/usage.jsonl"
echo "[run] $TOOL on $SLUG (model=${LAB_MODEL:-deepseek-v4-flash-free}, gateway-only egress)"
timeout "${ATTEMPT_TIMEOUT:-1800}" docker run --rm --platform linux/amd64 \
  --network swelive-int \
  -e LAB_BASE_URL="http://swelive-proxy:8080/zen/v1" \
  -e LAB_MODEL="${LAB_MODEL:-deepseek-v4-flash-free}" \
  -e OPENCODE_API_KEY="$OPENCODE_API_KEY" \
  -v "$ROOT/scenario":/scenario:ro \
  -v "$TDIR/trace":/trace \
  "$OVR" "$TOOL"
rc=$?
cp "$ROOT/proxytrace/usage.jsonl" "$TDIR/trace/usage.jsonl" 2>/dev/null || true
echo "[agent done] rc=$rc exit_code=$(cat "$TDIR/trace/exit_code" 2>/dev/null) patch_lines=$(cat "$TDIR/trace/model.patch.lines" 2>/dev/null)"
echo "[grade] offline, fresh instance container, upstream flow"
bash "$ROOT/eval_instance.sh" "$IMAGE" "$TEST_CMD" \
  "$TDIR/trace/model.patch" "$ROOT/dyn/test.patch" "$ROOT/dyn/f2p.json" "$ROOT/dyn/p2p.json" \
  "$TDIR/grade" 2>&1 | tee "$TDIR/grade.log" | tail -12
