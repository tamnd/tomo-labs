#!/usr/bin/env bash
# adapter drives tomo on its oi engine through one scenario, non-interactively,
# and leaves the work it did in /work for the checker to grade. It is identical
# to the tomo adapter but for one flag, --engine oi, which selects the Open
# Interpreter code-as-action loop instead of the default one. Everything else
# (network, trace capture, resource accounting, the autonomous policy) is the
# same for every tool, so a tomo-oi result compares to tomo like for like: same
# binary, same tools, same model, only the turn loop and its prompt differ.
#
# It is the container entrypoint. The harness mounts:
#   /work      the scenario's working tree, writable, and the agent's cwd
#   /scenario  the scenario definition, read-only (prompt.txt)
#   /trace     where stdout, the rendered config, and the time report land
#
# and passes LAB_BASE_URL, LAB_MODEL, OPENCODE_API_KEY, LAB_MAX_TURNS.
set -uo pipefail

prompt="$(cat /scenario/prompt.txt)"

# tomo runs with --yolo, its fully autonomous mode: every action the policy gate
# would escalate to an approval is auto-approved, so tomo acts unattended exactly
# like every rival. The container is the sandbox, so autonomy here is safe and is
# the fair equal of the other tools.

# tomo names a model as provider/model and sends the bare model id upstream.
# LAB_MODEL is the bare upstream id, so qualify it with the provider name here. If
# the harness already handed us a qualified id, leave it alone.
case "$LAB_MODEL" in
  */*) model="$LAB_MODEL" ;;
  *)   model="opencode/$LAB_MODEL" ;;
esac

cat >/trace/config.yaml <<YAML
default_model: ${model}
data_dir: /work/.tomodata
workspace: /work
providers:
  opencode:
    type: openai
    api_key: \${OPENCODE_API_KEY}
    base_url: ${LAB_BASE_URL}
agent:
  # No max_tokens and no max_turns here: tomo runs to its own natural stop, the
  # same as every rival, so a turn cap does not uniquely throttle tomo. The
  # per-attempt wall clock the harness enforces is the one bound every tool
  # shares.
policy:
  read: allow
  net: allow
  write: allow
  exec: allow
sandbox: none
YAML

# Pin the agent to /work. The config sets workspace: /work; the symlinks are a
# fallback for an older binary that predates the workspace setting.
export HOME=/work
mkdir -p /home
ln -sfn /work /home/user
ln -sfn /work /home/agent

# The task goes in as one argument to `tomo -p`, so a multi-line prompt stays a
# single turn. --engine oi selects the code-as-action loop for this run; stdin is
# closed (</dev/null): with --yolo nothing prompts, so there is nothing to feed.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  tomo --config /trace/config.yaml --engine oi --yolo -p "$prompt" \
  </dev/null >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
