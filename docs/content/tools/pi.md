---
title: "pi"
description: "pi is the earendil-works coding agent CLI, driven headless with pi -p and pointed at the trace proxy through a custom lab provider."
weight: 80
---

## Overview

pi is a coding agent CLI built by earendil-works, published on npm as `@earendil-works/pi-coding-agent` with the binary `pi`.
Its design keeps the core small and pushes everything else into user extensions, so out of the box the agent gets a four-tool set and a plain agent loop.
The lab installs it into a container image, drives it through a small adapter that runs `pi -p "<prompt>"` once per scenario, and grades whatever pi leaves in `/work`.
pi speaks OpenAI chat-completions natively, so the trace proxy records and forwards its requests without translating a dialect.

pi is a lean flat-loop agent.
Its baked-in system prompt is small: on the 00-hello run the whole request measured 1,606 tokens, the second-smallest baseline in the group, just above tomo.
That lean baseline still clears every real scenario: pi passes all 14 of the lab's non-trivial scenarios cleanly.

pi does ship a plan mode, but it never fires in a headless lab run.
Plan mode is a read-only exploration extension gated behind an interactive TUI prompt: it restricts the model to read-only tools, writes a prose plan, then asks the operator whether to execute it.
It is not a plan tool the model calls mid-run, so in a one-shot `pi -p` run there is no prompt to answer and nothing to record.
The 00-hello trace reflects this: `plan_calls: 0` and `planned: false`.

### At a glance

| Property | Value |
| --- | --- |
| Runtime | Node 22, carried by the shared `tomolab-base` image |
| Install source | npm `@earendil-works/pi-coding-agent`, binary `pi`, installed `-g --ignore-scripts` |
| Version captured | `0.80.6` (Dockerfile `ARG PI_VERSION=0.80.6`) |
| Wire dialect | OpenAI chat-completions (`api: "openai-completions"`) |
| How the lab invokes it | `pi -p "<prompt>" --model lab/<model> -a`, one non-interactive call per scenario |
| Where it writes | `/work`, the agent's cwd and the tree the checker inspects |
| Base image | `tomolab-base` |
| Entrypoint | `/usr/local/bin/adapter` (the pi `adapter.sh`) |

### Tools and features

The adapter loads no extension, skill, or package, so pi runs with exactly its four built-in tools.
The set below is what the 00-hello request actually carried, matching the recovered system prompt.

| Tool | What it does |
| --- | --- |
| `read` | Read file contents. Supports text and images, truncates text to 2000 lines or 50KB, takes `offset`/`limit` for large files. |
| `bash` | Execute a bash command in the cwd, returning stdout and stderr. This is also how listing and search happen: the prompt routes `ls`, `rg`, and `find` through `bash`. |
| `edit` | Exact-text replacement on a single file. One call can carry multiple disjoint `edits[]`, each `oldText` matched against the original file. |
| `write` | Create or overwrite a whole file, creating parent directories as needed. |

There is no separate `grep`, `find`, `ls`, or plan tool in the default set.
The prompt tells the model to run search and listing through `bash` and to read files with `read` instead of shelling out to `cat` or `sed`.
Anything beyond the four defaults comes from skills, prompt templates, extensions, or pi packages, none of which the lab loads.

## Say Hi!

The 00-hello scenario hands pi the single prompt `Hi!` and checks that a greeting round trip completes.
This walks the newest run, `20260710T135633Z`, end to end.

### 1. The adapter reads the prompt and points pi at the proxy

The adapter reads the prompt from `/scenario/prompt.txt`, then writes pi's model config so a custom provider named `lab` points at the trace proxy.

```bash
prompt="$(cat /scenario/prompt.txt)"   # "Hi!"

mkdir -p "$HOME/.pi/agent"
cat >"$HOME/.pi/agent/models.json" <<JSON
{
  "providers": {
    "lab": {
      "baseUrl": "${LAB_BASE_URL}",
      "api": "openai-completions",
      "apiKey": "\$OPENCODE_API_KEY",
      "models": [
        { "id": "${LAB_MODEL}", "name": "${LAB_MODEL}",
          "contextWindow": 128000, "maxTokens": 8192 }
      ]
    }
  }
}
JSON
```

