---
title: "hermes"
description: "hermes is Nous Research's Hermes Agent, driven headless with hermes chat -q and speaking native chat-completions to the trace proxy."
weight: 60
---

## Overview

hermes is Hermes Agent by Nous Research, wired into the lab as one more coding agent under study.
The lab installs it from the npm package `hermes-agent`, a thin bridge whose postinstall step pip installs the upstream Python Hermes Agent, so the process the lab actually drives is a Python runtime the npm bridge launches.
The image pins the bridge with a build arg, `ARG HERMES_VERSION=0.18.2`, and installs it with `npm install -g hermes-agent@${HERMES_VERSION}`.
It presents as a general personal agent that also does coding work: the recovered system prompt opens with "You are Hermes Agent, an intelligent AI assistant created by Nous Research," and points at `https://hermes-agent.nousresearch.com/docs` as its own reference.

Its headline features, all grounded in the recovered prompt and the captured trace: persistent cross-session memory injected every turn, a save-and-patch skills loop, subagent delegation, a full terminal and file toolset, and a built-in command security scanner it calls "tirith."
That scanner announces itself on every run.
The first thing hermes prints after "Initializing agent..." is the line `⚠ tirith security scanner enabled but not available — command scanning will use pattern matching only`, so on the lab image the scanner degrades to pattern matching rather than its full check.

This page is grounded in the captured 00-hello run: the trace, the adapter, the Dockerfile, and the prompt recovered by `lab prompts hermes`, not a reading of Hermes' own source.

### At a glance

