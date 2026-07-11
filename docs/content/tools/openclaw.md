---
title: "openclaw"
description: "A self-hosted personal-assistant CLI installed from npm, wired into the lab as its own tool and measured through its native OpenAI chat-completions provider."
weight: 50
---

openclaw is a self-hosted personal AI assistant that ships as an npm package and runs from a single `openclaw` command.
The lab treats it as one tool among many: a small `adapter.sh` points it at the trace proxy, runs one scenario headless, and leaves the result in `/work` for the checker to grade.
Everything on this page is grounded in the captured artifacts, the Dockerfile, `adapter.sh`, the recovered system prompt, and one `00-hello` trace, and where a detail is not in those artifacts this page says so.
openclaw speaks native OpenAI chat-completions on the wire, so the proxy forwards it without translating a dialect.

## Overview

openclaw installs with `npm install -g openclaw@2026.7.1-beta.2`, the version pinned by the Dockerfile `ARG`.
It runs on the shared `tomolab-base` image, which already carries Node 22, the runtime openclaw needs.
Its purpose in the lab is narrow: drive one scenario to completion non-interactively and write files the checker can inspect.
The wider project is a personal assistant with messaging channels, a Gateway control plane, memory files, and a skills system, but the lab exercises only the file work the agent leaves behind.

The two headline features that set openclaw apart from a plain single-loop agent both show up in its own prompt and tool schema:

- A dedicated **plan tool**, `update_plan`, plus a goal family (`create_goal`, `get_goal`, `update_goal`).
- A whole **subagent layer**: `sessions_spawn`, `sessions_yield`, `sessions_list`, `sessions_history`, `sessions_send`, and `subagents` for spawning and coordinating isolated child sessions.

Both exist in every run, but the sweep shows the agent rarely reaches for them.
Across the 14-scenario sweep openclaw invoked its plan tool in only 1 scenario, and only when the prompt explicitly asked for a live plan.
In the `00-hello` run it planned nothing and spawned no subagents: `plan_calls: 0`, `subagents: 0`, `planned: false`.

At a glance:

| Field | Value |
| --- | --- |
| Runtime | Node 22 CLI, run in a podman container from `tomolab-base` |
| Install source | npm, `npm install -g openclaw@${OPENCLAW_VERSION}` |
| Version captured | `2026.7.1-beta.2` (Dockerfile `ARG OPENCLAW_VERSION`); config meta reports `lastRunVersion 2026.6.11` internally |
| Wire dialect | OpenAI chat-completions (prompt tag `wire chat`); `POST /v1/chat/completions` |
| How the lab invokes it | `openclaw agent --local --session-key run --json --timeout <n> --message "$prompt"` |
| Where it writes | `/work`, pinned via `agents.defaults.workspace` |
| Provider api | `openai-completions`, baseUrl is the trace proxy |
| Forced decoding | `temperature=0`, `top_p=1`, `seed=7`, `stream=true` |

The tool set advertised in the recovered prompt is 24 tools, grouped by what they do:

| Group | Tools | What they cover |
| --- | --- | --- |
| Files | `read`, `write`, `edit`, `apply_patch` | Read, create, edit; `apply_patch` applies multi-file patches in one call |
| Shell | `exec`, `process` | Run commands (pty available); `process` manages background exec sessions |
| Planning | `update_plan`, `create_goal`, `get_goal`, `update_goal` | The plan tool and a goal family |
| Subagents | `sessions_spawn`, `sessions_yield`, `sessions_list`, `sessions_history`, `sessions_send`, `subagents` | Spawn isolated children, wait on push-based completion, inspect and message sessions |
| Memory | `memory_search`, `memory_get` | Scan `MEMORY.md`, `memory/*.md`, indexed transcripts; pull only needed lines |
| Web | `web_search`, `web_fetch` | Search and fetch readable page content |
| Other | `cron`, `image`, `session_status`, `skill_workshop` | Scheduling, image analysis, status card, the skills workshop |

