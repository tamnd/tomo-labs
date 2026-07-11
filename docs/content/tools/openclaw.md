---
title: "openclaw"
description: "A self-hosted personal-assistant CLI installed from npm, wired into the lab as its own tool and driven through its native chat-completions provider."
weight: 50
---

openclaw is a self-hosted personal AI assistant that ships as an npm package and runs from a single `openclaw` command.
Its public home is [github.com/openclaw/openclaw](https://github.com/openclaw/openclaw), with docs at [docs.openclaw.ai](https://docs.openclaw.ai), and the captured system prompt cites both of those URLs itself.
This page is grounded in the captured run: the adapter, the Dockerfile, the recovered prompt, and one `00-hello` trace are the source for everything below, and where a detail is not in the trace this page says so.
openclaw plugs in the same way every tool does, through a small `adapter.sh` that points it at the trace proxy, and it speaks native chat-completions on the wire with no translation shim in between.

## What it is

openclaw is a CLI agent installed with `npm install -g openclaw`.
The lab runs it from an image named `tomolab-tool-openclaw`, built on the shared `tomolab-base` so it runs against the same toolchain as every other tool.
The base already carries Node 22, which the adapter notes openclaw needs.

From the recovered prompt, openclaw introduces itself as "a personal assistant running inside OpenClaw" and treats its working directory as a persistent workspace with injected files: `AGENTS.md`, `SOUL.md`, `IDENTITY.md`, `USER.md`, `TOOLS.md`, and a `BOOTSTRAP.md` first-run script.
The prompt points at local docs under `/usr/lib/node_modules/openclaw/docs` and describes a Gateway control plane, sub-agent sessions, memory files, and a skills system.
Those are the surfaces openclaw exposes to itself in the prompt.
What the lab exercises is the file work the agent leaves in `/work`, not the messaging channels the wider project is built around.

## Command surface

Every openclaw invocation in the lab comes from the adapter and the Dockerfile, so the verified surface is small.

Install, from the Dockerfile:

```bash
npm install -g openclaw@${OPENCLAW_VERSION}
```

`OPENCLAW_VERSION` defaults to `latest`, so a plain build takes whatever npm currently publishes.

Setup and configuration, from the adapter:

```bash
openclaw setup --non-interactive --accept-risk
openclaw config set models.providers.lab "$provider" --strict-json
openclaw models set "lab/${LAB_MODEL}"
openclaw config set agents.defaults.workspace /work
openclaw exec-policy preset yolo
openclaw config file
```

Running one task, the real invocation the adapter uses:

```bash
openclaw agent --local --session-key run --json \
  --timeout "$((LAB_MAX_TURNS * 120))" \
  --message "$prompt"
```

`--accept-risk` skips the interactive safety warning a headless run cannot answer.
`--strict-json` tells `config set` the value is a literal JSON document.
`--local` runs the agent inline instead of talking to a running Gateway daemon, `--session-key run` fixes the session name, and `--json` makes it emit a structured result on stdout.
`--timeout` is seconds, computed as `LAB_MAX_TURNS * 120`.
Flags beyond these are not exercised by the lab, so this page does not list them.

## How the lab drives it

`adapter.sh` is the container entrypoint and the only openclaw-specific glue in the lab.
The harness mounts `/work` (the scenario's working tree and the agent's cwd), `/scenario` (read-only, holds `prompt.txt`), and `/trace` (stdout, the rendered config, the time report), and passes `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

It runs in order:

1. `openclaw setup --non-interactive --accept-risk` lays down the baseline config and workspace.
2. It registers an OpenAI-compatible provider named `lab`. The provider JSON sets `"api":"openai-completions"`, `"baseUrl":"${LAB_BASE_URL}"`, `"apiKey":"${OPENCODE_API_KEY}"`, and one model entry with `"contextWindow":131072` and `"input":["text"]`. `LAB_BASE_URL` is the trace proxy, so every request, response, and token count is captured without openclaw's cooperation, and the proxy forwards upstream with the real key.
3. `openclaw models set "lab/${LAB_MODEL}"` makes that provider and model the active one.
4. `openclaw config set agents.defaults.workspace /work` moves the agent workspace off its default `~/.openclaw/workspace` and onto `/work`, so openclaw acts on the exact tree the checker grades, the same footing as tomo.
5. `openclaw exec-policy preset yolo` grants full exec with no approval prompts. The comment calls the container the sandbox and this openclaw's equivalent of tomo's all-allow policy.
6. It copies the effective config into the trace (`config.path` and `config.json`) for the record.
7. It `cd /work` and runs the agent under `/usr/bin/time -v -o /trace/time.txt` so the harness reads peak memory back.

The provider declaration is the key line, quoted from the adapter:

```bash
provider=$(cat <<JSON
{"api":"openai-completions","baseUrl":"${LAB_BASE_URL}","apiKey":"${OPENCODE_API_KEY}","models":[{"id":"${LAB_MODEL}","name":"${LAB_MODEL}","contextWindow":131072,"input":["text"]}]}
JSON
)
```

Because `api` is `openai-completions`, openclaw talks plain chat-completions and the proxy passes it through without dialect translation.
The image is `FROM tomolab-base`, with `ARG OPENCLAW_VERSION=latest`, and openclaw installed from npm at that version.

## Architecture

Everything in this section comes from the recovered prompt's tool schema and the one captured trace, not from openclaw's source.

The prompt advertises 24 tools, listed here in the order the trace records them: `apply_patch`, `create_goal`, `cron`, `edit`, `exec`, `get_goal`, `image`, `memory_get`, `memory_search`, `process`, `read`, `session_status`, `sessions_history`, `sessions_list`, `sessions_send`, `sessions_spawn`, `sessions_yield`, `skill_workshop`, `subagents`, `update_goal`, `update_plan`, `web_fetch`, `web_search`, `write`.

File work runs through `read`, `write`, `edit`, and `apply_patch`, where `apply_patch` is described as applying multi-file patches in one call.
Shell work runs through `exec`, with `process` managing background exec sessions.
The prompt tells the agent to bias toward acting: an actionable request should be handled in the current turn, weak tool results should be retried with a varied query or path, and a final answer needs evidence such as a build, test, or inspection rather than a promise.

Planning has a dedicated tool, `update_plan`, alongside a goal family of `create_goal`, `get_goal`, and `update_goal`.
The `00-hello` run recorded `plan_calls: 0` and `planned: false`, so the plan tool exists in the schema but was not invoked for that scenario.

Delegation is built around a sub-agent session family.
`sessions_spawn` starts an isolated sub-agent, with a note to omit `context` for an isolated child and set `context:"fork"` only when the child needs the current transcript.
`sessions_yield` ends the turn and waits for spawned sub-agent completion events, which the prompt describes as push-based.
`sessions_list`, `sessions_history`, and `sessions_send` inspect and message other sessions, and `subagents` gives on-demand status without polling.
The prompt is explicit that the agent should not run poll loops over `subagents list` or `sessions_list` and should use `sessions_yield` to wait instead.
In the captured hello run `subagents: 0`, so no sub-agents were spawned there.

Memory is file-backed.
`memory_search` scans `MEMORY.md`, `memory/*.md`, and indexed session transcripts, and `memory_get` pulls only the needed lines, with the prompt asking for a `Source: <path#line>` citation when it helps.
The workspace files themselves (`AGENTS.md`, `SOUL.md`, and the rest) describe daily notes under `memory/YYYY-MM-DD.md` and a curated `MEMORY.md` as the agent's continuity across sessions.
Other tools in the schema, `cron`, `web_search`, `web_fetch`, `image`, `session_status`, and `skill_workshop`, round out scheduling, web access, image analysis, status reporting, and the skills workshop, but the lab's scenarios do not exercise them in the captured hello trace.

## System prompt

The recovered prompt for openclaw is one distinct prompt, `wire chat`, 31517 chars, with 24 tools.
It was recovered from the trace proxy across 19 captured runs, so this is the exact text that reached the model, not a copy from openclaw's source.
The 19 per-run renderings are grouped into one prompt because they differ only in a single volatile `Runtime` line, which carries a session uuid and a host id that change every run.

The prompt opens by stating what it is and how it lists its tools:

```text
You are a personal assistant running inside OpenClaw.
## Tooling
Available tools are policy-filtered. Names are case-sensitive; call exactly as listed.
```

This sets the tool contract for the model: the tool set can be filtered by policy, and names must match exactly.

The delegation rules are spelled out inline, for example:

```text
Larger work: use `sessions_spawn`; completion is push-based.
`sessions_spawn`: omit `context` unless transcript needed; then set `context:"fork"`.
```

That is the same sub-agent behavior the tool schema exposes, restated as guidance so the model spawns children correctly and waits on events instead of polling.

The workspace is pinned to the path the lab grades:

```text
## Workspace
Your working directory is: /work
```

This is the effect of the adapter's `agents.defaults.workspace /work` setting showing up in the prompt the model actually reads.

The volatile part is the `Runtime` line near the end, which looks like this in the hello run:

```text
Runtime: agent=main | session=agent:main:run | sessionId=965dc6af-6cea-4e16-af13-ecb23b1dbfc3 | host=01d5f8f35702 | repo=/work | os=Linux 7.0.12-201.fc44.aarch64 (arm64) | node=v22.23.1 | model=lab/deepseek-v4-flash-free | default_model=lab/deepseek-v4-flash-free | thinking=off
```

The `sessionId` and `host` in that line are what differ across the 19 renderings; the rest of the prompt is identical.
The full text is on the prompt page at [/prompts/openclaw/](/prompts/openclaw/), recovered from the traces.

## Hi! end to end

The `00-hello` scenario sends the agent a single greeting and checks that a baseline round trip completed.
Here is that run from the trace.

The adapter builds its request as usual: `setup`, then the `lab` provider pointed at the proxy, then `openclaw agent --local --session-key run --json` with the prompt.
The proxy tap recorded 2 records total, a `GET /zen/` and one `POST /zen/v1/chat/completions`.
That single completion is the native chat wire on the untagged path: openclaw speaks openai-completions, so the proxy forwards it without translating a dialect.

The completion request carries `model` `deepseek-v4-flash-free` and the forced-determinism knobs the lab applies to every tool: `temperature=0`, `top_p=1`, `seed=7`, `stream=True`.
It sends two messages, a system prompt whose first line is "You are a personal assistant running inside OpenClaw." and a user message of `[Fri 2026-07-10 13:44 UTC] Hi!`, plus all 24 tools.

The model answered in one call.
`result.json` records `passed: true`, `attempts: 1`, `requests: 2`, `model_calls: 1`, `tool_calls: 0`, `subagents: 0`, and `planned: false`.
Tokens were 16637 prompt, 118 completion, 16755 total, with 7552 cached.
Latency was 1163 ms average time to first byte and 3632 ms average total over the one completion.
Install footprint was 417267 kb and peak resident set 375508 kb.

The agent's reply, from `stdout.log`, followed the `BOOTSTRAP.md` first-run script rather than a generic greeting:

```text
Hey 👋

I just came online. Fresh workspace, no memory yet — literally nothing but what's in those files. So let's figure this out together.

Who are you? And... who do you want me to be? Got a name in mind?
```

The checker's note is `baseline greeting round trip completed`, and the run passed on the first attempt.
