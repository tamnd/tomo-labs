#!/usr/bin/env bash
# adapter drives Codex through one scenario, non-interactively, and leaves the
# work it did in /work for the checker to grade. It is the only codex-specific
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

# Define a custom model provider named lab whose base_url is the trace proxy, so
# every request/response and its token usage get captured with no cooperation
# from codex. Current codex only speaks wire_api responses, and our free deepseek
# model is chat-only upstream, so the proxy translates responses to chat and back
# at its edge; codex talks its native wire and never knows. env_key names the
# environment variable codex reads the key from, and the proxy forwards to the
# real upstream with it. Config lives at $CODEX_HOME/config.toml, default ~/.codex.
mkdir -p "$HOME/.codex"
cat >"$HOME/.codex/config.toml" <<TOML
model = "${LAB_MODEL}"
model_provider = "lab"

[model_providers.lab]
name = "lab"
base_url = "${LAB_BASE_URL}"
env_key = "OPENCODE_API_KEY"
wire_api = "responses"
TOML
cp "$HOME/.codex/config.toml" /trace/config.toml 2>/dev/null || true

# exec is codex's headless one-shot mode: a single prompt, then exit, and it
# never stops for an approval. --sandbox danger-full-access drops the sandbox so
# the agent can act freely; the container is the sandbox, so this is codex's
# equivalent of tomo's all-allow policy. --skip-git-repo-check lets it run in
# /work, which is a plain tree rather than a git repo.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  codex exec --sandbox danger-full-access --skip-git-repo-check "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
