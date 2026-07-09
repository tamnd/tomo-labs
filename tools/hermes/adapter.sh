#!/usr/bin/env bash
# adapter drives Hermes through one scenario, non-interactively, and leaves the
# work it did in /work for the checker to grade. It is the only hermes-specific
# glue in the lab: everything upstream of it (network, trace capture, resource
# accounting) is the same for every tool.
#
# It is the container entrypoint. The harness mounts:
#   /work      the scenario's working tree, writable, and the agent's cwd
#   /scenario  the scenario definition, read-only (prompt.txt)
#   /trace     where stdout, the rendered config, and the time report land
#
# and passes LAB_BASE_URL, LAB_MODEL, OPENCODE_API_KEY, LAB_MAX_TURNS.
set -uo pipefail

prompt="$(cat /scenario/prompt.txt)"

# Point Hermes at a custom OpenAI-compatible endpoint that is really the trace
# proxy, so every request/response and its token usage get captured without the
# agent's cooperation. base_url takes precedence over the provider, so setting it
# routes the model calls straight through the proxy, which forwards to the real
# upstream with the real key. The key lives in an env var Hermes reads
# (OPENAI_API_KEY); the proxy carries the true credential onward.
export OPENAI_API_KEY="${OPENCODE_API_KEY}"
export OPENAI_BASE_URL="${LAB_BASE_URL}"
hermes config set model.provider custom  >/trace/setup.log 2>&1
hermes config set model.base_url "${LAB_BASE_URL}" >>/trace/setup.log 2>&1
hermes config set model.default "${LAB_MODEL}"     >>/trace/setup.log 2>&1

# Render the effective config into the trace for the record, same as the other
# tools do. Hermes keeps it under ~/.hermes/config.yaml.
cp "$HOME/.hermes/config.yaml" /trace/config.yaml 2>/dev/null || true

# yolo: auto-approve every action. The container is the sandbox, so let the agent
# act and measure whether it can finish the task. This is Hermes' equivalent of
# tomo's all-allow policy, and it lets the shell scenarios run headless.
export HERMES_YOLO_MODE=1

cd /work
/usr/bin/time -v -o /trace/time.txt \
  hermes chat -q "$prompt" --yolo \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
