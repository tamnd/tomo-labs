#!/usr/bin/env bash
# Three-arm A/B on one SWE-bench-Live instance, same task/model/grader, to
# separate two effects the context pack could have:
#   nopack   pin PRE_PACK          engine builds no context pack
#   pack     pin WITH_PACK         regex-resolved context pack (indentation slice)
#   pack-lsp pin WITH_PACK +LSP    same pack, symbols resolved through pyright
# Only pack-lsp sets TOMO_LSP, so the engine's stderr prints an
# "oi: context pack via LSP (...) resolved N/M symbols" line we grep to PROVE the
# LSP path was live and not a silent regex fallback.
#
#   OPENCODE_API_KEY=... LAB_MODEL=deepseek-v4-flash \
#   bash lsp_ab.sh <slug> <image> "<test_cmd>" [N]
set -uo pipefail
export DOCKER_BUILDKIT=1
ROOT="$HOME/swelive"
SLUG="$1"; IMAGE="$2"; TEST_CMD="$3"; N="${4:-1}"
: "${OPENCODE_API_KEY:?set OPENCODE_API_KEY}"
PRE="${PIN_PRE:-ef3262a}"; PACK="${PIN_PACK:-441f577}"
LSP_ARGV="${LSP_ARGV:-pyright-langserver --stdio}"
MODEL="${LAB_MODEL:-deepseek-v4-flash}"

bash "$ROOT/net/net_up.sh" >/dev/null
echo "[pull] $IMAGE"; docker pull --platform linux/amd64 "$IMAGE" 2>&1 | tail -1

build_overlay(){ # tag ref
  local ovr="tomolab-inst-agents:${SLUG}-$1"
  echo "[build] $ovr TOMO_REF=$2"
  # --load forces the built image into the local store (a BuildKit build can
  # otherwise leave it only in cache, so a later `docker image inspect` misses it).
  docker build --load --platform linux/amd64 --build-arg BASE="$IMAGE" --build-arg TOMO_REF="$2" \
    -t "$ovr" -f "$ROOT/overlay/Dockerfile.overlay" "$ROOT/overlay" 2>&1 | tail -4
  # Verify the baked binary is the pin we asked for. The version string ends in the
  # commit; a mismatch means the pin did not take, so fail rather than A/B two
  # copies of the same commit and call the null result a finding.
  local got
  got=$(docker run --rm --entrypoint tomo "$ovr" --version 2>/dev/null | head -1)
  if ! printf '%s' "$got" | grep -q "$2"; then
    echo "[error] $ovr baked '$got', expected commit $2" >&2; exit 4
  fi
  echo "[ok] $ovr baked $2"
}
build_overlay pre  "$PRE"
build_overlay pack "$PACK"

summary="$ROOT/runs/$SLUG/lsp_ab_summary.tsv"; mkdir -p "$(dirname "$summary")"; : > "$summary"
run_arm(){ # arm overlay_tag tomo_lsp
  local arm="$1" ovr="tomolab-inst-agents:${SLUG}-$2" lsp="$3"
  for i in $(seq 1 "$N"); do
    local label="tomo-oi-${arm}-${i}"
    echo "==================== $label (model=$MODEL, TOMO_LSP='${lsp}') ===================="
    OVERLAY_IMAGE="$ovr" RUN_LABEL="$label" TOMO_ENGINE=oi TOMO_LSP="$lsp" \
      LAB_MODEL="$MODEL" bash "$ROOT/run_task.sh" "$SLUG" "$IMAGE" tomo "$TEST_CMD"
    local d="$ROOT/runs/$SLUG/$label"
    local res pl ec lspline
    res=$(grep -m1 'RESOLVED:' "$d/grade.log" 2>/dev/null | awk '{print $2}')
    pl=$(cat "$d/trace/model.patch.lines" 2>/dev/null)
    ec=$(cat "$d/trace/exit_code" 2>/dev/null)
    lspline=$(grep -m1 'context pack via LSP\|LSP.*falling back' "$d/trace/stderr.log" 2>/dev/null | sed 's/\t/ /g')
    printf '%s\t%s\tpatch_lines=%s\texit=%s\tlsp=[%s]\n' \
      "$label" "${res:-NA}" "${pl:-NA}" "${ec:-NA}" "${lspline:-none}" | tee -a "$summary"
  done
}
run_arm nopack   pre  ""
run_arm pack     pack ""
run_arm pack-lsp pack "$LSP_ARGV"
echo "==================== SUMMARY ===================="; cat "$summary"
