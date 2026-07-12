---
title: "copilot"
description: "The GitHub Copilot CLI, driven headless through copilot -p in BYOK mode against the lab's trace proxy on the same fixed model as every other agent."
weight: 110
---

The GitHub Copilot CLI is a terminal coding agent built by GitHub.
It normally runs as an interactive session, but it also has a headless one-shot mode, `copilot -p`, that takes a single prompt, works the task autonomously, and exits.
That mode is how the lab drives it: one adapter script, one Dockerfile, and copilot's bring-your-own-key provider pointed at the trace proxy.
copilot speaks OpenAI chat-completions on the wire, so the proxy records it on its plain chat path with no translation.
This page is grounded entirely in the wired image, the adapter, and the newest `00-hello` trace; it claims only what those show.

## Overview

copilot is a coding agent you run in your terminal.
It reads and edits files, runs shell commands, searches the tree, fetches web pages, manages a per-session SQL database for todos and state, and can delegate to sub-agents.
Its BYOK mode points the CLI at any OpenAI-compatible provider entirely through environment variables, and when a provider base URL is set the CLI does not require GitHub authentication.
That is the whole reason the lab can drive it against the shared fixed model: no login, no GitHub round trip, just a base URL and a key.
In the lab it runs the same fixed model every other tool runs, so the only variable under study is the agent itself.

The Dockerfile installs copilot from npm, not from a source checkout, so the image never depends on a clone of the repo:

```dockerfile
FROM tomolab-base
ARG COPILOT_VERSION=1.0.70
RUN npm install -g @github/copilot@${COPILOT_VERSION}
COPY adapter.sh /usr/local/bin/adapter
```

The npm package is `@github/copilot`, the installed binary is `copilot`, and the captured version is `1.0.70`.
copilot sits in the middle of the suite on memory: the `00-hello` run peaks at 377 MB resident.

### At a glance

| Property | Value |
| --- | --- |
| Runtime | Node 22, from the shared `tomolab-base` image |
| Install source | npm package `@github/copilot`, binary `copilot` |
| Version captured | `1.0.70` (Dockerfile `COPILOT_VERSION`) |
| Wire dialect | OpenAI chat-completions, BYOK provider |
| How the lab invokes it | `copilot -p "$prompt" --allow-all --no-color --log-level none` |
| Provider config | `COPILOT_PROVIDER_*` env vars; no GitHub auth when a base URL is set |
| Where it writes | `/work` for edits, `/trace` for config, stdout, and the time report |
| Peak memory (00-hello) | 377 MB |
| Install footprint | 418 MB |

### Tools and features

The agent turn hands the model seventeen tools, the widest surface of any wired agent, taken from the recovered agent prompt and the `00-hello` request body.

| Tool | What it does |
| --- | --- |
| `bash` | Runs a shell command, sync or async, with backgrounding |
| `read_bash` | Reads output from a running or finished shell |
| `list_bash` | Lists shell sessions |
| `stop_bash` | Stops a shell session |
| `view` | Reads a file or a range of lines |
| `create` | Creates a new file |
| `edit` | Edits an existing file in place, batching edits per file |
| `glob` | Matches paths by pattern (ripgrep-backed) |
| `grep` | Searches file contents across the tree (ripgrep-backed) |
| `sql` | Queries the per-session SQL database |
| `session_store_sql` | Stores operational data in the session database |
| `task` | Delegates work to a sub-agent |
| `list_agents` | Lists available sub-agents |
| `read_agent` | Reads a sub-agent's result |
| `skill` | Loads a named skill on demand |
| `web_fetch` | Fetches a URL |
| `fetch_copilot_cli_documentation` | Fetches copilot's own docs before answering questions about itself |

The `00-hello` run exercises none of them: a greeting needs no tools, so the model answers in one turn with zero tool calls.

## Say Hi!

The `00-hello` scenario is the smallest run in the suite.
The prompt is `Hi!` and the checker asks only that a greeting round trip completed.
Here is the run end to end, from the newest trace (`20260711T100651Z`).

The adapter reads the prompt from the read-only scenario mount, then exports the BYOK provider env vars, pointing the base URL at the trace proxy, not the real upstream:

```bash
export COPILOT_PROVIDER_BASE_URL="${LAB_BASE_URL}"
export COPILOT_PROVIDER_API_KEY="${OPENCODE_API_KEY}"
export COPILOT_PROVIDER_TYPE="openai"
export COPILOT_PROVIDER_WIRE_API="completions"
export COPILOT_MODEL="${LAB_MODEL}"
export COPILOT_PROVIDER_MODEL_ID="${LAB_MODEL}"
```

It records the provider settings to `/trace/config.json` so the run captures exactly what copilot was told, then runs copilot once, non-interactively:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  copilot -p "$prompt" --allow-all --no-color --log-level none \
  >/trace/stdout.log 2>/trace/stderr.log