Only `read` is the tool that fires nothing in the hello run: that scenario made 1 model call and 0 tool calls.
The rest are schema surface openclaw carries into every request but did not need for a greeting.

## Say Hi!

The `00-hello` scenario sends one greeting and checks that a baseline round trip completed.
Here is that run end to end, from the trace at `20260710T134428Z`.

**1. The adapter reads the prompt.**
`adapter.sh` is the container entrypoint.
It reads the scenario text straight off the mounted file:

```bash
prompt="$(cat /scenario/prompt.txt)"
```

For `00-hello` that text is `Hi!`.

**2. It points openclaw at the proxy.**
The adapter lays down a baseline config, then registers an OpenAI-compatible provider named `lab` whose `baseUrl` is the trace proxy:

```bash
openclaw setup --non-interactive --accept-risk
provider=$(cat <<JSON
{"api":"openai-completions","baseUrl":"${LAB_BASE_URL}","apiKey":"${OPENCODE_API_KEY}","models":[{"id":"${LAB_MODEL}","name":"${LAB_MODEL}","contextWindow":131072,"input":["text"]}]}
JSON
)
openclaw config set models.providers.lab "$provider" --strict-json
openclaw models set "lab/${LAB_MODEL}"
openclaw config set agents.defaults.workspace /work
openclaw exec-policy preset yolo
```

The rendered config confirms it took: `models.providers.lab.api` is `openai-completions`, `baseUrl` is `http://tomolab-proxy:8080/v1`, and `agents.defaults.workspace` is `/work`.
`exec-policy preset yolo` sets `tools.exec` to `security=full, ask=off`, so the agent acts without approval prompts.

**3. It runs openclaw once, non-interactively, under `time`.**
Working directory is `/work`, and `/usr/bin/time -v` captures peak memory:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  openclaw agent --local --session-key run --json \
    --timeout "$((LAB_MAX_TURNS * 120))" \
    --message "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

`--local` runs the agent inline rather than through a Gateway daemon, `--session-key run` fixes the session name, and `--json` makes it emit a structured result on stdout.

**4. openclaw builds its request.**
The proxy tap recorded 2 records: a `GET /zen/` warmup and one `POST /zen/v1/chat/completions`.
That single completion carries the full agent context:

- a system message of 31,396 chars, first line `You are a personal assistant running inside OpenClaw.`
- a user message, exactly `[Fri 2026-07-10 13:44 UTC] Hi!` (openclaw stamps the local time onto the greeting)
- all 24 tools, with `tool_choice: auto`
- `max_completion_tokens: 8192`, openclaw's own cap

**5. The proxy pins decoding and forwards.**
The recorded body carries the lab's forced-determinism knobs, the same for every tool: `temperature=0`, `top_p=1`, `seed=7`, `stream=true`.
Because the provider api is `openai-completions`, the proxy forwards the request as-is, tees the streamed response into `resp-2.txt`, and records token usage into `usage.jsonl` without any dialect translation.

**6. One completion, zero tool calls.**
The model answered in a single call and reached for no tools.
The streamed response even carried `reasoning_content` deltas (64 reasoning tokens) before the visible text.

Trace numbers for the run:

| Metric | Value |
| --- | --- |
| passed | `true` |
| attempts | 1 of 3 max |
| requests | 2 (`GET /zen/`, `POST /zen/v1/chat/completions`) |
| model_calls | 1 |
| tool_calls | 0 |
| plan_calls | 0 |
| subagents | 0 |
| planned | `false` |
| prompt tokens | 16,637 |
| completion tokens | 118 |
| total tokens | 16,755 |
| cached tokens | 7,552 |
| avg ttfb | 1,163 ms |
| avg total | 3,632 ms |
| peak RSS | 375,508 kb (about 367 MB) |
| install footprint | 417,267 kb (about 407 MB) |
| wall time | 33 s |

