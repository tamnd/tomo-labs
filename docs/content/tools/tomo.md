---
title: "tomo"
description: "tomo is the harness author's own personal AI agent, a single Go binary driven headless with tomo -p and speaking native chat-completions."
weight: 10
---

tomo (友) is a personal AI agent that runs as one Go binary from the module `github.com/tamnd/tomo`.
It is the author's own agent, and the lab was built around it as the reference point before the other tools were wired in.
The lab drives it through a small adapter that runs `tomo -p "<prompt>"` once per scenario, points its model base URL at the trace proxy, and grades whatever it leaves in `/work`.
Its wire is native chat-completions, so the proxy records and forwards its requests without translating a dialect.

## What it is

tomo is a single self-contained binary.
The Dockerfile installs it with `go install github.com/tamnd/tomo/cmd/tomo`, so the tool image does not depend on any checkout of tomo on the host.
It presents as a personal agent that lives on the user's own machine and talks over a chat channel, which the captured system prompt states in its first line.
It carries nine tools in the versions the lab captured, covering shell, file read and write, web fetch, time, an in-context plan tool, two memory tools, and a skill reader.
The repo may be private, so this page sticks to what the trace, the adapter, the Dockerfile, and the recovered prompt show, and does not claim features it cannot see.

## Command surface

The lab uses one entry point, the headless one-shot mode.
`tomo -p "<prompt>"` runs a single task non-interactively and exits, instead of opening the chat REPL.

```bash
tomo -p "Hi!"
```

The adapter passes a config file explicitly rather than relying on a discovered one.

```bash
tomo --config /trace/config.yaml -p "$prompt"
```

Passing the whole task as one argument to `-p` keeps a multi-line prompt as a single turn.
The adapter notes that the chat REPL reads stdin line by line, which would fragment a multi-line prompt into one turn per line, and `-p` avoids that.
Standard input is left for approval answers, one `y` line per allowed escalation.

The flags this page can verify from the adapter and Dockerfile are `-p` for the one-shot prompt and `--config` for the config path.
Other subcommands and flags are not exercised by the lab, so they are not documented here.

The config file itself is where model, workspace, policy, and turn budget are set, covered in the next section.

## How the lab drives it

The tomo-specific glue is a single adapter script that is the container entrypoint.
Everything upstream of it, the network, the trace capture, and the resource accounting, is the same for every tool.

