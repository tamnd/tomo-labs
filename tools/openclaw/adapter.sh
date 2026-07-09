#!/usr/bin/env bash
# adapter drives openclaw through one scenario, non-interactively, and leaves
# the work it did in /work for the checker to grade. It is the only
# openclaw-specific glue in the lab: everything upstream of it (network, trace
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

# Baseline config and workspace. --accept-risk skips the interactive warning
# that a headless run can never answer.
openclaw setup --non-interactive --accept-risk >/trace/setup.log 2>&1

# Point openclaw at an OpenAI-compatible provider that is really the trace
# proxy, so every request/response and its token usage get captured without
# openclaw's cooperation. The proxy forwards to the upstream with the real key.
provider=$(cat <<JSON
{"api":"openai-completions","baseUrl":"${LAB_BASE_URL}","apiKey":"${OPENCODE_API_KEY}","models":[{"id":"${LAB_MODEL}","name":"${LAB_MODEL}","contextWindow":131072,"input":["text"]}]}
JSON
)
openclaw config set models.providers.lab "$provider" --strict-json >>/trace/setup.log 2>&1
openclaw models set "lab/${LAB_MODEL}" >>/trace/setup.log 2>&1

# Point the agent workspace at /work. By default openclaw does its file work in
# ~/.openclaw/workspace, which the harness never grades; making /work the
# canonical workspace is what puts openclaw on the same footing as tomo, whose
# cwd is /work. Both agents then act on the exact tree the checker inspects.
openclaw config set agents.defaults.workspace /work >>/trace/setup.log 2>&1

# yolo exec policy: full exec, no approval prompts. The container is the
# sandbox, so let the agent act and measure whether it can finish the task.
# This is openclaw's equivalent of tomo's all-allow policy.
openclaw exec-policy preset yolo >>/trace/setup.log 2>&1

# Render the effective config into the trace for the record, same as tomo does.
openclaw config file >/trace/config.path 2>/dev/null || true
cp "$HOME/.openclaw/openclaw.json" /trace/config.json 2>/dev/null || true

cd /work
/usr/bin/time -v -o /trace/time.txt \
  openclaw agent --local --session-key run --json \
    --timeout "$((LAB_MAX_TURNS * 120))" \
    --message "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