**7. The reply arrived as structured JSON payloads.**
Because of `--json`, `stdout.log` is not raw text but a payload envelope: a `payloads` array of `{text, mediaUrl}` plus a `meta` block.
The greeting text, verbatim:

```text
Hey 👋

I just came online. Fresh workspace, no memory yet — literally nothing but what's in those files. So let's figure this out together.

Who are you? And... who do you want me to be? Got a name in mind?
```

That is not a generic hello.
It follows the `BOOTSTRAP.md` first-run script that the prompt injects, which is why the agent asks who it should become.

**8. The checker grades a pass.**
`result.json` records the check note `baseline greeting round trip completed`, and the run passed on the first attempt.

## Architecture

This section is enough to reimplement the openclaw adapter from scratch.
openclaw is the reference example for adding a tool to the lab, so the moving parts are called out concretely.

### The container

The image is two files on top of the shared base:

```dockerfile
FROM tomolab-base
ARG OPENCLAW_VERSION=2026.7.1-beta.2
RUN npm install -g openclaw@${OPENCLAW_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

Because openclaw comes from npm, the image is independent of any host checkout.
`tomolab-base` supplies Node 22 and the shared toolchain, so openclaw builds and runs Go, Node, and make the same way every other tool does.
The entrypoint is `adapter.sh`, renamed `adapter`.

### Mounts

| Mount | Mode | Holds |
| --- | --- | --- |
| `/work` | writable | The scenario working tree and the agent's cwd; what the checker grades |
| `/scenario` | read-only | The scenario definition, `prompt.txt` |
| `/trace` | writable | `stdout.log`, `stderr.log`, `config.json`, `config.path`, `setup.log`, `time.txt`, `exit_code` |

### Harness environment

| Variable | Meaning |
| --- | --- |
| `LAB_BASE_URL` | The trace proxy URL, becomes the provider `baseUrl` |
| `LAB_MODEL` | The model id, e.g. `deepseek-v4-flash-free` |
| `OPENCODE_API_KEY` | The real upstream key; the proxy forwards with it |
| `LAB_MAX_TURNS` | Turn budget; the adapter turns it into a timeout of `LAB_MAX_TURNS * 120` seconds |

### The adapter, step by step

The adapter is the only openclaw-specific glue.
Everything upstream of it, network, trace capture, resource accounting, is identical for every tool.

Read the task and lay down a baseline workspace:

```bash
prompt="$(cat /scenario/prompt.txt)"
openclaw setup --non-interactive --accept-risk >/trace/setup.log 2>&1
```

`--accept-risk` skips the interactive safety warning a headless run cannot answer.

Point openclaw's `base_url` at the proxy by registering a provider, then make it active:

```bash
openclaw config set models.providers.lab "$provider" --strict-json >>/trace/setup.log 2>&1
openclaw models set "lab/${LAB_MODEL}" >>/trace/setup.log 2>&1
```

The provider JSON sets `"api":"openai-completions"`, `"baseUrl":"${LAB_BASE_URL}"`, `"apiKey":"${OPENCODE_API_KEY}"`, and one model with `"contextWindow":131072` and `"input":["text"]`.
`--strict-json` tells `config set` the value is a literal JSON document.

Pin the workspace so openclaw acts on the exact tree the checker grades:

```bash
openclaw config set agents.defaults.workspace /work >>/trace/setup.log 2>&1
```

By default openclaw works in `~/.openclaw/workspace`, which the harness never grades; moving it to `/work` puts openclaw on the same footing as tomo, whose cwd is `/work`.

Drop approvals so the agent can act:

```bash
openclaw exec-policy preset yolo >>/trace/setup.log 2>&1
```

The container is the sandbox, so this is openclaw's equivalent of tomo's all-allow policy, and it lets the shell scenarios run headless.

Record the effective config for the trace, then run the agent from `/work` under GNU `time`:

```bash
openclaw config file >/trace/config.path 2>/dev/null || true
cp "$HOME/.openclaw/openclaw.json" /trace/config.json 2>/dev/null || true

