---
title: "opencode"
description: "The sst/opencode terminal coding agent, driven headless through opencode run against the lab's trace proxy."
weight: 30
---

opencode is an open-source terminal coding agent written in TypeScript, built by the team at sst (`github.com/sst/opencode`).
It runs as a TUI by default, but it also has a headless one-shot mode that the lab uses to run a single scenario and exit.
The lab plugs it in with one adapter script and one Dockerfile, and points its model provider at the trace proxy.
opencode speaks OpenAI chat-completions natively through the AI SDK, so nothing in the wire has to be translated: the proxy records it on its plain chat path.

## What it is

opencode is a coding agent you run in your terminal.
It reads and edits files, runs shell commands, searches the tree, and fetches web pages, all under a permission model you can tighten or loosen.
The provider and model are configurable: opencode is not tied to one vendor, and it selects a model with the `provider/model` form.
In the lab it runs the same fixed model every other tool runs, so the only variable under study is the agent itself.
The binary is distributed on npm as the `opencode-ai` package, which is how the Dockerfile installs it, so the image never depends on a checkout of the source.

## Command surface

Without arguments, `opencode` starts the interactive TUI.
The lab never uses that path.
It uses `run`, the non-interactive one-shot mode: opencode takes a single prompt, works until it is idle, and exits.

```bash
# one-shot headless run: a prompt in, files and stdout out
opencode run --model provider/model "your prompt here"
```

The model is always given in `provider/model` form, either on the flag or as a default in config.

```bash
opencode run --model anthropic/claude-sonnet-4-5 "explain this repo"
```

The `run` command carries the flags the lab depends on:

- `--model` (`-m`) picks the model as `provider/model`.
- `--dir` pins the working directory the agent operates in.
- `--auto` approves every permission the run does not explicitly deny, which is what makes an unattended run possible.

Other subcommands exist (`serve` for a headless server, `auth`, `models`, `mcp`, `agent`, `session`, and more), but the lab only needs `run`.
There is no stdin prompt mode: the prompt is a positional argument at the end of the line.

## How the lab drives it

