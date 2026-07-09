#!/usr/bin/env bash
# lab.sh is the whole lab from the outside: build the images, run a tool over
# the scenarios, and report. Each run is one tool against one scenario in a
# throwaway container, with its LLM traffic routed through the trace proxy and
# every artifact, byte of I/O, and resource number captured under
# $HOME/data/<tool>/<scenario>/<timestamp>/.
#
#   ./lab.sh build [tool]            build base, proxy, and tool images
#   ./lab.sh run [tool] [scenario]   run all, or one tool, or one pair
#   ./lab.sh tools                   list wired tools
#   ./lab.sh scenarios               list scenarios
#   ./lab.sh report                  summarize captured runs
#
# It needs OPENCODE_API_KEY (or another OpenAI-compatible key, with LAB_UPSTREAM
# and LAB_MODEL pointed to match).
set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
source "$ROOT/lib/runtime.sh"

DATA="${LAB_DATA:-$HOME/data}"
LAB_MODEL="${LAB_MODEL:-opencode/deepseek-v4-flash-free}"
LAB_UPSTREAM="${LAB_UPSTREAM:-https://opencode.ai/zen}"
LAB_MAX_TURNS="${LAB_MAX_TURNS:-12}"
PROXY_HOST_PORT="${LAB_PROXY_PORT:-8899}"
NET="tomolab"

RT="$(detect_runtime)" || exit 1

log() { printf '%s\n' "$*" >&2; }
die() { log "error: $*"; exit 1; }

list_tools() {
  for d in "$ROOT"/tools/*/; do
    name="$(basename "$d")"
    [ "$name" = base ] && continue
    [ -f "$d/Dockerfile" ] && [ -f "$d/adapter.sh" ] && echo "$name"
  done
}

list_scenarios() {
  for d in "$ROOT"/scenarios/*/; do
    [ -f "$d/prompt.txt" ] && basename "$d"
  done
}

cmd_tools() { list_tools; }
cmd_scenarios() {
  for s in $(list_scenarios); do
    printf '%-22s %s\n' "$s" "$(head -1 "$ROOT/scenarios/$s/desc" 2>/dev/null)"
  done
}

cmd_build() {
  local only="${1:-}"
  log "[build] base image (tomolab-base)"
  "$RT" build -t tomolab-base "$ROOT/tools/base" || die "base image build failed"
  log "[build] trace proxy (tomolab-proxy)"
  "$RT" build -t tomolab-proxy "$ROOT/proxy" || die "proxy image build failed"
  for t in $(list_tools); do
    [ -n "$only" ] && [ "$t" != "$only" ] && continue
    log "[build] tool image (tomolab-tool-$t)"
    "$RT" build -t "tomolab-tool-$t" "$ROOT/tools/$t" || die "tool $t build failed"
  done
}

# stop_container removes a container by name, ignoring the case where it is not
# running, so a crashed prior run never blocks the next one.
stop_container() { "$RT" rm -f "$1" >/dev/null 2>&1 || true; }

ensure_network() { "$RT" network inspect "$NET" >/dev/null 2>&1 || "$RT" network create "$NET" >/dev/null; }

# run_one drives one tool through one scenario and writes a result.json.
run_one() {
  local tool="$1" scenario="$2"
  local sdir="$ROOT/scenarios/$scenario"
  [ -f "$sdir/prompt.txt" ] || die "unknown scenario: $scenario"
  "$RT" image inspect "tomolab-tool-$tool" >/dev/null 2>&1 || die "tool image missing, run: ./lab.sh build $tool"

  local ts run_dir work trace
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  run_dir="$DATA/$tool/$scenario/$ts"
  work="$run_dir/work"; trace="$run_dir/trace"
  mkdir -p "$work" "$trace"

  # Lay down the scenario's fixtures on the host, into the dir the container
  # will mount as its working tree.
  [ -f "$sdir/setup.sh" ] && bash "$sdir/setup.sh" "$work"
  local disk_before; disk_before="$(du -sk "$work" | cut -f1)"

  ensure_network
  stop_container tomolab-proxy; stop_container tomolab-web; stop_container tomolab-run

  # Trace proxy: this run's trace dir mounted in, its port published so the host
  # can tell when it is ready.
  "$RT" run -d --name tomolab-proxy --network "$NET" \
    -v "$trace:/trace" \
    -e "UPSTREAM=$LAB_UPSTREAM" -e TRACE_DIR=/trace \
    -p "127.0.0.1:$PROXY_HOST_PORT:8080" \
    tomolab-proxy >/dev/null || die "could not start proxy"
  ready_check "http://127.0.0.1:$PROXY_HOST_PORT/" || { "$RT" logs tomolab-proxy >&2; die "proxy never became ready"; }

  # Optional static web sidecar for scenarios that fetch a page.
  local used_web=0
  if [ -f "$sdir/web" ]; then
    used_web=1
    "$RT" run -d --name tomolab-web --network "$NET" \
      -v "$ROOT/webroot:/srv:ro" -w /srv \
      tomolab-base python3 -m http.server 80 >/dev/null || die "could not start web sidecar"
  fi

  log "[run] $tool / $scenario"
  local start end wall
  start="$SECONDS"
  "$RT" run --rm --name tomolab-run --network "$NET" \
    -v "$work:/work" -v "$trace:/trace" -v "$sdir:/scenario:ro" \
    -e "LAB_BASE_URL=http://tomolab-proxy:8080/v1" \
    -e "LAB_MODEL=$LAB_MODEL" \
    -e "OPENCODE_API_KEY=${OPENCODE_API_KEY:-}" \
    -e "LAB_MAX_TURNS=$LAB_MAX_TURNS" \
    "tomolab-tool-$tool"
  end="$SECONDS"; wall=$((end - start))

  stop_container tomolab-proxy
  [ "$used_web" = 1 ] && stop_container tomolab-web

  # Grade the work the agent left behind.
  local passed reason
  if reason="$(bash "$sdir/check.sh" "$work" 2>&1)"; then passed=true; else passed=false; fi
  local exit_code; exit_code="$(cat "$trace/exit_code" 2>/dev/null || echo -1)"
  local disk_after; disk_after="$(du -sk "$work" | cut -f1)"

  # Pull resource numbers out of the captured trace.
  local rss_kb elapsed reqs tok_prompt tok_completion tok_total
  rss_kb="$(awk -F': ' '/Maximum resident set size/ {print $2}' "$trace/time.txt" 2>/dev/null)"
  elapsed="$(awk -F': ' '/Elapsed \(wall clock\)/ {print $2}' "$trace/time.txt" 2>/dev/null)"
  reqs="$(wc -l < "$trace/requests.jsonl" 2>/dev/null | tr -d ' ')"
  read -r tok_prompt tok_completion tok_total < <(sum_tokens "$trace/usage.jsonl")

  write_result "$run_dir/result.json" \
    "$tool" "$scenario" "$ts" "$passed" "$exit_code" "$wall" \
    "${rss_kb:-0}" "${elapsed:-}" "${reqs:-0}" \
    "${tok_prompt:-0}" "${tok_completion:-0}" "${tok_total:-0}" \
    "$disk_before" "$disk_after" "$reason"

  local mark; [ "$passed" = true ] && mark="PASS" || mark="FAIL"
  printf '  %s  %-8s %-20s tokens=%s reqs=%s rss=%sMB disk=+%sKB  %s\n' \
    "$mark" "$tool" "$scenario" "${tok_total:-0}" "${reqs:-0}" \
    "$(kb_to_mb "${rss_kb:-0}")" "$((disk_after - disk_before))" \
    "$(printf '%s' "$reason" | head -1)"
}