cd /work
/usr/bin/time -v -o /trace/time.txt \
  openclaw agent --local --session-key run --json \
    --timeout "$((LAB_MAX_TURNS * 120))" \
    --message "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?
echo "$status" >/trace/exit_code
exit 0
```

`HOME` and cwd both resolve to the config and workspace the adapter set up, so the model reads a system prompt whose `## Workspace` line already says `/work`.
The wrapper captures stdout (the JSON envelope), stderr, the exit code, and peak memory in one shot.

### How openclaw reaches the proxy

openclaw thinks it is calling an ordinary OpenAI-compatible endpoint.
`LAB_BASE_URL` is the trace proxy, so every request, response, and token count is captured with no cooperation from openclaw, and the proxy forwards to the real upstream with the real key.
Nothing in openclaw is patched or instrumented; the interception is entirely in the provider `baseUrl`.

### The agent loop

`openclaw agent --local` runs the loop inline, bounded by `--timeout` (derived from `LAB_MAX_TURNS`).
Within a turn the model can call any of the 24 native tools; openclaw executes them and feeds results back until the model produces a final visible reply, which `--json` renders as the `payloads` envelope.

Two capabilities are worth calling out because they distinguish openclaw:

- **The plan tool.** `update_plan`, with the goal family, lets the agent record a plan and track goals across a task. It is present in every request but the sweep shows it used in only 1 of 14 scenarios, and only when the prompt asks for a live plan.
- **The subagent layer.** `sessions_spawn` starts an isolated child (`context:"fork"` only when the child needs the current transcript), and `sessions_yield` ends the turn and waits on push-based completion events. The prompt is explicit that the agent should not poll `subagents list` or `sessions_list` in a loop. No subagents were spawned in the hello run.

### The wire dialect

openclaw's wire dialect is chat, tagged `wire chat` on the prompt page.
It sends a standard OpenAI chat-completions body: `messages`, `tools`, `tool_choice`, `temperature`, `top_p`, `seed`, `stream`, `max_completion_tokens`, `model`.
The proxy records the request, normalizes each completion to the chat-completions shape, tees the streamed SSE response, tallies usage, and forwards upstream.
Since the provider api is already `openai-completions`, there is no dialect translation: the recorded request is the request openclaw sent, and the recorded decoding knobs (`temperature=0`, `top_p=1`, `seed=7`, `stream=true`) are what the lab pins for determinism across every tool.

## System Prompts

The prompt below is openclaw's OWN baked-in system prompt.
It was recovered verbatim from the trace proxy by `lab prompts openclaw`, not injected by the lab.
The proxy records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model.
Regenerate the prompt page with the same command; the file is versioned, so drift shows up in the diff when openclaw updates.

### What was captured

One distinct prompt was recovered, the agent prompt:

| Field | Value |
| --- | --- |
| Prompts captured | 1 (agent prompt) |
| Wire | `chat` |
| Size | 31,517 chars |
| Tools | 24 |
| Requests behind it | 96 |
| Per-run renderings | 19 |

Note that openclaw carries subagent and plan machinery, but only one prompt was captured.
The subagent orchestration and the plan tool do not surface as separately-recovered prompts; they live as sections inside this single agent prompt.
The 19 per-run renderings collapse into one prompt because they differ only in a volatile `Runtime` line.

### Identity and tool contract

The opener states what it is and how tools are named:

```text
You are a personal assistant running inside OpenClaw.
## Tooling
Available tools are policy-filtered. Names are case-sensitive; call exactly as listed.
```

Tools are then listed one per line with a short description each, which is where all 24 names come from.
A closing note draws the line between availability and guidance: `TOOLS.md is usage guidance, not availability.`

### Tool-use rules

The prompt tells the model when to narrate and how to handle approvals:

```text
## Tool Call Style
Routine low-risk calls: no narration.
Narrate only for complex, sensitive/destructive, or explicitly requested steps.
First-class tool exists: use it; do not ask user to run equivalent CLI/slash command.
```