The adapter is the container entrypoint and the only opencode-specific glue in the lab.
Everything upstream of it, the network, the trace capture, the resource accounting, is identical for every tool.
The harness mounts `/work` (the scenario's working tree and the agent's cwd), `/scenario` (read-only, holds `prompt.txt`), and `/trace` (where stdout and the time report land), and passes `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

First the adapter writes opencode's global config at `~/.config/opencode/opencode.json`, registering a custom OpenAI-compatible provider named `lab`:

```json
{
  "$schema": "https://opencode.ai/config.json",
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
```

The `baseURL` is the trace proxy, not the real upstream.
opencode fetches the `@ai-sdk/openai-compatible` package it names at first run, and every request, response, and token count then flows through the proxy with no cooperation from opencode.
The proxy forwards to the real upstream with the real key, and the config is copied to `/trace/config.json` so the run records exactly what opencode was told.

Then the adapter runs the task:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  opencode run --model "lab/${LAB_MODEL}" --dir /work --auto "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

`--model lab/$LAB_MODEL` selects the registered provider and model.
`--dir /work` pins the working tree to the exact tree the checker inspects.
`--auto` approves every permission the run does not explicitly deny, opencode's equivalent of tomo's all-allow policy, so shell scenarios run without a prompt; the container is the sandbox, so the agent acts autonomously and the lab measures whether it finishes.
The whole run is wrapped in `/usr/bin/time -v` so the harness reads peak memory back.

The image is built from `tomolab-base`, the shared base every tool runs against, which already carries Node 22.
opencode is installed with `npm install -g opencode-ai@${OPENCODE_VERSION}`, and `OPENCODE_VERSION` defaults to `latest`.

## Architecture

opencode's headless loop is a straight agentic cycle.
`run` sends the prompt as one user message, the model replies with text and any tool calls, opencode executes the tool calls and feeds the results back, and the loop repeats until the model stops asking for tools and the session goes idle, at which point the process exits.
Tool execution goes through the permission layer, which `--auto` waves through in the lab.

The trace shows the model was handed 10 tools on the main agent turn: `bash`, `edit`, `glob`, `grep`, `read`, `skill`, `task`, `todowrite`, `webfetch`, and `write`.
That set is the whole surface the agent acts through.
`bash` runs shell commands, `read`, `write`, and `edit` handle files, `grep` and `glob` search the tree, and `webfetch` pulls a URL.
`todowrite` is opencode's planning tool: the model writes and updates a task list so a longer job stays structured, though for a trivial task no plan is written at all (the hello run records zero plan calls).
`task` spawns a subagent, and the system prompt steers file search toward it to keep the main context small.
`skill` loads a named skill on demand; the trace's prompt advertises one built-in skill, `customize-opencode`, for editing opencode's own configuration.

The wire is plain OpenAI chat-completions.
opencode's provider is `@ai-sdk/openai-compatible`, which emits standard chat requests, so the proxy records them on its untagged chat path with no dialect translation.
The prompts page labels the wire `chat` for both prompts opencode sends, which is the same thing seen from the proxy's side.

## System prompt

The prompt on this page is recovered from the trace proxy, which records each completion after normalizing it to the chat-completions shape, so it is the exact text that reached the model, not a copy lifted from the source.
The proxy captured two distinct prompts across the run.

The first is the agent prompt: about 9559 characters, seen on 122 requests, carrying all 10 tools.
It opens by naming the tool and its job:

> You are opencode, an interactive CLI tool that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.

Most of the prompt is a brevity policy tuned for a terminal.
It pushes the model to answer in as few tokens as it can:

> IMPORTANT: Keep your responses short, since they will be displayed on a command line interface. You MUST answer concisely with fewer than 4 lines (not including tool use or code generation), unless user asks for detail.

It is strict about not leaving unasked-for traces in the code:

> - IMPORTANT: DO NOT ADD ***ANY*** COMMENTS unless asked

The tail of the prompt is filled in at runtime.
opencode substitutes the model identity ("You are powered by the model named deepseek-v4-flash-free"), an `<env>` block with the working directory and platform, and the list of available skills, so those lines describe the lab's container rather than any fixed default.

The second prompt is much smaller, about 2119 characters, seen on 27 requests, and it is opencode talking to itself.
It is a thread-title generator:

> You are a title generator. You output ONLY a thread title. Nothing else.

opencode names each session with a short title so a conversation is findable later, and it does that with a separate lightweight model call.
That call normally runs on opencode's `small_model`, a cheaper model for lightweight tasks like title generation; when no cheaper model is registered, opencode falls back to the main model.
In the lab only one model is registered, so the title generator runs the same deepseek model as the agent, which is why the side prompt shows up in the trace at all.
The full verbatim text of both prompts, with byte counts and request counts, is at [/prompts/opencode/](/prompts/opencode/).

What matched the public repo: the agent prompt's fixed body is opencode's own published system prompt, down to the feedback line pointing at `github.com/anomalyco/opencode` and the instruction to fetch `https://opencode.ai` when asked about opencode itself.
What is not from the repo is the runtime-substituted tail (model identity, `<env>`, skills list), which the proxy captured as it was actually sent.

## Hi! end to end

The `00-hello` scenario is the smallest run in the suite: the prompt is `Hi!` and the checker asks only that a greeting round trip completed.

opencode's `run` builds the turn and the proxy records three records for the run: a `GET /zen/` health probe and two `POST /zen/v1/chat/completions` calls.
Both completions land on the proxy's plain chat path, `/zen/v1/chat/completions`, with no translation, because opencode's SDK already speaks chat-completions.
Both carry the lab's forced determinism: `temperature=0`, `top_p=1`, `seed=7`, `stream=True`, so the run is judged under one decoding regime.

One of the two model calls is the title generator, not the agent.
Its request body has three messages with roles `system`, `user`, `user`: the system line is "You are a title generator. You output ONLY a thread title. Nothing else.", the first user turn is "Generate a title for this conversation:", and the second is the actual input, "Hi!".
It carries zero tools.
So the title-generator side call does appear in this run's trace, sitting right next to the agent call, which is why the run records two model calls for a one-line prompt.

The agent call itself needs no tools for a greeting, so it makes zero tool calls and writes no plan.
opencode streams the reply to stdout and exits.
The recorded reply is 30 bytes:

```
Hi! How can I help you today?
```

The numbers from `result.json`:

- passed on the first attempt, 1 of 1.
- 3 proxy records, 2 completion POSTs, 2 model calls, 0 tool calls, 0 plan calls, 0 subagents.
- tokens: 7236 prompt, 24 completion, 7260 total, with 7168 of the prompt served from cache.
- latency: about 1023 ms to first byte and 5837 ms total, averaged over the run's timed completions.
- install footprint about 431 MB, peak resident set about 676 MB.
- checker verdict: "baseline greeting round trip completed".

The greeting cost two model calls rather than one, because opencode titled the thread before it answered, and both calls came back clean on the shared deepseek model.
