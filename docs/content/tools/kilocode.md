---
title: "kilocode"
description: "Kilo Code, an opencode fork, driven headless through kilo run against the lab's trace proxy on the same fixed model as every other agent."
weight: 90
---

Kilo Code is a terminal coding agent built by Kilo-Org, and it is an opencode fork.
It inherits opencode's headless one-shot mode, `kilo run`, which takes a single prompt, works until it goes idle, and exits, and it reads the same custom-provider config, so the lab drives it the same way it drives opencode: one adapter, one Dockerfile, its model provider pointed at the trace proxy.
kilo speaks OpenAI chat-completions natively through the AI SDK, so the wire needs no translation and the proxy records it on its plain chat path.
This page is grounded entirely in the wired image, the adapter, and the newest `00-hello` trace; it claims only what those show.

## Overview

kilo is a coding agent you run in your terminal.
It reads and edits files, runs shell commands, searches the tree, fetches web pages, and manages a todo list, under a permission model you can tighten or loosen.
The provider and model are configurable through a `provider/model` selector, so kilo is not tied to one vendor.
In the lab it runs the same fixed model every other tool runs, so the only variable under study is the agent itself.

The Dockerfile installs kilo from npm, not from a source checkout, so the image never depends on a clone of the repo.
It pins the version explicitly:

```dockerfile
FROM tomolab-base
ARG KILOCODE_VERSION=7.4.5
RUN npm install -g @kilocode/cli@${KILOCODE_VERSION}
COPY adapter.sh /usr/local/bin/adapter
```

The npm package is `@kilocode/cli`, a thin launcher whose postinstall pulls the prebuilt binary for the image platform out of an optional dependency; the installed binary is `kilo`, and the captured version is `7.4.5`.
kilo is one of the heavier wired tools on memory: the `00-hello` run peaks at 574 MB resident, second only to opencode.

### At a glance

| Property | Value |
| --- | --- |
| Runtime | Node 22, from the shared `tomolab-base` image |
| Install source | npm package `@kilocode/cli`, binary `kilo` |
| Version captured | `7.4.5` (Dockerfile `KILOCODE_VERSION`) |
| Wire dialect | OpenAI chat-completions (`@ai-sdk/openai-compatible`) |
| How the lab invokes it | `kilo run --model lab/$LAB_MODEL --dir /work --auto "$prompt"` |
| Provider config | `~/.config/kilo/kilo.json`, a custom `lab` provider |
| Where it writes | `/work` for edits, `/trace` for config, stdout, and the time report |
| Peak memory (00-hello) | 574 MB, second highest of any wired tool |
| Install footprint | 592 MB |

### Tools and features

The agent turn hands the model thirteen tools, taken from the recovered agent prompt and the `00-hello` request body.

| Tool | What it does |
| --- | --- |
| `bash` | Runs a shell command |
| `background_process` | Starts and manages a long-running process |
| `read` | Reads a file |
| `write` | Writes a new file |
| `edit` | Edits an existing file in place |
| `glob` | Matches paths by pattern |
| `grep` | Searches file contents across the tree |
| `webfetch` | Pulls a URL, used to read kilo's own docs when asked about itself |
| `todowrite` | Writes and updates a task list so a longer job stays structured |
| `task` | Spawns a subagent, which the prompt steers file search toward to keep context small |
| `skill` | Loads a named skill on demand; the prompt advertises one, `kilo-config` |
| `suggest` | Offers the user a set of choices |
| `kilo_local_recall` | Recalls project-local context kilo has stored |

The `00-hello` run exercises none of them: a greeting needs no tools, so the model answers in one turn with zero tool calls.

## Say Hi!

The `00-hello` scenario is the smallest run in the suite.
The prompt is `Hi!` and the checker asks only that a greeting round trip completed.
Here is the run end to end, from the newest trace (`20260711T093008Z`).

The adapter reads the prompt from the read-only scenario mount, then writes kilo's config, registering a custom OpenAI-compatible provider named `lab` whose `baseURL` is the trace proxy, not the real upstream:

```json
{
  "$schema": "https://app.kilo.ai/config.json",
  "provider": {
    "lab": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "lab",
      "options": {
        "baseURL": "http://tomolab-proxy:8080/v1",
        "apiKey": "sk-...redacted..."
      },
      "models": {
        "nemotron-3-ultra-free": { "name": "nemotron-3-ultra-free" }
      }
    }
  }
}
```

That file is copied to `/trace/config.json` so the run records exactly what kilo was told.
Then the adapter pins the working tree and runs kilo once, non-interactively:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  kilo run --model "lab/${LAB_MODEL}" --dir /work --auto "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

kilo builds its turn from the config: the baked-in agent system prompt, the user message `Hi!`, the thirteen tool schemas, all on the chat wire, with the lab's forced decoding (`temperature` 0, `top_p` 1, `seed` 7, `stream` true, and the proxy's `max_tokens` floor).

### Why a bare hello made 2 requests and 1 model call

At the proxy the run lands as two records, a health probe and one completion:

| seq | Method and path | Role | Messages | Tools |
| --- | --- | --- | --- | --- |
| 1 | `GET /zen/` | health probe | none | none |
| 2 | `POST /zen/v1/chat/completions` | agent | `system`, `user` | 13 |

