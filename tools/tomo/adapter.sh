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
cat >/trace/config.yaml <<YAML
default_model: ${LAB_MODEL}
data_dir: /work/.tomodata
providers:
  opencode:
    type: openai
    api_key: \${OPENCODE_API_KEY}
    base_url: ${LAB_BASE_URL}
agent:
  max_tokens: 2000
  max_turns: ${LAB_MAX_TURNS:-12}
policy:
  read: allow
  net: allow
  write: allow
  exec: allow
sandbox: none
YAML

# stdin: the task, then the approval budget as y lines, then /exit.
feed() {
  printf '%s\n' "$prompt"
  local i
  for ((i = 0; i < approvals; i++)); do echo y; done
  echo /exit
}

cd /work
/usr/bin/time -v -o /trace/time.txt \
  tomo chat --config /trace/config.yaml \
  < <(feed) >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
