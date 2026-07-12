#!/usr/bin/env bash
# adapter drives tomo through one scenario, non-interactively, and leaves the
# work it did in /work for the checker to grade. It is the only tomo-specific
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

# tomo runs with --yolo, its fully autonomous mode: every action the policy gate
# would escalate to an approval is auto-approved, so tomo acts unattended exactly
# like every rival (aider --yes-always, claude-code --dangerously-skip-permissions,
# copilot --allow, gemini-cli and hermes --yolo, kilocode and opencode --auto).
# Without it, tomo's taint guard escalates a write or exec once a session pulls in
# outside content, and headless there is no human to approve, so a run that
# fetched a page could no longer edit its own workspace. The container is the
# sandbox, so autonomy here is safe and is the fair equal of the other tools.

# A fully autonomous policy: the container is the sandbox, so let the agent act
# and measure whether it can finish the task. sandbox: none because we are
# already inside a throwaway container.
# tomo names a model as provider/model and sends the bare model id upstream.
# LAB_MODEL is the bare upstream id (openclaw sends it as-is), so qualify it with
# the provider name here. If the harness already handed us a qualified id, leave
# it alone.
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
  # same as every rival. The other adapters do not cap turns (opencode, codex,
  # aider, claude-code, copilot, gemini-cli, hermes, kilocode, pi ignore
  # LAB_MAX_TURNS; openclaw reads it only as a wall-clock timeout), so a turn
  # cap here would uniquely throttle tomo and cut real runs off mid-task. The
  # per-attempt wall clock the harness enforces (LAB_ATTEMPT_TIMEOUT) is the one
  # bound every tool shares, so a run that never converges is killed and graded
  # rather than left to burn.
policy:
  read: allow
  net: allow
  write: allow
  exec: allow
sandbox: none
YAML

# Pin the agent to /work. The config above sets workspace: /work, so a tomo that
# supports it roots the file and shell tools there and tells the model as much.
# The symlinks are a belt-and-suspenders fallback for an older tomo binary that
# predates the workspace setting and would otherwise invent a home to write in;
# they can go once the released binary carries the feature.
export HOME=/work
mkdir -p /home
ln -sfn /work /home/user
ln -sfn /work /home/agent

# The task goes in as one argument to `tomo -p`, so a multi-line prompt stays a
# single turn instead of fragmenting into one turn per line the way the chat REPL
# reads it. stdin is closed (</dev/null): with --yolo nothing prompts, so there
# is nothing to feed.
cd /work
/usr/bin/time -v -o /trace/time.txt \
  tomo --config /trace/config.yaml --yolo -p "$prompt" \
  </dev/null >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
