#!/usr/bin/env bash
# Sweep one swebench-live task across the shared free model roster with tomo-oi,
# scoring aborts apart from real failures. A run that ends with a recorded error
# and did not pass is an infra abort on the flaky free tier, not a task failure,
# so it is retried up to a bound instead of being counted. This is the per-task
# workhorse of the fifteen-task campaign: one command, one out-directory per
# model, an abort-aware grid at the end.
#
#   scripts/campaign_sweep.sh <task> [flags]
#
#     --models "a b c"   space-separated model list (default: the five free ones)
#     --engine <name>    engine to drive (default oi)
#     --out <dir>        parent out-dir (default <LAB_DATA>/campaign/<engine>/<task>)
#     --max-rounds <n>   per-run round cap (default 30)
#     --timeout <dur>    per-run inner deadline (default 600s)
#     --retries <n>      abort retries per model (default 2)
#     --no-grade         skip check.sh (default grades)
#     --lab <path>       lab binary (default builds to /tmp/lab)
#     --base-url <url>   OpenAI-compatible base (default zen); e.g. a local
#                        gateway http://host:8888/v1. Bearer from OPENCODE_API_KEY.
#
# The runner resolves the task dir relative to the working directory's
# swebench-live, so it MUST be run from the tomo-labs repo root. Source the
# OPENCODE_API_KEY first; live free runs need it.
set -uo pipefail

FREE_MODELS="opencode/deepseek-v4-flash-free opencode/mimo-v2.5-free opencode/hy3-free opencode/nemotron-3-ultra-free opencode/north-mini-code-free"

task="${1:-}"
if [ -z "$task" ] || [ "$task" = "-h" ] || [ "$task" = "--help" ]; then
  sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
fi
shift

models="$FREE_MODELS"
engine="oi"
out=""
max_rounds="30"
timeout="600s"
retries="2"
grade="--grade"
lab=""
base_url=""

while [ $# -gt 0 ]; do
  case "$1" in
    --models) models="$2"; shift 2;;
    --engine) engine="$2"; shift 2;;
    --out) out="$2"; shift 2;;
    --max-rounds) max_rounds="$2"; shift 2;;
    --timeout) timeout="$2"; shift 2;;
    --retries) retries="$2"; shift 2;;
    --no-grade) grade=""; shift;;
    --lab) lab="$2"; shift 2;;
    --base-url) base_url="$2"; shift 2;;
    *) echo "unknown flag: $1" >&2; exit 2;;
  esac
done

# Default to the durable lab store so a completed sweep survives and is not
# rerun. LAB_DATA overrides the root, matching the container harness and every
# task setup.sh; the layout is <root>/campaign/<engine>/<task>/<model_slug>/.
out="${out:-${LAB_DATA:-$HOME/data/tomo-labs}/campaign/$engine/$task}"
if [ -z "$lab" ]; then
  lab="/tmp/lab"
  echo "building lab -> $lab" >&2
  go build -o "$lab" ./cmd/lab || exit 1
fi

mkdir -p "$out"
echo "task=$task engine=$engine out=$out grade=${grade:-off}" >&2

for m in $models; do
  slug="$(echo "$m" | tr '/' '_')"
  mdir="$out/$slug"
  attempt=0
  while :; do
    echo ">> $m (attempt $((attempt+1)))" >&2
    "$lab" probe "$task" --engine "$engine" --model "$m" \
      --max-rounds "$max_rounds" --timeout "$timeout" $grade \
      ${base_url:+--base-url "$base_url"} \
      --out "$mdir" >/dev/null 2>>"$out/sweep.log"
    err="$(python3 -c "import json,sys;d=json.load(open('$mdir/summary.json'));print((d.get('error') or '').strip())" 2>/dev/null)"
    passed="$(python3 -c "import json,sys;d=json.load(open('$mdir/summary.json'));print(bool(d.get('passed')))" 2>/dev/null)"
    timed_out="$(python3 -c "import json,sys;d=json.load(open('$mdir/summary.json'));print(bool(d.get('timed_out')))" 2>/dev/null)"
    # A run that produced ZERO model tokens over a single round never actually
    # talked to the model: the gateway returned an empty/errored completion the
    # probe swallowed as a no-op (seen when a local model is evicted under memory
    # pressure mid-board). That is an abort, not a real attempt, so retry it.
    zero_tok="$(python3 -c "import json,sys;d=json.load(open('$mdir/summary.json'));print(bool((d.get('input_tokens') or 0)==0 and (d.get('output_tokens') or 0)==0 and not d.get('passed')))" 2>/dev/null)"
    # A timeout (deadline exceeded) is a real run that ran out of wall clock, not
    # a flaky-transport abort, so retrying it just burns another full timeout with
    # no new information. Only retry genuine infra aborts (429/400/no-route/etc).
    case "$err" in *"deadline exceeded"*|*"context canceled"*) timed_out="True";; esac
    if { [ -n "$err" ] || [ "$zero_tok" = "True" ]; } && [ "$passed" != "True" ] && [ "$timed_out" != "True" ] && [ "$attempt" -lt "$retries" ]; then
      echo "   abort: ${err:0:70}${err:+ }${zero_tok:+zero-token }-- retrying" >&2
      attempt=$((attempt+1))
      continue
    fi
    break
  done
done

echo "=== grid ($task) ===" >&2
python3 "$(dirname "$0")/campaign_grid.py" "$out"
