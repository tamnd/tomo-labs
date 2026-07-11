---
title: "opencode"
description: "The sst/opencode terminal coding agent, driven headless through opencode run against the lab's trace proxy."
weight: 30
---

opencode is an open-source terminal coding agent written in TypeScript by the team at sst.
It normally runs as a TUI, but it also ships a headless one-shot mode, `opencode run`, that takes a single prompt, works until it goes idle, and exits.
That mode is the whole reason the lab can drive it: one adapter script, one Dockerfile, and opencode's model provider pointed at the trace proxy.
opencode speaks OpenAI chat-completions natively through the AI SDK, so the wire needs no translation and the proxy records it on its plain chat path.
This page is grounded entirely in the wired image, the adapter, and the newest `00-hello` trace; it claims only what those show.

## Overview

opencode is a coding agent you run in your terminal.
It reads and edits files, runs shell commands, searches the tree, and fetches web pages, all under a permission model you can tighten or loosen.
The provider and model are configurable, so opencode is not tied to one vendor: it selects a model with the `provider/model` form.
In the lab it runs the same fixed model every other tool runs, so the only variable under study is the agent itself.

The Dockerfile installs opencode from npm, not from a source checkout, so the image never depends on a clone of the repo.
It pins the version explicitly:

```dockerfile
FROM tomolab-base
ARG OPENCODE_VERSION=1.17.18
RUN npm install -g opencode-ai@${OPENCODE_VERSION}
COPY adapter.sh /usr/local/bin/adapter
```

The npm package is `opencode-ai`, the installed binary is `opencode`, and the captured version is `1.17.18`.
opencode is the heaviest of the wired tools on memory: the `00-hello` run peaks at 676 MB resident, the highest of any tool in the suite.

### At a glance

| Property | Value |
| --- | --- |
| Runtime | Node 22, from the shared `tomolab-base` image |
| Install source | npm package `opencode-ai`, binary `opencode` |
| Version captured | `1.17.18` (Dockerfile `OPENCODE_VERSION`) |
| Wire dialect | OpenAI chat-completions (`@ai-sdk/openai-compatible`) |
| How the lab invokes it | `opencode run --model lab/$LAB_MODEL --dir /work --auto "$prompt"` |
| Provider config | `~/.config/opencode/opencode.json`, a custom `lab` provider |
| Where it writes | `/work` for edits, `/trace` for config, stdout, and the time report |
| Peak memory (00-hello) | 676 MB, the highest of any wired tool |
| Install footprint | 431 MB |

### Tools and features

The agent turn hands the model ten tools.
These are the whole surface opencode acts through, taken from the recovered agent prompt and the `00-hello` request body.

| Tool | What it does |
| --- | --- |
| `bash` | Runs a shell command in a persistent session, with optional timeout |
| `read` | Reads a file |
| `write` | Writes a new file |
| `edit` | Edits an existing file in place |
| `glob` | Matches paths by pattern |
| `grep` | Searches file contents across the tree |
| `webfetch` | Pulls a URL, used to read opencode's own docs when asked about itself |
| `todowrite` | Writes and updates a task list so a longer job stays structured |
| `task` | Spawns a subagent, which the prompt steers file search toward to keep context small |
| `skill` | Loads a named skill on demand; the prompt advertises one, `customize-opencode` |

Two behaviors are worth flagging up front because the `00-hello` run exercises neither.
`todowrite` is planning, and for a trivial task opencode writes no plan at all.
`task` and `skill` are optional escalation paths, not used on a bare greeting.

## Say Hi!

The `00-hello` scenario is the smallest run in the suite.
The prompt is `Hi!` and the checker asks only that a greeting round trip completed.
Here is the run end to end, from the newest trace (`20260710T134917Z`).

The adapter reads the prompt from the read-only scenario mount:

```bash
prompt="$(cat /scenario/prompt.txt)"   # "Hi!"
```

