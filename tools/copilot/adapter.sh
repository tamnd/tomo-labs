#!/usr/bin/env bash
# adapter drives the GitHub Copilot CLI through one scenario, non-interactively,
# and leaves the work it did in /work for the checker to grade. It is the only
# copilot-specific glue in the lab: everything upstream of it (network, trace
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

# Copilot's BYOK mode points the CLI at a custom OpenAI-compatible provider
# entirely through environment variables, and when a provider base URL is set the
# CLI does not require GitHub authentication. Point the base at the trace proxy so
# every request/response and its token usage get captured with no cooperation from
# copilot; the proxy forwards to the real upstream with the real key. type openai
# and wire_api completions select the plain chat-completions dialect our proxy
# speaks; the model id is pinned to the lab's fixed model so token limits resolve
# against it rather than an unknown-model lookup.
export COPILOT_PROVIDER_BASE_URL="${LAB_BASE_URL}"
export COPILOT_PROVIDER_API_KEY="${OPENCODE_API_KEY}"
export COPILOT_PROVIDER_TYPE="openai"
export COPILOT_PROVIDER_WIRE_API="completions"
export COPILOT_MODEL="${LAB_MODEL}"
export COPILOT_PROVIDER_MODEL_ID="${LAB_MODEL}"
cat >/trace/config.json <<JSON
{
  "provider_base_url": "${LAB_BASE_URL}",
  "provider_type": "openai",
  "wire_api": "completions",
  "model": "${LAB_MODEL}"
}
JSON

# -p is copilot's headless one-shot mode: a single prompt, then exit. --allow-all
# turns on every permission at once (tools, paths, urls), copilot's equivalent of
# tomo's all-allow policy and what -p needs to run without stopping to confirm; the
# container is the sandbox, so the agent acts autonomously and we measure whether
# it finishes. --no-color keeps the log plain and --log-level none keeps copilot's
# own diagnostics out of the trace. It works in the current directory, so /work is
# the tree the checker inspects.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  copilot -p "$prompt" --allow-all --no-color --log-level none \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
