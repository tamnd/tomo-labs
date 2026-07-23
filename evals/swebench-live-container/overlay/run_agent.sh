#!/usr/bin/env bash
# In-container agent driver. Runs INSIDE the prebuilt SWE-bench-Live instance
# image, so the agent iterates against the task's real environment (its own
# Python, its own installed deps) at /testbed. Mounts:
#   /scenario  read-only: prompt.txt, base_commit   (NO gold/test patch here)
#   /trace     writable: stdout/stderr/time/config/exit_code/model.patch
# Env: LAB_BASE_URL, LAB_MODEL, OPENCODE_API_KEY.
set -uo pipefail
TOOL="${1:?tool: tomo|opencode|pi|codex}"
prompt="$(cat /scenario/prompt.txt)"
export HOME=/root
cd /testbed
git config --global --add safe.directory /testbed 2>/dev/null || true

# ---- anti-leak: make the gold fix unrecoverable from the working tree ----
# Drop every remote (no `git fetch` of the PR) and delete every ref/branch/tag
# beyond base_commit, then detach HEAD at base_commit and expire the reflog, so
# `git log/show/branch/tag/stash` cannot surface the solution. github is also
# DNS-blackholed at the docker layer (see run_task.sh --add-host).
BASE="$(cat /scenario/base_commit 2>/dev/null || git rev-parse HEAD)"
for r in $(git remote 2>/dev/null); do git remote remove "$r" 2>/dev/null; done
git checkout -q --detach "$BASE" 2>/dev/null || true
git for-each-ref --format='%(refname)' 2>/dev/null | while read -r ref; do git update-ref -d "$ref" 2>/dev/null; done
: > .git/packed-refs 2>/dev/null || true
git reflog expire --expire=now --all 2>/dev/null || true
git gc --prune=now --quiet 2>/dev/null || true

run(){
  if [ -x /usr/bin/time ]; then
    /usr/bin/time -v -o /trace/time.txt "$@" </dev/null >/trace/stdout.log 2>/trace/stderr.log
  else
    "$@" </dev/null >/trace/stdout.log 2>/trace/stderr.log
  fi
  echo $? >/trace/exit_code
}