Record 1 is kilo checking the provider is reachable before it starts.
Record 2 is the agent turn: the 11341-char agent system prompt, the user `Hi!`, all thirteen tools, `tool_choice: auto`.
A greeting needs no tools, so the agent makes zero tool calls and writes no plan, streams its reply, and exits.
Unlike opencode, kilo does not make a separate title-generator call, so the bare greeting costs one model call, not two.

### The numbers

| Metric | Value |
| --- | --- |
| Passed | true, on attempt 1 of 3 allowed |
| Proxy records | 2 (1 `GET`, 1 `POST`) |
| Model calls | 1 |
| Tool calls | 0 |
| Plan calls | 0 |
| Subagents | 0 |
| Prompt tokens | 10435 |
| Completion tokens | 30 |
| Total tokens | 10465 |
| TTFB | 942 ms |
| Total (agent call) | 1962 ms |
| Peak RSS | 574 MB, second highest of any wired tool |
| Install footprint | 592 MB |
| Wall clock | 0:05.17 |

The recorded reply on stdout is verbatim:

```
Hi! How can I help you today?
```

The checker grades a pass with the verdict `baseline greeting round trip completed`.

## Architecture

### The container

The image is built from `tomolab-base`, the shared base every tool runs against, which already carries the Node 22 kilo needs.
On top of that the Dockerfile does exactly one install step and installs the adapter as the entrypoint:

```dockerfile
FROM tomolab-base
ARG KILOCODE_VERSION=7.4.5
RUN npm install -g @kilocode/cli@${KILOCODE_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

There is no kilo source in the image.
`@kilocode/cli` is a launcher that pulls its prebuilt binary at install, and the `@ai-sdk/openai-compatible` provider it names is fetched at first run.

### Mounts

The harness mounts three directories into the container.

| Mount | Access | Purpose |
| --- | --- | --- |
| `/work` | read-write | The scenario's working tree and the agent's cwd; the tree the checker grades |
| `/scenario` | read-only | The scenario definition, holds `prompt.txt` |
| `/trace` | read-write | Where the config, stdout, stderr, exit code, and time report land |

### Harness environment

The harness passes four environment variables into the adapter: `LAB_BASE_URL` (written into the provider `baseURL`), `LAB_MODEL` (registered as a `lab` model and passed to `--model`), `OPENCODE_API_KEY` (written into the provider `apiKey`), and `LAB_MAX_TURNS` (present for parity, but kilo's headless loop ends on idle rather than a turn cap enforced from outside).

### The adapter step by step

The adapter is the container entrypoint and the only kilo-specific glue in the lab.
It reads the prompt, writes `~/.config/kilo/kilo.json` with the `lab` provider pointed at the proxy, copies that file to `/trace/config.json`, then runs kilo once under `/usr/bin/time -v` so the harness can read peak resident set back from the GNU time report.
`run` is the headless one-shot mode, `--model lab/$LAB_MODEL` selects the `lab` provider and its one model, `--dir /work` pins the working tree the checker inspects, and `--auto` approves every permission the run does not explicitly deny, so shell scenarios run unattended; the container is the sandbox.
The adapter records kilo's real exit status to `/trace/exit_code` and then always `exit 0`s, so a nonzero agent exit does not crash the container before the trace is written.

### How kilo reaches the proxy

kilo never knows it is being traced.
It reads the `lab` provider from config, and the `@ai-sdk/openai-compatible` package sends ordinary chat-completions requests to `baseURL`, which is the proxy.
The proxy normalizes each completion to the chat-completions shape, tees the request body, streamed response, and token usage into `/trace`, and forwards to the real upstream with the real key.
The wire is plain OpenAI chat-completions end to end, so the proxy records it on its untagged chat path with no dialect translation.

## System Prompts

The prompt kilo runs is its own baked-in system prompt, recovered verbatim by `lab prompts kilocode`, not something the lab injects.
The lab injects nothing into the prompt; it only redirects the provider `baseURL` so the proxy can record what kilo already sends.
Because kilo is an opencode fork, the prompt reads like one: the same terminal brevity policy, the same `file_path:line_number` convention, the same batch-your-tool-calls rule.
The proxy captured the base agent prompt twice, byte-identical above a runtime-substituted tail that names the model; the only reason there are two is that two different free models served the runs.
Full verbatim text, byte counts, and request counts are at [/prompts/kilocode/](/prompts/kilocode/).

The tail is the only volatile span, and it is the part worth ignoring when diffing captures:

```text
You are powered by the model named nemotron-3-ultra-free. The exact model ID is lab/nemotron-3-ultra-free
Here is some useful information about the environment you are running in:
<env>
  Is directory a git repo: no
  Platform: linux
  Today's date: Sat Jul 11 2026
  Project config: .kilo/command/*.md, .kilo/agent/*.md, kilo.json, AGENTS.md. Put new commands and agents in .kilo/. Do not use .kilocode/ or .opencode/.
  Global config: /root/.config/kilo/ (same structure)
</env>
```

Everything above that tail is fixed policy: the identity block, the strict tone rules (no "Great", no "Certainly", answers under four lines), the code-style rule against adding comments, and the tool-usage policy that steers file search to the `task` subagent.
