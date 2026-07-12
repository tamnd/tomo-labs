#!/usr/bin/env bash
# adapter drives tomo through one scenario, non-interactively, and leaves the
# work it did in /work for the checker to grade. It is the only tomo-specific
# glue in the lab: everything upstream of it (network, trace capture, resource
# accounting) is the same for every tool.
#
# It is the container entrypoint. The harness mounts:
#   /work      the scenario's working tree, writable, and the agent's cwd
#   /scenario  the scenario definition, read-only (prompt.txt, optional approvals)
#   /trace     where stdout, the rendered config, and the time report land
#
# and passes LAB_BASE_URL, LAB_MODEL, OPENCODE_API_KEY, LAB_MAX_TURNS.
set -uo pipefail

prompt="$(cat /scenario/prompt.txt)"
# tomo's gate escalates a write or exec to an approval prompt once the session
# has pulled in outside content. Headless, we answer those from a fixed budget
# the scenario declares; most scenarios declare zero because they never fetch.
approvals=0
[ -f /scenario/approvals ] && approvals="$(tr -dc '0-9' </scenario/approvals)"
: "${approvals:=0}"

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
  # per-rep wall-clock timeout the harness already imposes is the only bound.
policy:
  read: allow
  net: allow
  write: allow
  exec: allow
sandbox: none
YAML

# stdin: just the approval budget as y lines. The task itself goes in as one
# argument to `tomo -p`, so a multi-line prompt stays a single turn instead of
# fragmenting into one turn per line the way the chat REPL reads it. Any gate
# that escalates mid-run reads its answer from these lines.
feed() {
  local i
  for ((i = 0; i < approvals; i++)); do echo y; done
}

# Pin the agent to /work. The config above sets workspace: /work, so a tomo that
# supports it roots the file and shell tools there and tells the model as much.
# The symlinks are a belt-and-suspenders fallback for an older tomo binary that
# predates the workspace setting and would otherwise invent a home to write in;
# they can go once the released binary carries the feature.
export HOME=/work
mkdir -p /home
ln -sfn /work /home/user
ln -sfn /work /home/agent

cd /work
/usr/bin/time -v -o /trace/time.txt \
  tomo --config /trace/config.yaml -p "$prompt" \
  < <(feed) >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
