#!/usr/bin/env bash
# A/B one SWE-bench-Live instance across two tomo pins, N attempts each, on the
# faithful container flow. Use it to test whether an engine change (e.g. the
# deterministic context pack) actually moves pass/fail on a real task, against
# the exact same task, model, and grader. Same-image, same-prompt, only the tomo
# commit baked into the agent differs.
#
#   OPENCODE_API_KEY=... LAB_MODEL=deepseek-v4-flash-free \
#   bash ab_pin.sh <slug> <image> "<test_cmd>" [N]
#
# PIN_OFF (baseline) and PIN_ON (change under test) override the two pins.
set -uo pipefail
export DOCKER_BUILDKIT=1   # cache-mounts in Dockerfile.overlay need BuildKit
ROOT="$HOME/swelive"
SLUG="$1"; IMAGE="$2"; TEST_CMD="$3"; N="${4:-5}"
: "${OPENCODE_API_KEY:?set OPENCODE_API_KEY}"
OFF="${PIN_OFF:-ef3262a}"; ON="${PIN_ON:-91127a1}"

bash "$ROOT/net/net_up.sh" >/dev/null
echo "[pull] $IMAGE"; docker pull --platform linux/amd64 "$IMAGE" 2>&1 | tail -1

build_overlay(){ # arm ref
  local ovr="tomolab-inst-agents:${SLUG}-$1"
  if ! docker image inspect "$ovr" >/dev/null 2>&1; then
    echo "[build] $ovr TOMO_REF=$2"
    docker build --platform linux/amd64 --build-arg BASE="$IMAGE" --build-arg TOMO_REF="$2" \
      -t "$ovr" -f "$ROOT/overlay/Dockerfile.overlay" "$ROOT/overlay" 2>&1 | tail -6
  fi
}
build_overlay packoff "$OFF"
build_overlay packon  "$ON"

summary="$ROOT/runs/$SLUG/ab_summary.tsv"; mkdir -p "$(dirname "$summary")"; : > "$summary"
for arm in packoff packon; do
  ovr="tomolab-inst-agents:${SLUG}-${arm}"
  for i in $(seq 1 "$N"); do
    label="tomo-oi-${arm}-${i}"
    echo "==================== $label ===================="
    OVERLAY_IMAGE="$ovr" RUN_LABEL="$label" TOMO_ENGINE=oi \
      LAB_MODEL="${LAB_MODEL:-deepseek-v4-flash-free}" \
      bash "$ROOT/run_task.sh" "$SLUG" "$IMAGE" tomo "$TEST_CMD"
    d="$ROOT/runs/$SLUG/$label"
    res=$(grep -m1 'RESOLVED:' "$d/grade.log" 2>/dev/null | awk '{print $2}')
    pl=$(cat "$d/trace/model.patch.lines" 2>/dev/null)
    ec=$(cat "$d/trace/exit_code" 2>/dev/null)
    printf '%s\t%s\tpatch_lines=%s\texit=%s\n' "$label" "${res:-NA}" "${pl:-NA}" "${ec:-NA}" | tee -a "$summary"
  done
done
echo "==================== SUMMARY ===================="; cat "$summary"
