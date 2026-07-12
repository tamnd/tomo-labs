#!/usr/bin/env bash
# adapter drives pi through one scenario, non-interactively, and leaves the work
# it did in /work for the checker to grade. It is the only pi-specific glue in
# the lab: everything upstream of it (network, trace capture, resource
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

# Register a custom OpenAI-compatible provider named lab whose baseUrl is the
# trace proxy, so every request/response and its token usage get captured with no
# cooperation from pi. pi reads model config from ~/.pi/agent/models.json; the
# api "openai-completions" points it at the proxy's /v1/chat/completions, which
# is what the proxy speaks. The apiKey is left as the literal $OPENCODE_API_KEY
# so pi interpolates the env var at run time and the real key never lands in the
# copy we drop in /trace. The proxy forwards to the real upstream with it.
mkdir -p "$HOME/.pi/agent"
# The model entry carries only id and name. pi's maxTokens (per-completion output
# budget) and contextWindow are both optional and pi fills them from its own
# defaults when omitted: maxTokens 16384, contextWindow 128000 (model-registry.js
# `modelDef.maxTokens ?? 16384`, `?? 128000`). We leave them out on purpose. An
# earlier config pinned maxTokens to 8192, half pi's default, which would have
# capped pi's completions below what pi ships with and let its compaction trip
# early. Every other adapter leaves the tool's own output budget untouched, so
# pi's does too: pi runs under exactly the budget it uses off the shelf.
cat >"$HOME/.pi/agent/models.json" <<JSON
{
  "providers": {
    "lab": {
      "baseUrl": "${LAB_BASE_URL}",
      "api": "openai-completions",
      "apiKey": "\$OPENCODE_API_KEY",
      "models": [
        {
          "id": "${LAB_MODEL}",
          "name": "${LAB_MODEL}"
        }
      ]
    }
  }
}
JSON
cp "$HOME/.pi/agent/models.json" /trace/config.json 2>/dev/null || true

# -p runs one prompt headless and prints the result, then exits. pi has no
# sandbox of its own and does not stop to approve a shell command or a file
# write: its built-in tools run with the process's permissions, which in this
# container is pi's equivalent of tomo's all-allow policy, the container being
# the sandbox. --model lab/$LAB_MODEL selects the proxied model, and -a trusts
# the project-local tree for this run so nothing pauses on a trust check. The run
# happens in /work, the exact tree the checker inspects.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  pi -p "$prompt" --model "lab/${LAB_MODEL}" -a \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
