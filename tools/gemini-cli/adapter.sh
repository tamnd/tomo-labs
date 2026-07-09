#!/usr/bin/env bash
# adapter drives Google's Gemini CLI through one scenario, non-interactively, and
# leaves the work it did in /work for the checker to grade. It is the only
# gemini-cli-specific glue in the lab: everything upstream of it (network, trace
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

# gemini-cli speaks the Gemini generateContent wire, and our shared free deepseek
# model is chat-only upstream, so the proxy translates Gemini to chat and back at
# its edge; gemini-cli talks its native wire and never knows. The SDK builds its
# URL as {base}/v1beta/models/{model}:generateContent, so the base must not carry
# the /v1 suffix the OpenAI tools use. Strip it, and point the SDK at the proxy.
export GOOGLE_GEMINI_BASE_URL="${LAB_BASE_URL%/v1}"

# The key is our upstream credential; gemini-cli sends it as x-goog-api-key, which
# the proxy folds into the bearer it forwards.
export GEMINI_API_KEY="${OPENCODE_API_KEY}"

# Headless gemini-cli will not pick an auth method on its own, and its folder-trust
# feature otherwise downgrades --yolo back to prompting for approval. Both are
# settled in settings.json: pin the API-key auth so it never waits for an
# interactive choice, and disable folder trust so the agent can act. The
# --skip-trust flag below trusts the workspace for this session as a belt to that
# suspenders.
mkdir -p "$HOME/.gemini"
cat >"$HOME/.gemini/settings.json" <<'JSON'
{
  "security": {
    "auth": { "selectedType": "gemini-api-key" },
    "folderTrust": { "enabled": false }
  }
}
JSON

# Record the effective wiring into the trace for the record, same as the other
# tools do, without ever writing the key itself.
cat >/trace/config.env <<CFG
GOOGLE_GEMINI_BASE_URL=${GOOGLE_GEMINI_BASE_URL}
model=${LAB_MODEL}
CFG
cp "$HOME/.gemini/settings.json" /trace/settings.json 2>/dev/null || true

# -p is gemini-cli's headless one-shot mode: a single prompt, then exit.
# --approval-mode yolo auto-approves every tool call so the agent can act without
# stopping for an approval; the container is the sandbox, so this is gemini-cli's
# equivalent of tomo's all-allow policy. --skip-trust trusts the workspace so yolo
# is not downgraded. -m pins the shared model, which rides in the request URL and
# is what the proxy reads to translate the call.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  gemini -m "${LAB_MODEL}" --approval-mode yolo --skip-trust -p "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