The execution bias pushes the agent to act rather than promise:

```text
## Execution Bias
- Actionable request: act in this turn.
- Continue until done or genuinely blocked; do not finish with a plan/promise when tools can move it forward.
- Weak/empty tool result: vary query, path, command, or source before concluding.
- Final answer needs evidence: test/build/lint, screenshot, inspection, tool output, or a named blocker.
```

### The plan tool

Planning has thin narrative in the prompt.
`update_plan` is listed among the tools with no dedicated instruction block, and the goal family (`create_goal`, `get_goal`, `update_goal`) is listed the same way.
There is no section that tells the model to draft a plan before acting, which lines up with the sweep result that the plan tool fired in only 1 of 14 scenarios.
Planning is available, not encouraged.

### Subagent orchestration

Delegation, by contrast, gets explicit rules:

```text
Larger work: use `sessions_spawn`; completion is push-based.
`sessions_spawn`: omit `context` unless transcript needed; then set `context:"fork"`.
Do not poll `subagents list` / `sessions_list` in a loop; use `sessions_yield` when waiting for spawned sub-agent completion events, and check status only on-demand (for intervention, debugging, or when explicitly asked).
```

The same behavior is restated in the injected `Messaging` context: spawn with a clear objective, omit `context` for isolated children, and use `sessions_yield` to wait.

### Safety

The safety block forbids independent goals and protects config and prompts:

```text
## Safety
No independent goals: no self-preservation, replication, resource acquisition, power-seeking, or long-term plans beyond the user's request.
Safety/oversight over completion. Conflicts: pause/ask. Obey stop/pause/audit; never bypass safeguards.
Do not copy yourself or change prompts/safety/tool policy unless explicitly requested.
```

### Workspace, memory, and bootstrap

The workspace section is the adapter's `agents.defaults.workspace /work` showing up in the text the model reads:

```text
## Workspace
Your working directory is: /work
```

A memory-recall section tells the model to run `memory_search` over `MEMORY.md` and `memory/*.md` before answering anything about prior work, and to cite `Source: <path#line>` when it helps.
A bootstrap block is what produced the hello greeting:

```text
## Bootstrap Pending
BOOTSTRAP.md is included below in Project Context; follow it before replying normally.
Do not use a generic first greeting or reply normally until after you have handled BOOTSTRAP.md.
Your first user-visible reply for a bootstrap-pending workspace must follow BOOTSTRAP.md, not a generic greeting.
```

The prompt then injects the whole workspace: `AGENTS.md`, `SOUL.md`, `IDENTITY.md`, `USER.md`, `TOOLS.md`, `BOOTSTRAP.md`, and `HEARTBEAT.md` as project context, plus 13 named skills under `/usr/lib/node_modules/openclaw/skills`.

### Formatting directives

Output directives govern attachments and silent replies:

```text
## Assistant Output Directives
- Attach media in the final visible reply with `MEDIA:<path-or-url>` on its own line.
## Silent Replies
When you have nothing to say, respond with ONLY: NO_REPLY
```

### Volatile spans

When diffing this prompt across runs, ignore the `Runtime` line near the end, which carries a session uuid and a host id that change every run:

```text
Runtime: agent=main | session=agent:main:run | sessionId=965dc6af-6cea-4e16-af13-ecb23b1dbfc3 | host=01d5f8f35702 | repo=/work | os=Linux 7.0.12-201.fc44.aarch64 (arm64) | node=v22.23.1 | model=lab/deepseek-v4-flash-free | default_model=lab/deepseek-v4-flash-free | thinking=off
```

The `sessionId` and `host` are what differ across the 19 renderings; everything else is identical.
The stamped time in the user message (`[Fri 2026-07-10 13:44 UTC] Hi!`) is volatile too.

The full verbatim text is on the prompt page at [/prompts/openclaw/](/prompts/openclaw/).