Before running anything it writes opencode's global config, registering a custom OpenAI-compatible provider named `lab` whose `baseURL` is the trace proxy, not the real upstream:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "lab": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "lab",
      "options": {
        "baseURL": "http://tomolab-proxy-2:8080/v1",
        "apiKey": "sk-...redacted..."
      },
      "models": {
        "deepseek-v4-flash-free": { "name": "deepseek-v4-flash-free" }
      }
    }
  }
}
```

That file is copied to `/trace/config.json` so the run records exactly what opencode was told.
Then the adapter pins the working tree and runs opencode once, non-interactively:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  opencode run --model "lab/${LAB_MODEL}" --dir /work --auto "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

opencode builds its turn from the config: the baked-in agent system prompt, the user message `Hi!`, the ten tool schemas, all on the chat wire.
At the proxy the run lands as three records, and the two model calls both arrive on the plain chat path `POST /zen/v1/chat/completions` with the lab's forced decoding.

| Field | Value |
| --- | --- |
| `temperature` | 0 |
| `top_p` | 1 |
| `seed` | 7 |
| `stream` | true |
| `stream_options` | `{ "include_usage": true }` |
| `max_tokens` | 32000 |

### Why a bare hello made 3 requests and 2 model calls

The three proxy records are one health probe and two completion POSTs:

| seq | Method and path | Role | Messages | Tools |
| --- | --- | --- | --- | --- |
| 1 | `GET /zen/` | health probe | none | none |
| 2 | `POST /zen/v1/chat/completions` | title generator | `system`, `user`, `user` | none |
| 3 | `POST /zen/v1/chat/completions` | agent | `system`, `user` | 10 |

Record 1 is opencode checking the provider is reachable before it starts, `ttfb` 770 ms.
Record 2 is opencode talking to itself: it names every session with a short title so a conversation is findable later, and it does that with a separate model call.
Its body carries three messages (`system` "You are a title generator. You output ONLY a thread title. Nothing else.", then `user` "Generate a title for this conversation:", then `user` "Hi!") and zero tools.
That title call normally runs on opencode's cheaper `small_model`, but the lab registers only one model, so it falls back to the same deepseek model as the agent, which is why the side call shows up in the trace at all.
Record 3 is the actual agent turn: the 9559-char agent system prompt, the user `Hi!`, all ten tools, `tool_choice: auto`.
A greeting needs no tools, so the agent makes zero tool calls and writes no plan, streams its reply, and exits.

### The numbers

| Metric | Value |
| --- | --- |
| Passed | true, on attempt 1 of 3 allowed |
| Proxy records | 3 (1 `GET`, 2 `POST`) |
| Model calls | 2 (agent + title generator) |
| Tool calls | 0 |
| Plan calls | 0 |
| Subagents | 0 |
| Prompt tokens | 7236 |
| Completion tokens | 24 (14 of them reasoning) |
| Total tokens | 7260 |
| Cached tokens | 7168 |
| Cache hit rate | 7168 / 7236 = 99.1% of prompt served from cache |
| TTFB (agent call) | 1023 ms |
| Total (agent call) | 5837 ms |
| Peak RSS | 676 MB, highest of any wired tool |
| Install footprint | 431 MB |
| Wall clock | 0:10.72 |

The cache hit rate is the number to notice: 99.1% of the prompt came back from cache, so only 68 prompt tokens were fresh on the agent call.
opencode's cost for the run reads `0.00000000` (the model is free), and the two calls came back clean.

The recorded reply on stdout is verbatim:

```
Hi! How can I help you today?
```

The checker grades a pass with the verdict `baseline greeting round trip completed`.

## Architecture

Enough here to reimplement the wiring from scratch.

### The container

The image is built from `tomolab-base`, the shared base every tool runs against, which already carries the Node 22 opencode needs.
On top of that the Dockerfile does exactly one install step and installs the adapter as the entrypoint:

```dockerfile
FROM tomolab-base
ARG OPENCODE_VERSION=1.17.18
RUN npm install -g opencode-ai@${OPENCODE_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

There is no opencode source in the image.
`opencode-ai` is a self-contained npm binary, and the `@ai-sdk/openai-compatible` provider it names is fetched at first run.

### Mounts

The harness mounts three directories into the container.

| Mount | Access | Purpose |
| --- | --- | --- |
| `/work` | read-write | The scenario's working tree and the agent's cwd; the tree the checker grades |
| `/scenario` | read-only | The scenario definition, holds `prompt.txt` |
| `/trace` | read-write | Where the config, stdout, stderr, exit code, and time report land |

### Harness environment

The harness passes four environment variables into the adapter.

| Variable | Meaning | Used by the adapter |
| --- | --- | --- |
| `LAB_BASE_URL` | Proxy base URL, e.g. `http://tomolab-proxy-2:8080/v1` | Written into the provider `baseURL` |
| `LAB_MODEL` | Model id, e.g. `deepseek-v4-flash-free` | Registered as a `lab` model and passed to `--model` |
| `OPENCODE_API_KEY` | Upstream key the proxy forwards with | Written into the provider `apiKey` |
| `LAB_MAX_TURNS` | Turn budget shared across tools | Passed in but not wired to a flag; opencode's loop ends on idle |

`LAB_MAX_TURNS` is present for parity with other adapters, but the opencode adapter does not translate it into a `run` flag.
opencode's headless loop terminates on its own when the model stops asking for tools, so the turn cap is not enforced from the outside here.

### The adapter step by step

The adapter is the container entrypoint and the only opencode-specific glue in the lab.
Everything upstream of it, the network, the trace capture, the resource accounting, is identical for every tool.

First it reads the prompt:

```bash
prompt="$(cat /scenario/prompt.txt)"
```

Then it writes opencode's global config to `~/.config/opencode/opencode.json`.
This is the load-bearing step: it registers a custom OpenAI-compatible provider named `lab` and points its `baseURL` at the proxy instead of the real upstream.

```bash
mkdir -p "$HOME/.config/opencode"
cat >"$HOME/.config/opencode/opencode.json" <<JSON
{
  "\$schema": "https://opencode.ai/config.json",
  "provider": {
    "lab": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "lab",
      "options": {
        "baseURL": "${LAB_BASE_URL}",
        "apiKey": "${OPENCODE_API_KEY}"
      },
      "models": {
        "${LAB_MODEL}": { "name": "${LAB_MODEL}" }
      }
    }
  }
}
JSON
cp "$HOME/.config/opencode/opencode.json" /trace/config.json 2>/dev/null || true
```

Reading this config back:

- `npm: @ai-sdk/openai-compatible` tells opencode which provider package to use; it emits standard chat requests.
- `options.baseURL` is the proxy, so every request, response, and token count is captured with no cooperation from opencode.
- `options.apiKey` is the real key; the proxy forwards it to the real upstream.
- the single entry under `models` is the model the lab registers, keyed and named by `LAB_MODEL`.

Then it pins cwd, model qualification, and approvals, and runs opencode once:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  opencode run --model "lab/${LAB_MODEL}" --dir /work --auto "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?
echo "$status" >/trace/exit_code
exit 0
```

- `run` is the headless one-shot mode: one message in, files and stdout out, then exit.
- `--model lab/$LAB_MODEL` qualifies the model as `provider/model`, selecting the `lab` provider and its one model.
- `--dir /work` pins the working tree to the exact tree the checker inspects; `cd /work` also sets cwd, and `$HOME` is where the config was written.
- `--auto` approves every permission the run does not explicitly deny, opencode's equivalent of an all-allow policy, so shell scenarios run unattended; the container is the sandbox.
- output capture: stdout to `/trace/stdout.log`, stderr to `/trace/stderr.log`, exit code to `/trace/exit_code`.
- `/usr/bin/time -v -o /trace/time.txt` wraps the whole run so the harness reads peak resident set back from the GNU time report.

The adapter always `exit 0`s after recording opencode's real status, so a nonzero agent exit does not crash the container before the trace is written.

### How opencode reaches the proxy, and the agent loop

opencode never knows it is being traced.
It reads the `lab` provider from config, and the `@ai-sdk/openai-compatible` package sends ordinary chat-completions requests to `baseURL`, which is the proxy.
The proxy normalizes each completion to the chat-completions shape, tees the request body, streamed response, and token usage into `/trace`, and forwards to the real upstream with the real key.

The headless loop is a straight agentic cycle.
`run` sends the prompt as one user message, the model replies with text and any tool calls, opencode executes the tool calls through the permission layer (waved through by `--auto`) and feeds the results back, and the loop repeats until the model stops asking for tools and the session goes idle.
Native tool-calling drives this: the ten tools ship as function schemas with `tool_choice: auto`, and opencode dispatches whatever the model requests.
The wire is plain OpenAI chat-completions end to end, so the proxy records it on its untagged chat path with no dialect translation.

## System Prompts

The prompt on this page is opencode's own baked-in system prompt, recovered verbatim by `lab prompts opencode`, not something the lab injects.
The lab injects nothing into the prompt; it only redirects the provider `baseURL` so the proxy can record what opencode already sends.
The recovery reads each completion after the proxy normalizes it to the chat-completions shape, so it is the exact text that reached the model, not a copy lifted from the source.
Full verbatim text, byte counts, and request counts are at [/prompts/opencode/](/prompts/opencode/).

The proxy captured two distinct prompts across the run.

| Prompt | Role | Size | Requests | Tools | Wire |
| --- | --- | --- | --- | --- | --- |
| 1, agent | the working prompt opencode runs on | 9559 chars | 122 | 10 | chat |
| 2, side | thread-title generator, opencode talking to itself | 2119 chars | 27 | 0 | chat |

Prompt 1 is the working prompt: it is what record 3 in the `00-hello` trace carries, with all ten tools and the user turn.
Prompt 2 is the title generator from record 2, a lighter call with no tools, described in the Say Hi section above.

### Prompt 1, the agent prompt

It opens by naming the tool and its job:

```text
You are opencode, an interactive CLI tool that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.
```

The body breaks into labeled sections, each doing one job.

Identity and self-reference.
When asked about itself, opencode is told to fetch its own docs rather than answer from memory, and it is given the feedback channel:

```text
When the user directly asks about opencode (eg 'can opencode do...', 'does opencode have...') or asks in second person (eg 'are you able...', 'can you do...'), first use the WebFetch tool to gather information to answer the question from opencode docs at https://opencode.ai
```

Tone and safety.
Most of the prompt is a brevity policy tuned for a terminal, pushing the model to spend as few tokens as it can:

```text
IMPORTANT: Keep your responses short, since they will be displayed on a command line interface. You MUST answer concisely with fewer than 4 lines (not including tool use or code generation), unless user asks for detail.
```

It also bars guessing URLs, tells the model not to explain refusals, and reserves emojis for explicit requests.

Editing conventions.
It insists the model match the surrounding code before changing it, and it is blunt about not leaving unasked-for traces:

```text
# Code style
- IMPORTANT: DO NOT ADD ***ANY*** COMMENTS unless asked
```

It also forbids committing unless the user explicitly asks, and it tells the model to run lint and typecheck after a task when those commands are known.

Planning and tool policy.
File search is steered toward the `task` subagent to keep the main context small, and independent tool calls are batched:

```text
# Tool usage policy
- When doing file search, prefer to use the Task tool in order to reduce context usage.
- You have the capability to call multiple tools in a single response. When multiple independent pieces of information are requested, batch your tool calls together for optimal performance.
```

Formatting.
Output is treated as CommonMark rendered in a monospace terminal, and code references use the `file_path:line_number` pattern so the user can jump to source.

Volatile tail.
The last lines are filled in at runtime, so they describe the lab's container, not any fixed default, and they are the spans worth ignoring when diffing prompt captures:

```text
You are powered by the model named deepseek-v4-flash-free. The exact model ID is lab/deepseek-v4-flash-free
Here is some useful information about the environment you are running in:
<env>
  Working directory: /work
  Workspace root folder: /
  Is directory a git repo: no
  Platform: linux
  Today's date: Fri Jul 10 2026
</env>
```

The available-skills list is also substituted here, advertising the one built-in skill `customize-opencode` for editing opencode's own configuration.
Everything above this tail matched opencode's published system prompt, down to the feedback line and the docs URL; only the tail is runtime-substituted.

### Prompt 2, the side prompt

The second prompt is opencode naming each session so a conversation is findable later.
It is strict and single-purpose:

```text
You are a title generator. You output ONLY a thread title. Nothing else.
```

Its rules force a single line under 50 characters, ban tool names and complaints, and handle the trivial case explicitly:

```text
- If the user message is short or conversational (e.g. "hello", "lol", "what's up", "hey"):
  → create a title that reflects the user's tone or intent (such as Greeting, Quick check-in, Light chat, Intro message, etc.)
```

This is the prompt behind the extra model call on every run, including the bare `Hi!`.
It normally runs on a cheaper `small_model`; the lab registers only one model, so it runs the same deepseek model as the agent, which is the only reason it appears in the trace.
