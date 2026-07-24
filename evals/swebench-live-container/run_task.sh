#!/usr/bin/env bash
# One (task,tool) attempt, SWE-bench-Live-faithful:
#  1. overlay tool CLIs on the prebuilt instance image (setup phase, has network)
#  2. run the agent INSIDE that image at /testbed on an INTERNAL docker net whose
#     only reachable host is the gateway proxy -> no github/pypi/remote leak
#  3. grade offline in a FRESH instance container (--network none), upstream flow
set -uo pipefail
export DOCKER_BUILDKIT=1   # cache-mounts in Dockerfile.overlay need BuildKit
ROOT="$HOME/swelive"
SLUG="$1"; IMAGE="$2"; TOOL="$3"; TEST_CMD="$4"
# OVERLAY_IMAGE: use a specific prebuilt overlay (e.g. one pinned to an engine
# commit under test) instead of building the default tag. RUN_LABEL: name the
# output dir so repeated/A-B attempts don't overwrite each other.
OVR="${OVERLAY_IMAGE:-tomolab-inst-agents:$SLUG}"
LABEL="${RUN_LABEL:-$TOOL}"
bash "$ROOT/net/net_up.sh" >/dev/null
if ! docker image inspect "$OVR" >/dev/null 2>&1; then
  # A caller that hands us a specific OVERLAY_IMAGE (e.g. an A/B pinned to a tomo
  # commit) owns building it. Silently rebuilding here dropped the pin, since this
  # path only forwards TOMO_REF when set, so it baked the Dockerfile's DEFAULT
  # commit and the "pinned" arm ran the wrong binary. Fail loudly instead.
  if [ -n "${OVERLAY_IMAGE:-}" ]; then
    echo "[error] OVERLAY_IMAGE=$OVR not found; the caller must build it (pinned). Refusing to rebuild unpinned." >&2
    exit 3
  fi
  echo "[build] $OVR FROM $IMAGE"
  docker build --load --platform linux/amd64 --build-arg BASE="$IMAGE" \
    ${TOMO_REF:+--build-arg TOMO_REF="$TOMO_REF"} \
    -t "$OVR" -f "$ROOT/overlay/Dockerfile.overlay" "$ROOT/overlay" 2>&1 | tail -8
fi
TDIR="$ROOT/runs/$SLUG/$LABEL"; rm -rf "$TDIR"; mkdir -p "$TDIR/trace"
: > "$ROOT/proxytrace/usage.jsonl"
echo "[run] $TOOL on $SLUG (model=${LAB_MODEL:-deepseek-v4-flash-free}, gateway-only egress)"
timeout "${ATTEMPT_TIMEOUT:-1800}" docker run --rm --platform linux/amd64 \
  --network swelive-int \
  -e LAB_BASE_URL="http://swelive-proxy:8080/zen/v1" \
  -e LAB_MODEL="${LAB_MODEL:-deepseek-v4-flash-free}" \
  -e TOMO_ENGINE="${TOMO_ENGINE:-}" \
  -e TOMO_LSP="${TOMO_LSP:-}" \
  -e OPENCODE_API_KEY="$OPENCODE_API_KEY" \
  -v "$ROOT/scenario":/scenario:ro \
  -v "$TDIR/trace":/trace \
  "$OVR" "$TOOL"
rc=$?
cp "$ROOT/proxytrace/usage.jsonl" "$TDIR/trace/usage.jsonl" 2>/dev/null || true
echo "[agent done] rc=$rc exit_code=$(cat "$TDIR/trace/exit_code" 2>/dev/null) patch_lines=$(cat "$TDIR/trace/model.patch.lines" 2>/dev/null)"
# abort-is-not-fail: if the run died on a provider/gateway error (zen 400/401/429,
# 5xx), classify it as an ABORT and skip grading, so an infra failure never scores
# as a resolved=False capability result. The A/B summary scripts skip trace/aborted.
if ! bash "$ROOT/classify_run.sh" "$TDIR/trace"; then
  echo "[grade] skipped: run aborted on infra, not a model result (prune or retry)"
  exit 0
fi
echo "[grade] offline, fresh instance container, upstream flow"
bash "$ROOT/eval_instance.sh" "$IMAGE" "$TEST_CMD" \
  "$TDIR/trace/model.patch" "$ROOT/dyn/test.patch" "$ROOT/dyn/f2p.json" "$ROOT/dyn/p2p.json" \
  "$TDIR/grade" 2>&1 | tee "$TDIR/grade.log" | tail -12
