#!/usr/bin/env bash
# adapter drives Claude Code through one scenario, non-interactively, and leaves
# the work it did in /work for the checker to grade. It is the only
# claude-code-specific glue in the lab: everything upstream of it (network, trace
# capture, resource accounting) is the same for every tool.
#
# It is the container entrypoint. The harness mounts:
#   /work      the scenario's working tree, writable, and the agent's cwd
#   /scenario  the scenario definition, read-only (prompt.txt)
#   /trace     where stdout, the rendered config, and the time report land
#
# and passes LAB_BASE_URL, LAB_MODEL, OPENCODE_API_KEY, LAB_MAX_TURNS.
set -uo pipefail

prompt="$(cat /scenario/prompt.txt)"

# Point Claude Code at the trace proxy as if it were the Anthropic API. Claude
# Code appends /v1/messages to ANTHROPIC_BASE_URL, so strip the trailing /v1 the
# harness passes. The proxy translates that Messages call into the chat shape the
# free deepseek model speaks and back again, so Claude Code talks its native wire
# and never knows. ANTHROPIC_MODEL (and the small fast model it uses for cheap
# side tasks) both point at LAB_MODEL, so every request rides the shared model.
export ANTHROPIC_BASE_URL="${LAB_BASE_URL%/v1}"
export ANTHROPIC_AUTH_TOKEN="${OPENCODE_API_KEY}"
export ANTHROPIC_MODEL="${LAB_MODEL}"
export ANTHROPIC_SMALL_FAST_MODEL="${LAB_MODEL}"

# Keep the run offline except for the model: no update checks, telemetry, error
# reports, or other nonessential traffic that would muddy the trace.
export DISABLE_AUTOUPDATER=1
export DISABLE_TELEMETRY=1
export DISABLE_ERROR_REPORTING=1
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1

# Skip the first-run onboarding and folder-trust prompts a headless run can never
# answer, so the CLI goes straight to work.
cat >"$HOME/.claude.json" <<JSON
{"hasCompletedOnboarding": true, "bypassPermissionsModeAccepted": true}
JSON
cp "$HOME/.claude.json" /trace/config.json 2>/dev/null || true

# -p runs one prompt headless and prints the result. --dangerously-skip-permissions
# drops every approval prompt; the container is the sandbox, so this is Claude
# Code's equivalent of tomo's all-allow policy. --output-format text keeps stdout
# a plain transcript. The run is wrapped in /usr/bin/time -v for peak memory.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  claude -p "$prompt" --dangerously-skip-permissions --output-format text \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
