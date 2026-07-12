#!/usr/bin/env bash
# adapter drives aider through one scenario, non-interactively, and leaves the
# work it did in /work for the checker to grade. It is the only aider-specific
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

# aider speaks to any OpenAI-compatible endpoint through litellm: a model named
# openai/<id> routes to litellm's OpenAI handler, which reads its base URL and key
# from OPENAI_API_BASE and OPENAI_API_KEY. Point the base at the trace proxy, so
# every request/response and its token usage get captured with no cooperation
# from aider, and hand it the real key; the proxy forwards to the real upstream.
export OPENAI_API_BASE="${LAB_BASE_URL}"
export OPENAI_API_KEY="${OPENCODE_API_KEY}"
cat >/trace/config.json <<JSON
{
  "model": "openai/${LAB_MODEL}",
  "openai_api_base": "${LAB_BASE_URL}"
}
JSON

# --message is aider's headless one-shot mode: a single instruction, then exit.
# --yes-always answers every confirmation with yes, aider's equivalent of tomo's
# all-allow policy, so it creates and edits files without stopping to ask; the
# container is the sandbox, so the agent acts autonomously and we measure whether
# it finishes. --no-git keeps aider out of version control, since /work is a plain
# tree rather than a repo, and stops it committing its own edits. The model our
# free upstream serves is not in litellm's metadata table, so
# --no-show-model-warnings skips the unknown-model prompt, and --no-check-update
# and --no-analytics keep it from phoning home. Its chat and input history are
# aider's own bookkeeping, not the agent's work, so they go to /trace rather than
# polluting the tree the checker inspects.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  aider \
    --model "openai/${LAB_MODEL}" \
    --no-git --yes-always \
    --no-show-model-warnings --no-check-update --no-analytics \
    --chat-history-file /trace/aider.chat.history.md \
    --input-history-file /trace/aider.input.history \
    --message "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
