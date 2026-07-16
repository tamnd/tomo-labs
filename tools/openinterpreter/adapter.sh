#!/usr/bin/env bash
# adapter drives Open Interpreter through one scenario, non-interactively, and
# leaves the work it did in /work for the checker to grade. It is the only
# oi-specific glue in the lab: everything upstream of it (network, trace capture,
# resource accounting) is the same for every tool.
#
# It is the container entrypoint. The harness mounts:
#   /work      the scenario's working tree, writable, and the agent's cwd
#   /scenario  the scenario definition, read-only (prompt.txt)
#   /trace     where stdout, the rendered config, and the time report land
#
# and passes LAB_BASE_URL, LAB_MODEL, OPENCODE_API_KEY, LAB_MAX_TURNS.
#
# Current OI is a Rust Codex fork driven by `interpreter exec`. It reads a
# codex-style config.toml for its model, provider, sandbox, and approval policy,
# so the adapter renders one that routes every request through the lab proxy and
# turns the container itself into the sandbox (no bwrap, no approval prompts).
set -uo pipefail

# Keep everything OI writes inside the throwaway tree: HOME for stray dotfiles,
# and INTERPRETER_HOME for its config, sessions, and logs. The self-contained
# binary lives at /usr/local/bin/interpreter and needs neither.
export HOME=/work
export INTERPRETER_HOME=/work/.openinterpreter
mkdir -p "$INTERPRETER_HOME"

# The provider's bearer token. OI reads it from the env var named by the
# provider's env_key below, so route the lab key there. Set OPENAI_* too so any
# path that falls back to the ambient OpenAI vars still hits the proxy.
export LAB_API_KEY="${OPENCODE_API_KEY}"
export OPENAI_API_BASE="${LAB_BASE_URL}"
export OPENAI_API_KEY="${OPENCODE_API_KEY}"

# The model slug the proxy expects. The old litellm driver prefixed openai/ to
# select its OpenAI handler; the Rust CLI names the model bare and lets the
# provider decide the endpoint, so strip that prefix if the harness added it.
model="${LAB_MODEL#openai/}"

# Render the config. A custom "lab" provider points OI's chat-completions wire at
# the proxy; sandbox danger-full-access and approval never make the container the
# only guardrail, matching the all-allow autonomy every rival gets. The harness
# is left unset so OI auto-selects by model family (a deepseek slug picks its
# deepseek-tui harness), which is OI as it ships; LAB_OI_HARNESS overrides it for
# a pinned-harness run.
cat >"$INTERPRETER_HOME/config.toml" <<EOF
model_provider = "lab"
model = "${model}"
sandbox_mode = "danger-full-access"
approval_policy = "never"
check_for_update_on_startup = false
harness_guidance = true
EOF
if [ -n "${LAB_OI_HARNESS:-}" ]; then
  echo "harness = \"${LAB_OI_HARNESS}\"" >>"$INTERPRETER_HOME/config.toml"
fi
cat >>"$INTERPRETER_HOME/config.toml" <<EOF

[model_providers.lab]
name = "Lab Proxy"
base_url = "${LAB_BASE_URL}"
env_key = "LAB_API_KEY"
wire_api = "chat"
EOF

# Keep a copy of the rendered config in the trace for debugging a run.
cp "$INTERPRETER_HOME/config.toml" /trace/config.toml

cd /work
# `interpreter exec -` reads the task from stdin and runs to completion with no
# TUI. --skip-git-repo-check lets it run in a plain working tree, --ephemeral
# skips a persisted session record, and -o writes the final assistant message for
# the checker to inspect. GNU time wraps it so the harness reads peak memory back
# the same way it does for every tool.
/usr/bin/time -v -o /trace/time.txt \
  interpreter exec - \
    --skip-git-repo-check \
    --sandbox danger-full-access \
    --ephemeral \
    -o /trace/final.txt \
    </scenario/prompt.txt >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
