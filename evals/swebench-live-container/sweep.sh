#!/usr/bin/env bash
# Run all three agents sequentially on one task, then print a result table.
set -uo pipefail
ROOT="$HOME/swelive"
SLUG="$1"; IMAGE="$2"; TEST_CMD="$3"
for TOOL in opencode tomo pi; do
  echo "########################## $TOOL ##########################"
  ATTEMPT_TIMEOUT="${ATTEMPT_TIMEOUT:-1800}" LAB_MODEL="${LAB_MODEL:-deepseek-v4-flash-free}" \
    bash "$ROOT/run_task.sh" "$SLUG" "$IMAGE" "$TOOL" "$TEST_CMD"
  echo
done
echo "######################## SUMMARY ########################"
printf "%-10s %-9s %-11s %-9s %s\n" tool exit_code patch_lines resolved f2p_passed
for TOOL in opencode tomo pi; do
  T="$ROOT/runs/$SLUG/$TOOL"
  ec=$(cat "$T/trace/exit_code" 2>/dev/null||echo -)
  pl=$(cat "$T/trace/model.patch.lines" 2>/dev/null||echo -)
  res=$(grep -oE 'RESOLVED: (True|False)' "$T/grade.log" 2>/dev/null | tail -1 | awk '{print $2}')
  fp=$(grep -cE '^  PASSED ' "$T/grade/"*/dev/null 2>/dev/null; grep -A6 'FAIL_TO_PASS' "$T/grade.log" 2>/dev/null | grep -c 'PASSED')
  printf "%-10s %-9s %-11s %-9s %s\n" "$TOOL" "$ec" "$pl" "${res:-?}" "${fp:-?}"
done