# sum_tokens adds up usage across every recorded reply, printing three numbers.
sum_tokens() {
  local f="$1"
  if [ ! -s "$f" ] || ! command -v jq >/dev/null 2>&1; then echo "0 0 0"; return; fi
  jq -s '[.[].prompt_tokens] as $p | [.[].completion_tokens] as $c | [.[].total_tokens] as $t
         | "\(($p|add)//0) \(($c|add)//0) \(($t|add)//0)"' "$f" 2>/dev/null | tr -d '"' || echo "0 0 0"
}

kb_to_mb() { awk -v k="${1:-0}" 'BEGIN{ printf "%.1f", k/1024 }'; }

write_result() {
  local out="$1"; shift
  local tool="$1" scenario="$2" ts="$3" passed="$4" exit_code="$5" wall="$6" \
        rss_kb="$7" elapsed="$8" reqs="$9" tp="${10}" tc="${11}" tt="${12}" \
        db="${13}" da="${14}" reason="${15}"
  local reason_json; reason_json="$(printf '%s' "$reason" | head -1 | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))' 2>/dev/null || echo '""')"
  cat > "$out" <<JSON
{
  "tool": "$tool",
  "scenario": "$scenario",
  "timestamp": "$ts",
  "model": "$LAB_MODEL",
  "runtime": "$RT",
  "passed": $passed,
  "exit_code": $exit_code,
  "wall_seconds": $wall,
  "elapsed_clock": "$elapsed",
  "max_rss_kb": $rss_kb,
  "requests": $reqs,
  "tokens": { "prompt": $tp, "completion": $tc, "total": $tt },
  "disk_before_kb": $db,
  "disk_after_kb": $da,
  "disk_delta_kb": $((da - db)),
  "check": $reason_json
}
JSON
}

cmd_run() {
  local tool="${1:-}" scenario="${2:-}"
  local tools scenarios
  if [ -n "$tool" ]; then tools="$tool"; else tools="$(list_tools)"; fi
  if [ -n "$scenario" ]; then scenarios="$scenario"; else scenarios="$(list_scenarios)"; fi
  [ -n "$tools" ] || die "no tools wired yet (see tools/<name>/)"
  for t in $tools; do
    "$RT" image inspect "tomolab-tool-$t" >/dev/null 2>&1 || { log "skip $t: image missing, run ./lab.sh build $t"; continue; }
    for s in $scenarios; do run_one "$t" "$s"; done
  done
}

cmd_report() {
  command -v jq >/dev/null 2>&1 || die "report needs jq"
  local files; files="$(find "$DATA" -name result.json 2>/dev/null)"
  [ -n "$files" ] || { log "no runs captured yet under $DATA"; return; }
  echo "$files" | xargs cat | jq -s '
    group_by(.tool)[] |
    { tool: .[0].tool,
      runs: length,
      passed: (map(select(.passed)) | length),
      total_tokens: (map(.tokens.total) | add),
      avg_rss_mb: ((map(.max_rss_kb) | add) / length / 1024 | floor),
      avg_wall_s: ((map(.wall_seconds) | add) / length | floor) }'
}

case "${1:-}" in
  build)     shift; cmd_build "$@" ;;
  run)       shift; cmd_run "$@" ;;
  tools)     cmd_tools ;;
  scenarios) cmd_scenarios ;;
  report)    cmd_report ;;
  *) log "usage: ./lab.sh {build|run|tools|scenarios|report} [args]"; exit 2 ;;
esac