case "$TOOL" in
  tomo)
    case "$LAB_MODEL" in */*) m="$LAB_MODEL";; *) m="opencode/$LAB_MODEL";; esac
    cat >/trace/config.yaml <<YAML
default_model: ${m}
data_dir: /root/.tomo
workspace: /testbed
providers:
  opencode:
    type: openai
    api_key: \${OPENCODE_API_KEY}
    base_url: ${LAB_BASE_URL}
policy:
  read: allow
  net: allow
  write: allow
  exec: allow
sandbox: none
YAML
    # engine: default agent engine unless TOMO_ENGINE overrides (e.g. oi)
    eng=""; [ -n "${TOMO_ENGINE:-}" ] && eng="--engine ${TOMO_ENGINE}"
    echo "engine=${TOMO_ENGINE:-agent}" > /trace/engine
    run tomo --config /trace/config.yaml --yolo $eng -p "$prompt"
    ;;
  opencode)
    mkdir -p "$HOME/.config/opencode"
    cat >"$HOME/.config/opencode/opencode.json" <<JSON
{ "\$schema": "https://opencode.ai/config.json",
  "provider": { "lab": { "npm": "@ai-sdk/openai-compatible", "name": "lab",
    "options": { "baseURL": "${LAB_BASE_URL}", "apiKey": "${OPENCODE_API_KEY}" },
    "models": { "${LAB_MODEL}": { "name": "${LAB_MODEL}" } } } } }
JSON
    cp "$HOME/.config/opencode/opencode.json" /trace/config.json 2>/dev/null || true
    run opencode run --model "lab/${LAB_MODEL}" --dir /testbed --auto "$prompt"
    ;;
  pi)
    mkdir -p "$HOME/.pi/agent"
    cat >"$HOME/.pi/agent/models.json" <<JSON
{ "providers": { "lab": { "baseUrl": "${LAB_BASE_URL}", "api": "openai-completions",
   "apiKey": "\$OPENCODE_API_KEY", "models": [ { "id": "${LAB_MODEL}", "name": "${LAB_MODEL}" } ] } } }
JSON
    cp "$HOME/.pi/agent/models.json" /trace/config.json 2>/dev/null || true
    run pi -p "$prompt" --model "lab/${LAB_MODEL}" -a
    ;;
  codex)
    # codex speaks the Responses wire natively; point it at the lab provider,
    # which is the usage proxy in front of the subscription bridge. The bridge
    # holds the real ChatGPT token, so OPENCODE_API_KEY here is a placeholder the
    # bridge overwrites. Model and effort are pinned bridge-side too.
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
    run codex exec --sandbox danger-full-access --skip-git-repo-check "$prompt"
    ;;
  claude)
    # claude-code speaks the Anthropic Messages wire. Point it at the lab
    # endpoint (LAB_BASE_URL, an Anthropic-compatible gateway) via the env vars
    # the CLI honors; the key rides in ANTHROPIC_AUTH_TOKEN. --dangerously-skip-
    # permissions runs it non-interactively, matching codex's danger-full-access.
    export ANTHROPIC_BASE_URL="${LAB_BASE_URL%/v1}"
    export ANTHROPIC_AUTH_TOKEN="${OPENCODE_API_KEY}"
    export ANTHROPIC_MODEL="${LAB_MODEL}"
    export DISABLE_TELEMETRY=1 DISABLE_AUTOUPDATER=1 CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
    cat >/trace/config.json <<JSON
{ "base_url": "${LAB_BASE_URL%/v1}", "model": "${LAB_MODEL}" }
JSON
    run claude -p "$prompt" --model "$LAB_MODEL" --dangerously-skip-permissions
    ;;
  *) echo "unknown tool $TOOL" >&2; exit 2;;
esac

# ---- capture the tool's OWN native session store, for audit ----
# Each agent records its own complete session log under $HOME (codex writes a
# rollout jsonl, pi/opencode keep their own history). We copy that native store
# out under /trace/native/ so a downstream audit can diff our reconstructed
# session.jsonl against the tool's ground truth and prove nothing is missing.
# It is never published; only the reconstructed session.jsonl is. Best-effort,
# so a tool that keeps no store simply yields an empty native/ dir.
capture_native(){
  local dst="/trace/native"; mkdir -p "$dst"
  case "$TOOL" in
    codex)    [ -d "$HOME/.codex/sessions" ] && cp -r "$HOME/.codex/sessions" "$dst/codex-sessions" 2>/dev/null || true ;;
    pi)       [ -d "$HOME/.pi" ]      && cp -r "$HOME/.pi"      "$dst/pi" 2>/dev/null || true ;;
    opencode)
      # opencode persists the run to a SQLite database; dump its message+part
      # tables to JSONL in creation order (one {role, part} object per line) so
      # the audit reads plain JSON. The quoted heredoc keeps the shell out of the
      # SQL's $.role json path.
      db="$HOME/.local/share/opencode/opencode.db"
      if [ -f "$db" ]; then
        mkdir -p "$dst/opencode"
        sqlite3 -readonly "$db" >"$dst/opencode/session.jsonl" 2>/dev/null <<'SQL' || true
select json_object('role', json_extract(m.data,'$.role'), 'part', json(p.data))
from part p join message m on p.message_id = m.id
order by p.time_created, p.id;
SQL
      fi ;;
    claude)   [ -d "$HOME/.claude/projects" ] && cp -r "$HOME/.claude/projects" "$dst/claude" 2>/dev/null || true ;;
    tomo)     [ -d "$HOME/.tomo" ] && cp -r "$HOME/.tomo" "$dst/tomo" 2>/dev/null || true ;;
  esac
}
capture_native

# ---- capture model_patch: tracked edits vs base_commit, tool data dirs excluded ----
git -C /testbed diff HEAD -- . ':(exclude).tomo' ':(exclude).opencode' ':(exclude).pi' ':(exclude).codex' ':(exclude).claude' > /trace/model.patch 2>/dev/null || true
wc -l < /trace/model.patch > /trace/model.patch.lines 2>/dev/null || true
exit 0