```

copilot builds its turn from the provider config: the baked-in agent system prompt, the user message `Hi!`, the seventeen tool schemas, all on the chat wire, with the lab's forced decoding (`temperature` 0, `top_p` 1, `seed` 7, `stream` true, and the proxy's `max_tokens` floor).

### Why a bare hello made 2 requests and 1 model call

At the proxy the run lands as two records, a health probe and one completion:

| seq | Method and path | Role | Messages | Tools |
| --- | --- | --- | --- | --- |
| 1 | `GET /zen/` | health probe | none | none |
| 2 | `POST /zen/v1/chat/completions` | agent | `system`, `user` | 17 |

Record 1 is copilot checking the provider is reachable before it starts.
Record 2 is the agent turn: the 23383-char agent system prompt, the user `Hi!`, all seventeen tools, `tool_choice: auto`.
A greeting needs no tools, so the agent makes zero tool calls, streams its reply, and exits.
The prompt is the longest in the suite, which is why copilot's prompt-token count is high on a run this trivial.

### The numbers

| Metric | Value |
| --- | --- |
| Passed | true, on attempt 1 of 3 allowed |
| Proxy records | 2 (1 `GET`, 1 `POST`) |
| Model calls | 1 |
| Tool calls | 0 |
| Plan calls | 0 |
| Subagents | 0 |
| Prompt tokens | 12636 |
| Completion tokens | 36 |
| Total tokens | 12672 |
| TTFB | 931 ms |
| Total (agent call) | 9397 ms |
| Peak RSS | 377 MB |
| Install footprint | 418 MB |
| Wall clock | 0:10.80 |

The recorded reply on stdout is verbatim:

```
Hi! How can I help you today?
```

The checker grades a pass with the verdict `baseline greeting round trip completed`.

## Architecture

### The container

The image is built from `tomolab-base`, which already carries the Node 22 copilot needs.
The Dockerfile does one install step and installs the adapter as the entrypoint:

```dockerfile
FROM tomolab-base
ARG COPILOT_VERSION=1.0.70
RUN npm install -g @github/copilot@${COPILOT_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

There is no copilot source in the image; `@github/copilot` is a self-contained npm binary.

### Mounts

The harness mounts three directories into the container.

| Mount | Access | Purpose |
| --- | --- | --- |
| `/work` | read-write | The scenario's working tree and the agent's cwd; the tree the checker grades |
| `/scenario` | read-only | The scenario definition, holds `prompt.txt` |
| `/trace` | read-write | Where the config, stdout, stderr, exit code, and time report land |

### Harness environment

The harness passes `LAB_BASE_URL` (exported as `COPILOT_PROVIDER_BASE_URL`), `LAB_MODEL` (exported as both `COPILOT_MODEL` and `COPILOT_PROVIDER_MODEL_ID`), `OPENCODE_API_KEY` (exported as `COPILOT_PROVIDER_API_KEY`), and `LAB_MAX_TURNS` (present for parity; copilot's `-p` mode works the task to completion rather than a turn loop the lab caps).
Two more env vars are fixed by the adapter, not the harness: `COPILOT_PROVIDER_TYPE=openai` and `COPILOT_PROVIDER_WIRE_API=completions`, which select the plain chat-completions dialect the proxy speaks.

### The adapter step by step

The adapter is the container entrypoint and the only copilot-specific glue in the lab.
It reads the prompt, exports the six BYOK provider env vars pointed at the proxy, records the provider settings to `/trace/config.json`, then runs copilot once under `/usr/bin/time -v` so the harness can read peak resident set back from the GNU time report.

The flags matter:

- `-p` is copilot's headless one-shot mode: a single prompt, then exit.
- `--allow-all` turns on every permission at once (tools, paths, urls), copilot's all-allow policy and what `-p` needs to run without stopping to confirm; the container is the sandbox.
- `--no-color` keeps the log plain.
- `--log-level none` keeps copilot's own diagnostics out of the trace.

The adapter records copilot's real exit status to `/trace/exit_code` and then always `exit 0`s, so a nonzero agent exit does not crash the container before the trace is written.

### How copilot reaches the proxy

copilot never knows it is being traced.
The BYOK provider reads its base URL and key from the `COPILOT_PROVIDER_*` env vars and sends ordinary chat-completions requests to that base URL, which is the proxy.
Because a provider base URL is set, copilot skips GitHub authentication entirely, which is what lets it run headless against the lab's fixed model.
The proxy normalizes each completion to the chat-completions shape, tees the request body, streamed response, and token usage into `/trace`, and forwards to the real upstream with the real key.
The wire is plain OpenAI chat-completions end to end, so the proxy records it on its untagged chat path with no dialect translation.

## System Prompts

copilot's prompt is its own baked-in system prompt, recovered verbatim by `lab prompts copilot`, not something the lab injects.
The lab injects nothing into the prompt; it only redirects the provider base URL so the proxy can record what copilot already sends.
It is the longest single prompt in the suite, 23383 characters, and the most detailed about how it wants its tools used: when to background a bash command, how to batch reads, when to use the session SQL database instead of a scratch markdown file, when to delegate to a sub-agent and when not to.
Full verbatim text, byte counts, and request counts are at [/prompts/copilot/](/prompts/copilot/).

The volatile spans are runtime-substituted with the version, the model name, and the environment block, and they are the parts worth ignoring when diffing captures:

```text
<version_information>Version number: 1.0.70</version_information>

<model_information>Powered by <model name="nemotron-3-ultra-free" id="nemotron-3-ultra-free" />.
```

Everything above those is fixed policy: the non-interactive contract (work until done, make reasonable assumptions, never stop to ask), the parallel-tool-calling rule, the surgical-code-change rules, the security prohibitions, and the per-tool guidance that makes up the bulk of the text.
