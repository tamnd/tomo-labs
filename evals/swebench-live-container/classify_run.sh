#!/usr/bin/env bash
# Classify one finished (task,tool) attempt as a genuine result or an infra
# ABORT, so a provider/gateway failure never again masquerades as a graded model
# failure. This is the abort-is-not-fail rule enforced in the harness: a run that
# died because the zen free tier returned 400 "Upstream request failed", ran out
# of credits (401 Insufficient balance), got rate-limited (429), or 5xx'd was
# never given a fair chance, so grading its empty working tree as resolved=False
# with patch=0 is a false capability signal. dynaconf-1225 taught this the hard
# way: three deepseek arms all scored patch=0 and were read as "capability-bound"
# when in fact two died on zen 400/401 and one was cut off mid-solve.
#
# Usage: classify_run.sh <trace_dir>
# Writes <trace_dir>/aborted (holding "<reason>\t<evidence>") when the run is an
# infra abort, and exits 10. Exits 0 when the run is a real result to grade.
set -uo pipefail
TRACE="${1:?trace dir}"
ERR="$TRACE/stderr.log"
mark(){ printf '%s\t%s\n' "$1" "$2" > "$TRACE/aborted"; echo "[abort] $1: $2"; exit 10; }

if [ -f "$ERR" ]; then
  # The tomo CLI renders provider errors inside a hard-wrapped box, so a signature
  # like "Upstream request failed" is split across physical lines with trailing
  # padding. Collapse all whitespace (newlines included) to single spaces first so
  # wrapped messages still match, then scan the flattened text.
  flat="$(tr '\n\t' '  ' < "$ERR" | tr -s ' ')"
  low="$(printf '%s' "$flat" | tr '[:upper:]' '[:lower:]')"
  case "$low" in
    *"insufficient balance"*|*creditserror*|*"402 "*)
      mark provider_credits "$(printf '%s' "$flat" | grep -oiE '[^.]*(insufficient balance|creditserror)[^.]*' | head -1)" ;;
    *"upstream request failed"*|*"bad gateway"*|*"502 "*|*"503 "*|*"504 "*|*"529 "*|*"service unavailable"*|*overloaded*)
      mark provider_upstream "$(printf '%s' "$flat" | grep -oiE '[^.]*(upstream request failed|bad gateway|service unavailable|overloaded|50[234] |529 )[^.]*' | head -1)" ;;
    *"rate limit"*|*"rate-limit"*|*"429 "*|*"too many requests"*)
      mark provider_ratelimit "$(printf '%s' "$flat" | grep -oiE '[^.]*(rate.?limit|429 |too many requests)[^.]*' | head -1)" ;;
    *"invalid_request_error"*|*"400 bad request"*|*"401 unauthorized"*|*"500 internal"*)
      mark provider_error "$(printf '%s' "$flat" | grep -oiE '[^.]*(40[01] |500 |invalid_request_error)[^.]*' | head -1)" ;;
  esac
fi

# No provider error in stderr, but the agent never wrote its exit code: the run
# was cut off mid-flight (external kill, OOM, wall-clock chop) rather than
# finishing. An incomplete trace is not a gradeable model result either.
if [ ! -f "$TRACE/exit_code" ]; then
  mark incomplete "agent did not write exit_code; run cut off before finishing"
fi
exit 0
