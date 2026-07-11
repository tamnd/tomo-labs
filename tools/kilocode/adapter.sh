#!/usr/bin/env bash
# adapter drives kilocode through one scenario, non-interactively, and leaves the
# work it did in /work for the checker to grade. It is the only kilocode-specific
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

# Kilo Code is an opencode fork and reads the same custom-provider config, from
# ~/.config/kilo/kilo.json. Register an OpenAI-compatible provider named lab whose
# baseURL is the trace proxy, so every request/response and its token usage get
# captured with no cooperation from kilo. The proxy forwards to the real upstream
# with the real key. The @ai-sdk/openai-compatible package it names is fetched at
# first run.
mkdir -p "$HOME/.config/kilo"
cat >"$HOME/.config/kilo/kilo.json" <<JSON
{
  "\$schema": "https://app.kilo.ai/config.json",
  "provider": {
    "lab": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "lab",
      "options": {
        "baseURL": "${LAB_BASE_URL}",
        "apiKey": "${OPENCODE_API_KEY}"
      },
      "models": {
        "${LAB_MODEL}": { "name": "${LAB_MODEL}" }
      }
    }
  }
}
JSON
cp "$HOME/.config/kilo/kilo.json" /trace/config.json 2>/dev/null || true

# run is kilo's headless one-shot mode: a single message, then exit. --auto
# approves every permission the run does not explicitly deny, kilo's equivalent of
# tomo's all-allow policy, so the shell scenarios run without a prompt; the
# container is the sandbox, so the agent acts autonomously and we measure whether
# it finishes. --dir pins the working tree to /work, the exact tree the checker
# inspects.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  kilo run --model "lab/${LAB_MODEL}" --dir /work --auto "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