The harness mounts three paths into the container.
`/work` is the scenario's working tree, writable, and the agent's cwd.
`/scenario` is the read-only scenario definition, holding `prompt.txt` and an optional `approvals` file.
`/trace` is where stdout, the rendered config, and the time report land.
It also passes `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

The adapter reads the prompt from `/scenario/prompt.txt`.
It reads an approval budget from `/scenario/approvals`, defaulting to `0`, because most scenarios never fetch outside content and so never trip an escalation.
tomo's gate escalates a write or an exec to an approval prompt once the session has pulled in outside content, and headless those prompts are answered from this fixed budget.

tomo names a model as `provider/model`.
`LAB_MODEL` is the bare upstream id, so the adapter qualifies it with a provider name unless it already carries a slash.

```bash
case "$LAB_MODEL" in
  */*) model="$LAB_MODEL" ;;
  *)   model="opencode/$LAB_MODEL" ;;
esac
```

It then renders `/trace/config.yaml`.

```yaml
default_model: ${model}
data_dir: /work/.tomodata
workspace: /work
providers:
  opencode:
    type: openai
    api_key: ${OPENCODE_API_KEY}
    base_url: ${LAB_BASE_URL}
agent:
  max_turns: ${LAB_MAX_TURNS:-12}
policy:
  read: allow
  net: allow
  write: allow
  exec: allow
sandbox: none
```

The provider is declared as `type: openai` with `base_url` set to `LAB_BASE_URL`, which is the trace proxy.
That is how tomo's traffic reaches the proxy instead of the real upstream: it thinks it is talking to an OpenAI-shaped endpoint, and the proxy sits at that address.
`max_turns` is pinned to `LAB_MAX_TURNS` with a default of `12`, because the turn budget is the one budget every tool shares.
There is no `max_tokens` in the config on purpose, so tomo uses its own default output budget rather than being capped below what a shipped tomo would run with.
The policy allows read, net, write, and exec, and `sandbox: none`, because the throwaway container is already the sandbox.

Approvals are fed on stdin as `y` lines, one per allowed escalation.

```bash
feed() {
  local i
  for ((i = 0; i < approvals; i++)); do echo y; done
}
```

The adapter pins the agent to `/work` in two ways.
The config sets `workspace: /work`, which roots the file and shell tools there for a tomo that supports the setting.
It also sets `HOME=/work` and symlinks `/home/user` and `/home/agent` to `/work`, a fallback for an older binary that predates the workspace setting and would otherwise invent a home to write in.

The run itself is wrapped in GNU time for the resource numbers.

```bash
/usr/bin/time -v -o /trace/time.txt \
  tomo --config /trace/config.yaml -p "$prompt" \
  < <(feed) >/trace/stdout.log 2>/trace/stderr.log
```

stdout goes to `/trace/stdout.log`, which is the reply the checker and this page read, and stderr goes to `/trace/stderr.log`.

The image pins a specific version.
The Dockerfile builds on `golang:1.26-bookworm` and installs `ARG TOMO_VERSION=v0.2.4-0.20260711031055-906bc4beb933` with `CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}`.
The result is copied onto the shared `tomolab-base` image alongside the adapter, which is the entrypoint.

## Architecture

tomo runs an agent loop bounded by `max_turns`.
The loop sends the conversation to the model, and when the model asks for a tool, tomo runs it and feeds the result back, until the model answers without a tool call or the turn budget runs out.
For the 00-hello run the loop made exactly one model call and zero tool calls, which the trace records as `model_calls: 1` and `tool_calls: 0`.

Tool calls go over the model's native function-calling.
Each request carries the nine tool schemas, and the model picks one by name.
The tools the trace names are `shell`, `read_file`, `write_file`, `fetch`, `time`, `plan`, `memory_write`, `memory_read`, and `skill_read`.

Planning is in-context rather than a separate orchestrator.
The `plan` tool is one of the nine, and the agent prompt directs tomo to call it first when a task has three or more distinct steps, lay out the steps, work through them in the same turn, and call `plan` again to mark each one done.
A one or two step request skips the plan and just runs.
There is also a separate planner prompt captured once, tomo's planner, which turns a job into a JSON array of steps with goals, deps, inputs, an executor per step, and a mechanical postcondition.
This is the side prompt in the prompt page, and it appears in only one captured request, so the common path is the in-context plan tool, not the JSON planner.

tomo speaks native chat-completions.
The prompt page tags every tomo prompt as wire `chat`, and the how-it-works proxy normalizes each request to the chat-completions shape before recording it.
Because tomo already sends that shape, the proxy does not shim its dialect; it tees a copy into the trace and forwards it upstream.
The 00-hello trace confirms the path is `/zen/v1/chat/completions`, a plain chat call with no dialect translation tag.

## System prompt

The [prompts/tomo](/prompts/tomo/) page holds the verbatim text the proxy captured.
It was recovered with `lab prompts tomo` across 31 captured runs, and it is the exact text that reached the model, not a copy from tomo's source.
Because tomo's repo may be private, this recovered trace is the ground truth for what the prompt says.

The proxy captured four distinct prompts, all on wire `chat`.
Three are agent prompts and one is a side prompt for the planner.
The agent prompts run 935, 1402, and 643 chars, and the planner is 900 chars.
The 935-char and 643-char agent prompts and the planner all appear on 2026-07-10, and the 1402-char agent prompt on 2026-07-11, so the differences track tomo versions rather than a per-run toggle.
The 935 and 1402 char prompts advertise nine tools; the 643-char prompt advertises eight and drops `plan`.

The prompt opens by fixing tomo's identity and register.

```text
You are tomo (友), a personal AI agent that lives on your user's own machine.
You are talking with your user over a chat channel. Be direct, warm, and brief; this is a conversation, not a report.
```

It sets a hard rule against making things up.

```text
Never invent facts about the user's machine, files, or accounts: look them up or say you do not know.
```

The larger prompts spell out planning and verification.
The planning paragraph is what drives the plan tool.

```text
When a task has three or more distinct steps, call the plan tool first to lay out the steps, then work through them in this same turn, calling plan again to mark each done. Keep the whole job in one turn: do not stop until it is finished.
```

The 1402-char prompt adds a verify-before-done rule that the shorter prompts do not carry.

```text
When you write or change code, verify it before you say it is done: run the project's tests or build with the shell tool, read the output, and if it fails, fix the code and run again until it passes.
```

Every prompt roots the work in `/work` and closes with a volatile date line, "Today is Friday, 2026-07-10." or "Today is Saturday, 2026-07-11.", which changes per run and is worth ignoring when comparing prompt text.
This section is recovered from traces, not copied from tomo's source, so it reflects what tomo actually sent.

## Hi! end to end

The 00-hello scenario hands tomo the single prompt `Hi!` and checks that a greeting round trip completes.

The adapter reads `Hi!` from `/scenario/prompt.txt` and runs `tomo --config /trace/config.yaml -p "Hi!"`.
tomo builds the request around it: a system message with the agent prompt, whose first line is "You are tomo (友), a personal AI agent that lives on your user's own machine.", then the user message `Hi!`.
The request also carries the nine tool schemas (`shell`, `read_file`, `write_file`, `fetch`, `time`, `plan`, `memory_write`, `memory_read`, `skill_read`), sent on the native chat wire.

At the proxy the request lands as a POST to `/zen/v1/chat/completions` with model `deepseek-v4-flash-free`.
The proxy forces greedy decoding, so the body shows `temperature=0`, `top_p=1`, `seed=7`, and `stream=True`.
It is a plain chat-completions call with two messages, roles system and user, and no dialect translation.
The full proxy tap for the run is two records, a `GET /zen/` health touch and the one `POST /zen/v1/chat/completions`.

That one request gets one upstream completion, and tomo answers without calling a tool.
The trace records `requests: 2`, `orchestration.model_calls: 1`, `tool_calls: 0`, `plan_calls: 0`, and `planned: false`.
Tokens were 1550 prompt, 35 completion, 1585 total.
Latency was 704 ms to first byte and 1545 ms total, over one timed completion.

The reply that reached the user, from `stdout.log` at 39 bytes, is:

```text
Hey! Good to see you. What's going on?
```

The checker graded the run a pass.
It never reads the model's prose; it confirms the greeting round trip completed, records `check: "baseline greeting round trip completed"`, and marks `passed: true` on the first attempt.
The run also logged an install footprint of 22178 KB and a peak RSS of 12572 KB.