| Field | Value |
| --- | --- |
| Runtime | podman container on the shared `tomolab-base` image |
| Install source | npm `hermes-agent`, postinstall pip installs the upstream Python Hermes Agent (Nous Research) |
| Version captured | `hermes-agent@0.18.2` (Dockerfile `ARG HERMES_VERSION=0.18.2`) |
| Wire dialect | native chat-completions, streamed SSE, no dialect translation |
| Lab invocation | `hermes chat -q "<prompt>" --yolo`, one-shot, non-interactive |
| Model config | `hermes config set model.*` writes `~/.hermes/config.yaml` before the run |
| Working dir | `/work` (the agent's cwd, writable) |
| Trace out | `/trace` (stdout, rendered config, GNU time report) |
| 00-hello result | pass, 26 requests, 1 model call, 0 tool calls |

### Tools and features

Each completion carries all 19 tool schemas; the model picks one by name.
The trace names them and the prompt describes what they are for.

| Tool | Role (from the prompt and trace) |
| --- | --- |
| `terminal` | shell, git, builds, tests, inspection; the scanner target |
| `read_file` / `write_file` | read files, author new files |
| `patch` | edit existing files, `mode='replace'` preferred, `mode='patch'` (V4A) for multi-file |
| `search_files` | locate code and symbols across the tree |
| `execute_code` | run code directly |
| `process` | background/long-running work |
| `todo` | in-context planning for multi-step work |
| `delegate_task` | hand work to a subagent |
| `memory` | durable facts about user and environment, injected every turn |
| `session_search` | recall task progress and past transcripts (kept out of memory) |
| `skills_list` / `skill_view` / `skill_manage` | enumerate, read, and create/patch skills (the learning loop) |
| `clarify` | ask the user a question |
| `cronjob` | schedule local jobs |
| `image_generate` / `vision_analyze` / `text_to_speech` | media tools |

Feature notes grounded in the prompt: memory is compact and declarative, and the prompt explicitly bars saving task progress, PR/issue numbers, or anything stale within a week; skills are the place for reusable procedures; the coding block tells hermes to gather context first, edit through tools rather than chat, and verify with real tool output before claiming a task done.

## Say Hi!

The 00-hello scenario hands hermes the single prompt `Hi!` and checks that a greeting round trip completes.
This is the walkthrough of the passing run captured at `20260710T134145Z`.

### The adapter sets up and fires once

The adapter reads the prompt, points hermes at the trace proxy two ways at once, and runs it once non-interactively.
It exports the OpenAI-style env vars and also writes the same values into hermes' config, because the custom provider reads the config, not `OPENAI_API_KEY`.

```bash
prompt="$(cat /scenario/prompt.txt)"

export OPENAI_API_KEY="${OPENCODE_API_KEY}"
export OPENAI_BASE_URL="${LAB_BASE_URL}"
hermes config set model.provider custom
hermes config set model.base_url "${LAB_BASE_URL}"
hermes config set model.default "${LAB_MODEL}"
hermes config set model.api_key "${OPENCODE_API_KEY}"
```

The rendered `~/.hermes/config.yaml` that reached this run:

```yaml
model:
  provider: custom
  base_url: http://tomolab-proxy-1:8080/v1
  default: deepseek-v4-flash-free
  api_key: sk-...            # the real upstream key, forwarded by the proxy
```

`LAB_BASE_URL` is the trace proxy, so setting `model.base_url` routes every model call through it; the proxy forwards to the real upstream with the real key.
The adapter then sets `HERMES_YOLO_MODE=1` and passes `--yolo` to auto-approve every action, wraps the run in GNU time, and captures streams into `/trace`.

```bash
export HERMES_YOLO_MODE=1
cd /work
/usr/bin/time -v -o /trace/time.txt \
  hermes chat -q "$prompt" --yolo \
  >/trace/stdout.log 2>/trace/stderr.log
```

### hermes builds one request, but sends 26

hermes builds a single chat completion around the prompt: a system message with the general agent prompt, then the user message `Hi!`, plus all 19 tool schemas, on the native chat wire.
The wire body confirms it: `model=deepseek-v4-flash-free`, two messages (system 7526 chars, user 3 chars, which is `Hi!`), and 19 tools.
This trace predates a harness change, when the proxy still pinned greedy decoding, so the body shows `temperature=0`, `top_p=1`, `seed=7`, and `stream=true`; today the proxy passes each tool's own sampling through untouched.

The standout fact is that answering "Hi!" logged 26 requests while only one was a model completion.
The other 25 are hermes' own scaffolding during the "Initializing agent..." phase: capability probes against the OpenAI-compatible endpoint plus the setup chatter the CLI prints.
The trace shows the probes plainly; they are repeated GET and POST sweeps for provider metadata that all 404 against the proxy, before the one real completion goes out.

| seq | request | status | note |
| --- | --- | --- | --- |
| 1 | `GET /zen/` | 200 | reach the endpoint |
| 2-11 | `GET /zen/api/v1/models`, `/zen/api/tags`, `/zen/v1/props`, `/zen/props`, `/zen/version` (twice) | 404 | capability discovery sweep |
| 12 | `GET /zen/v1/models` | 200 | model list found |
| 13 | `POST /zen/api/show` | 404 | Ollama-style model probe |
| 14-19 | more `models`/`tags`/`props`/`version` + `GET /zen/v1/models/deepseek-v4-flash-free` | 404 | keep feeling out the API |
| 20 | `GET /zen/v1/models` | 200 | model list again |
| 21-25 | `models`/`tags`/`props`/`version` sweep | 404 | last discovery pass |
| 26 | `POST /zen/v1/chat/completions` | 200 | the one model call |

Alongside the probes, stdout shows the tirith security scanner announcing that it is enabled but unavailable and falling back to pattern matching.
That scanner banner is part of the same init scaffolding: hermes stands up its own tooling and provider checks before it ever talks to the model, and that is why a bare greeting cost 26 requests for a single completion.

### The one completion, and the numbers

That one request gets one streamed completion, and hermes answers without calling a tool.
Only the completion is a timed model call; the 25 probes each returned in roughly 240 to 350 ms, while the completion took 8136 ms to first byte.

| Metric | Value |
| --- | --- |
| passed | true (attempt 1 of 3) |
| requests | 26 (1 completion, 25 init probes), the most of any tool for a bare greeting |
| model_calls | 1 |
| tool_calls / plan_calls / subagents | 0 / 0 / 0 |
| planned | false |
| tokens | 13586 prompt, 33 completion, 13619 total |
| cached tokens | 7808 (of the prompt), 23 reasoning tokens |
| ttfb | 8136 ms |
| total (completion) | 9562 ms, the slowest baseline in the group |
| wall clock | 26 s (elapsed 0:24.60) |
| peak RSS | 124868 KB (~122 MB) |
| install footprint | 226651 KB (~221 MB) |
| runtime | podman |

The large prompt count is the 7500-plus-char system prompt and the 19 tool schemas; the cache hit covers 7808 of those prompt tokens.
The completion streamed as SSE chunks and finished with `finish_reason: "stop"`, usage `prompt_tokens=13586, completion_tokens=33, total_tokens=13619, prompt_cache_hit_tokens=7808`.

The reply that reached the user, from `stdout.log`, is:

```text
Hi! How can I help you today?
```

stdout also shows the terminal chrome around that line: the tirith fallback warning, a Reasoning box ("The user is just saying 'Hi!' ... I'll respond warmly and helpfully."), the Hermes reply panel, and a session footer ("Messages: 2 (1 user, 0 tool calls)").

### The checker grades a pass

The checker never reads the model's prose.
It confirms the greeting round trip completed, records `check: "baseline greeting round trip completed"`, and marks `passed: true` on the first attempt with `exit_code: 0`.

## Architecture

Enough to reimplement the harness side from scratch.

### The container

The image builds on the shared `tomolab-base`, which already carries Node 22 and Python 3.11 that the npm bridge and the Python agent both need.
Debian bookworm marks its system Python as externally managed (PEP 668), so the bridge's pip step needs `PIP_BREAK_SYSTEM_PACKAGES=1` at build time.
The entrypoint is the adapter.

```dockerfile
FROM tomolab-base
ENV PIP_BREAK_SYSTEM_PACKAGES=1
ARG HERMES_VERSION=0.18.2
RUN npm install -g hermes-agent@${HERMES_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

### Mounts and harness env

The adapter is the only hermes-specific glue; everything upstream of it (network, trace capture, resource accounting) is identical for every tool.
The harness mounts three paths:

| Mount | Access | Purpose |
| --- | --- | --- |
| `/work` | read-write | the scenario's working tree, and the agent's cwd |
| `/scenario` | read-only | the scenario definition, holds `prompt.txt` |
| `/trace` | read-write | stdout, stderr, rendered config, GNU time report |

And passes four environment variables:

| Env var | Used for |
| --- | --- |
| `LAB_BASE_URL` | the trace proxy URL, set into `model.base_url` and `OPENAI_BASE_URL` |
| `LAB_MODEL` | the model id, set into `model.default` (`deepseek-v4-flash-free`) |
| `OPENCODE_API_KEY` | the real upstream key, set into `model.api_key` and `OPENAI_API_KEY` |
| `LAB_MAX_TURNS` | the turn budget; the adapter does not reference it, so hermes uses its own default |

### The adapter, step by step

1. Read the task: `prompt="$(cat /scenario/prompt.txt)"`.
2. Point at the proxy twice: export `OPENAI_API_KEY` and `OPENAI_BASE_URL`, then `hermes config set model.provider custom`, `model.base_url`, `model.default`, and `model.api_key`.
3. Persist the key in config: the custom provider does not read `OPENAI_API_KEY`, so without `model.api_key` the proxy forwards an empty bearer and the upstream rejects the one real chat call with 401.
4. Record the effective config: `cp "$HOME/.hermes/config.yaml" /trace/config.yaml`.
5. Approvals off the leash: `export HERMES_YOLO_MODE=1` plus `--yolo`, the hermes equivalent of tomo's all-allow policy, so shell scenarios run headless.
6. Pin cwd to `/work`: `cd /work` so all file work lands where the checker grades it. HOME stays the container default `/root`, where `.hermes/` lives.
7. Run once under GNU time: `/usr/bin/time -v -o /trace/time.txt hermes chat -q "$prompt" --yolo` with stdout to `/trace/stdout.log` and stderr to `/trace/stderr.log`.
8. Capture the exit code: `echo "$status" >/trace/exit_code` and `exit 0` so the container itself always exits clean and the harness reads the result from the trace.

### How hermes reaches the proxy

`model.base_url` is `http://tomolab-proxy-1:8080/v1`, which is the trace proxy, not the upstream.
The proxy tees a copy of every request into the trace under a `/zen/` namespace and forwards it upstream with the real credential.
hermes already speaks native chat-completions, so the proxy does not shim a dialect: it normalizes each request to the chat-completions shape, records it, and forwards.
The 00-hello trace confirms the completion path is `POST /zen/v1/chat/completions`, a plain streamed chat call with two messages and no translation.

### The agent loop

hermes runs an agent loop over the model's native function-calling: it sends the conversation with all 19 tool schemas, runs any tool the model asks for and feeds the result back, and stops when the model answers without a tool call.
The loop is bounded by hermes' own max-turns default here, since the adapter does not wire `LAB_MAX_TURNS`.
For 00-hello the loop made exactly one model call and zero tool calls (`model_calls: 1`, `tool_calls: 0`, `planned: false`).

### The init phase inflates the request count

Before that one completion, hermes stands up its own tooling: it announces the tirith command security scanner (which is unavailable on this image and falls back to pattern matching), and it probes the OpenAI-compatible endpoint for capabilities with repeated GET/POST sweeps for `models`, `tags`, `props`, `version`, and `show`.
Those 25 probe requests are why a one-line greeting logged 26 requests for a single model completion; they are hermes' own scaffolding, not lab-injected traffic.

## System Prompts

This is hermes' OWN baked-in system prompt.
It was recovered verbatim by `lab prompts hermes` from the trace proxy across 15 captured runs (newest `20260710T134419Z`), so it is the exact text hermes sent to the model, NOT anything the lab injected.
Every completion routes through the proxy, which records each request after normalizing it to the chat-completions shape, so the [/prompts/hermes/](/prompts/hermes/) page holds the ground truth this section reads from.

### What was captured

The proxy captured four distinct prompts, all on wire `chat`.

| Prompt | Size | Requests | Tools | Role |
| --- | --- | --- | --- | --- |
| Prompt 1 | 7562 chars | 64 | 19 | general agent prompt |
| Prompt 2 | 10561 chars | 16 | 19 | agent + coding block, project `go.mod` |
| Prompt 3 | 10567 chars | 3 | 19 | agent + coding block, project `package.json` |
| Prompt 4 | 312 chars | 13 | 0 | side prompt: title a conversation |

Prompt 1 is the working prompt for 00-hello: the wire body for the passing run carried the general agent prompt (7526 chars as sent) with the user message `Hi!` and all 19 tool schemas.
Prompts 2 and 3 are a near-twin pair that differ only in the workspace manifest they name, `go.mod` versus `package.json`, so the difference tracks the scenario's project, not hermes' behavior.
Prompt 4 is a utility call hermes makes to name the session, separate from the agent loop, which is why it appears on its own with no tools attached.

### What each part does

The prompt opens by fixing identity and register.

```text
You are Hermes Agent, an intelligent AI assistant created by Nous Research. You are helpful, knowledgeable, and direct.
```

It points hermes at its own docs as the source of truth and tells it to load its self-skill for workflows.

```text
the documentation at https://hermes-agent.nousresearch.com/docs is your authoritative reference ... Load the `hermes-agent` skill with skill_view(name='hermes-agent') for additional guidance and proven workflows, but treat the docs as the source of truth when the two differ.
```

A "Finishing the job" block sets a hard rule against fabricating results, the same discipline the lab grades on.

```text
NEVER substitute plausible-looking fabricated output (made-up data, invented file contents, synthesised API responses) for results you couldn't actually produce. Reporting a blocker honestly is always better than inventing a result.
```

Tool-use enforcement bars planning without acting.

```text
You MUST use your tools to take action — do not describe what you would do or plan to do without actually doing it. ... Never end your turn with a promise of future action — execute it now.
```

The memory guidance is unusually specific about what does not belong in memory, which is what keeps the injected memory block small.

```text
Do NOT save task progress, session outcomes, completed-work logs, or temporary TODO state to memory; use session_search to recall those from past transcripts.
```

The skills loop tells hermes to save proven approaches and patch stale ones on sight.

```text
When using a skill and finding it outdated, incomplete, or wrong, patch it immediately with skill_manage(action='patch') — don't wait to be asked. Skills that aren't maintained become liabilities.
```

The coding prompt (Prompts 2 and 3) adds a senior-engineer block on top of the general prompt.

```text
You are a coding agent pairing with the user inside their codebase. Operate like a careful senior engineer.
```

It sequences the work: gather context first with `read_file` and `search_files`, edit through `patch`/`write_file` rather than printing code to chat, verify with real `terminal` output before claiming done, and track multi-step work with `todo`.
It also sets safety rails: do not commit, push, or rewrite history unless asked, and leave `.env` and credential files alone.

The side prompt (Prompt 4) is a small titling utility, not part of the agent loop.

```text
Generate a short, descriptive title (3-7 words) for a conversation that starts with the following exchange. ... Return ONLY the title text, nothing else.
```

### Spans worth ignoring when diffing

Some spans are volatile and change per run, so ignore them when diffing prompts for drift: the `Conversation started:` date line, the `Model:` and `Provider:` lines, the `go.mod` versus `package.json` workspace manifest that separates Prompts 2 and 3, and the host detail lines (`Host:`, `User home directory:`, `Current working directory:`).
These track the environment and scenario, not a change in hermes' behavior.

See the [/prompts/hermes/](/prompts/hermes/) page for the four prompts in full.