The rendered config for this run, copied verbatim into `/trace/config.json`, resolved to:

```json
{
  "providers": {
    "lab": {
      "baseUrl": "http://tomolab-proxy-2:8080/v1",
      "api": "openai-completions",
      "apiKey": "$OPENCODE_API_KEY",
      "models": [
        {
          "id": "deepseek-v4-flash-free",
          "name": "deepseek-v4-flash-free",
          "contextWindow": 128000,
          "maxTokens": 8192
        }
      ]
    }
  }
}
```

The `apiKey` stays the literal string `$OPENCODE_API_KEY`, not the expanded value, so pi interpolates the env var itself at run time and the real key never lands in the trace copy.

### 2. One non-interactive run

The adapter runs pi once, headless, wrapped in GNU time, in `/work`.

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  pi -p "$prompt" --model "lab/${LAB_MODEL}" -a \
  >/trace/stdout.log 2>/trace/stderr.log
```

`-p` runs one prompt and exits instead of opening the TUI, `--model lab/deepseek-v4-flash-free` selects the proxied model, and `-a` trusts the project-local tree so nothing pauses on a trust check.
Because the run is headless, pi's interactive plan-mode prompt is never reachable, so pi stays in its flat loop.

### 3. pi builds the request

pi assembles a chat-completions request: its small baked-in system prompt, the user message `Hi!`, and the four tool schemas (`read`, `bash`, `edit`, `write`).
The user turn arrives as structured content, `[{"type":"text","text":"Hi!"}]`.

### 4. The decoding on the wire

At the proxy the call lands as `POST /zen/v1/chat/completions` with model `deepseek-v4-flash-free`.
This trace predates a harness change: the proxy then pinned decoding, which is why the body carries a fixed seed. Today the proxy passes each tool's own sampling through untouched and records it; repeatability comes from aggregating repeats, not from forcing the sampler.

| Parameter | Value |
| --- | --- |
| `temperature` | `0` |
| `top_p` | `1` |
| `seed` | `7` |
| `stream` | `true` (`stream_options.include_usage: true`) |
| `max_completion_tokens` | `8192` |
| `store` | `false` |

The full proxy tap for the run is two records: a `GET /zen/` health touch, then the one `POST /zen/v1/chat/completions`.

### 5. One completion, zero tool calls

pi answers directly, without calling a tool, so the loop ends after a single model call.
The model streamed 23 reasoning tokens before the visible reply, which the trace counts under completion tokens.

| Metric | Value |
| --- | --- |
| Requests | 2 (`GET /zen/` health, one chat completion) |
| Model calls | 1 |
| Tool calls | 0 |
| Plan calls | 0 (`planned: false`) |
| Prompt tokens | 1,552 |
| Completion tokens | 54 |
| Total tokens | 1,606 (second-smallest baseline, just above tomo) |
| Cached prompt tokens | 1,536 of 1,552 |
| First-byte latency | 10,859 ms (the highest first-byte latency of the group on this run) |
| Total latency | 12,110 ms over one timed completion |
| Peak RSS | 123,380 KB (about 120 MB) |
| Install footprint | 160,225 KB (about 156 MB) |
| Attempts | 1 of 3 |

Almost the entire prompt came back cached, 1,536 of 1,552 tokens, so only 16 tokens were fresh input.
Latency dominated the run: nearly all the wall time was spent waiting for the model to start replying.

### 6. The verbatim reply

The reply that reached `/trace/stdout.log`, 120 bytes, is:

```text
Hello! How can I help you today? Feel free to ask me to read files, run commands, edit code, or anything else you need.
```

### 7. The checker grades a pass

The checker never reads the model's prose.
It confirms the greeting round trip completed, records `check: "baseline greeting round trip completed"`, and marks `passed: true` on the first attempt with `exit_code: 0`.

## Architecture

This is enough to reimplement the pi harness from scratch.

### The container

The image is the shared `tomolab-base` plus pi and the adapter.

```dockerfile
FROM tomolab-base
ARG PI_VERSION=0.80.6
RUN npm install -g --ignore-scripts @earendil-works/pi-coding-agent@${PI_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

The base already carries Node 22, which pi needs.
pi is installed from npm with `--ignore-scripts`, the install pi's own docs prescribe, so the image is independent of any pi checkout on the host.
The version is pinned by `ARG PI_VERSION=0.80.6`, and the adapter is the container entrypoint.

### Mounts

The harness mounts three paths into the container.

| Mount | Mode | Purpose |
| --- | --- | --- |
| `/work` | read-write | The scenario's working tree and the agent's cwd. The checker inspects this after the run. |
| `/scenario` | read-only | The scenario definition. Holds `prompt.txt`. |
| `/trace` | read-write | Where stdout, stderr, the rendered config, and the time report land. |

### Harness environment

The harness passes four environment variables.

| Variable | Role |
| --- | --- |
| `LAB_BASE_URL` | The trace proxy base URL. Becomes the `lab` provider's `baseUrl`. On this run, `http://tomolab-proxy-2:8080/v1`. |
| `LAB_MODEL` | The model id, used both as the provider's model and in `--model lab/<model>`. On this run, `deepseek-v4-flash-free`. |
| `OPENCODE_API_KEY` | The upstream key. Left as a literal in the config so pi interpolates it at run time and the trace never captures it. |
| `LAB_MAX_TURNS` | The turn cap for the agent loop. |

### The adapter, step by step

The adapter is the only pi-specific glue in the lab.
Everything upstream of it, the network, the trace capture, and the resource accounting, is the same for every tool.

First it reads the scenario prompt.

```bash
prompt="$(cat /scenario/prompt.txt)"
```

Then it registers the custom provider by writing `~/.pi/agent/models.json`, which is where pi reads model config.
Pointing `baseUrl` at `LAB_BASE_URL` is what sends pi's traffic to the proxy instead of the real upstream, and `api: "openai-completions"` selects pi's OpenAI chat-completions client.
The same file is copied to `/trace/config.json` for the record.

```bash
cp "$HOME/.pi/agent/models.json" /trace/config.json 2>/dev/null || true
```

Then it pins the cwd to `/work` and runs pi once under GNU time.

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  pi -p "$prompt" --model "lab/${LAB_MODEL}" -a \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?
echo "$status" >/trace/exit_code
exit 0
```

The relevant flags and choices:

| Choice | Effect |
| --- | --- |
| `-p "$prompt"` | Runs one prompt headless, prints the result, and exits. No TUI, so the interactive plan-mode prompt is never reached. |
| `--model "lab/${LAB_MODEL}"` | Names the model as `provider/model`, where `lab` is the provider the adapter registered. |
| `-a` | Trusts the project-local tree for the run, so nothing pauses on a trust check. |
| `cd /work` | Pins the cwd, so pi's tool writes land in the tree the checker grades. `$HOME` holds `~/.pi/agent/models.json`. |
| GNU `time -v` | Captures peak RSS and wall clock into `/trace/time.txt`. |

pi ships no built-in permission system, so in headless mode its tools run with the process's own permissions.
The adapter never approves individual shell commands or file writes: the container is the sandbox, which is pi's equivalent of tomo's all-allow policy.
Output capture is plain redirection: stdout to `/trace/stdout.log` (the reply the checker reads), stderr to `/trace/stderr.log`, and the exit code to `/trace/exit_code`.
The adapter exits `0` regardless, so the harness grades on the trace, not on the adapter's own status.

### The agent loop

pi runs a flat agent loop bounded by `LAB_MAX_TURNS`.
Each turn sends the conversation to the model over native chat-completions.
When the model asks for a tool, pi runs it and feeds the result back; when the model answers with no tool call, the loop ends.
Tool calls are native function calls: the four tool schemas ride in the `tools` array of each request, and the model selects them by name.
For 00-hello the loop made exactly one model call and zero tool calls.

pi stays flat in headless runs by design.
Its plan mode is an interactive read-only extension: it would restrict the model to read-only tools, write a prose plan, and then block on a TUI prompt asking whether to execute.
A `pi -p` run has no interactive surface, so that prompt is never shown, no plan is written, and no plan tool is exposed to the model.
The loop is just model, optional tool, model, until an answer.

### Reaching the proxy and the wire

pi already speaks the proxy's dialect, so nothing is translated.
The provider's `api` is `openai-completions`, and the proxy speaks OpenAI chat-completions at `/v1/chat/completions`.
The proxy normalizes each request to the chat-completions shape, tees a copy into `/trace` (`requests.jsonl`, `resp-N.txt`, `latency.jsonl`, `usage.jsonl`), and forwards it upstream with the real key it holds.
Because pi's traffic is already chat-completions, there is no dialect shim; the 00-hello path is a plain `POST /zen/v1/chat/completions`.

## System Prompts

This is pi's OWN baked-in system prompt, not something the lab injects.
It was recovered verbatim by `lab prompts pi`, which reads the copy the trace proxy captured on the wire, so it is the exact text that reached the model.
The full text lives at [/prompts/pi/](/prompts/pi/).

The proxy captured one distinct prompt across 17 runs (newest `20260710T135900Z`).
It is a single static rendering on wire `chat`, 2,433 chars, seen across 85 requests, advertising four tools: `bash`, `edit`, `read`, `write`.
That is a simpler and smaller picture than tools that ship several versioned prompts: pi sent the same working prompt every run, and it is compact for what it does.

The prompt opens by fixing pi's identity and role.

```text
You are an expert coding assistant operating inside pi, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files.
```

It then names the four tools directly, which is the whole tool surface the model has.

```text
Available tools:
- read: Read file contents
- bash: Execute bash commands (ls, grep, find, etc.)
- edit: Make precise file edits with exact text replacement, including multiple disjoint edits in one call
- write: Create or overwrite files
```

The tool-use rules push search and listing onto `bash` and reading onto `read`, rather than shelling out to `cat` or `sed`.

```text
- Use bash for file operations like ls, rg, find
- Use read to examine files instead of cat or sed.
```

The bulk of the remaining text is edit-tool discipline: keep `oldText` small but unique, match against the original file, and merge nearby changes into one call.

```text
- Use edit for precise changes (edits[].oldText must match exactly)
- When changing multiple separate locations in one file, use one edit call with multiple entries in edits[] instead of multiple edit calls
- Each edits[].oldText is matched against the original file, not after earlier edits are applied. Do not emit overlapping or nested edits. Merge nearby changes into one edit.
- Keep edits[].oldText as small as possible while still being unique in the file. Do not pad with large unchanged regions.
- Use write only for new files or complete rewrites.
```

Formatting rules are two lines: be concise, and show file paths clearly.

```text
- Be concise in your responses
- Show file paths clearly when working with files
```

A large block near the end is pi self-documentation: where the README, docs, and examples live inside the installed package, and to read them only when the user asks about pi itself, its SDK, extensions, themes, skills, or TUI.
This is where the plan-mode extension would be discovered if a user asked, under `examples/extensions/`, but nothing in the prompt exposes a plan tool to the model.

```text
Pi documentation (read only when the user asks about pi itself, its SDK, extensions, themes, skills, or TUI):
- Main documentation: /usr/lib/node_modules/@earendil-works/pi-coding-agent/README.md
- Additional docs: /usr/lib/node_modules/@earendil-works/pi-coding-agent/docs
- Examples: /usr/lib/node_modules/@earendil-works/pi-coding-agent/examples (extensions, custom tools, SDK)
```

The prompt has no explicit safety or refusal policy: safety in the lab comes from the container boundary, not from prompt text.
The flat-loop behavior is implicit in the tool list and the edit rules; there is no plan, todo, or subagent instruction anywhere in the prompt.

The last two lines are volatile and worth ignoring when diffing prompt text.

```text
Current date: 2026-07-10
Current working directory: /work
```

The date rolls each day and the cwd is fixed to `/work`, so a diff that flags only these lines is noise, not a real prompt change.
