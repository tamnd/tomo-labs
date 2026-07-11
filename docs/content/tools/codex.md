---
title: "codex"
description: "OpenAI's open source Codex CLI, driven headless by its codex exec mode over the Responses wire, which the proxy shims to chat."
weight: 20
---

codex is the OpenAI Codex CLI, an open source terminal coding agent from github.com/openai/codex, installed here from the npm package `@openai/codex`.
The lab runs it non-interactively through its `codex exec` mode, one prompt in and one final message out.
codex only speaks the OpenAI Responses wire, so the adapter points it at the trace proxy with `wire_api = "responses"`, and the proxy translates each Responses request into a chat-completions call for the free deepseek model and streams the answer back.
codex talks its native wire the whole time and never learns that a translation happened.

## What it is

codex is a coding agent that runs in your terminal and edits files, runs commands, and iterates until a task is done.
It is a compiled binary shipped through npm, so the image installs it with `npm install -g @openai/codex` and does not need a codex checkout on the host.
It has an interactive TUI for a human at a keyboard and a headless `exec` subcommand for automation.
The lab only uses the headless path, because a benchmark run has no human to answer approval prompts.
Its model, provider, sandbox policy, and approval policy are read from `config.toml` under `$CODEX_HOME`, which defaults to `~/.codex`.

## Command surface

The interactive entry point is bare `codex`, which opens the TUI.
The lab never touches that.
It uses `codex exec`, the non-interactive one-shot mode, which takes the prompt as a positional argument, runs to completion, and exits without ever pausing for approval.

```bash
codex exec "explain this repo"
```

`exec` streams its progress to stderr and prints only the final agent message to stdout, which is exactly what a checker wants: the work lands on disk and the last line of prose lands in a log.

The flags the lab relies on:

```bash
codex exec --sandbox danger-full-access --skip-git-repo-check "$prompt"
```

- `--sandbox <level>` sets filesystem and execution isolation, with levels `read-only`, `workspace-write` (the default), and `danger-full-access`.
  `danger-full-access` removes all restrictions so the agent can write anywhere and run any command, and it also lifts the network block that `workspace-write` imposes by default.
- `--skip-git-repo-check` disables the guard that otherwise refuses to run outside a git repository, so codex will operate in a plain directory.
- In `exec` mode there is no `--ask-for-approval` in play, because the mode is non-interactive and never prompts, so `--sandbox` is the only autonomy control that matters.

Provider and model come from config, not flags.
A custom provider is declared under `[model_providers.<id>]` with a `base_url`, an `env_key` naming the environment variable that holds the API key, and a `wire_api`.
As of recent releases `wire_api` only accepts `responses`, since Codex dropped its chat-completions client, so a chat-only endpoint has to sit behind a translating proxy.

## How the lab drives it

The adapter is the only codex-specific glue in the lab.
Everything upstream of it, the network path, the trace capture, and the resource accounting, is identical to every other tool.
It runs as the container entrypoint, with `/work` mounted as the writable cwd, `/scenario` mounted read-only for `prompt.txt`, and `/trace` for output, and the harness passes in `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

First the adapter writes `~/.codex/config.toml` and defines the provider that sends codex through the proxy:

```toml
model = "${LAB_MODEL}"
model_provider = "lab"

[model_providers.lab]
name = "lab"
base_url = "${LAB_BASE_URL}"
env_key = "OPENCODE_API_KEY"
wire_api = "responses"
```

`base_url` is the trace proxy, so every request, response, and token count is captured with no cooperation from codex.
`wire_api = "responses"` is not a choice, it is the only wire recent codex speaks.
The free deepseek model is chat-only upstream, so the proxy translates the Responses request into a chat request at its edge and the chat reply back into a Responses stream.
`env_key = "OPENCODE_API_KEY"` names the variable codex reads the key from, and the proxy forwards to the real upstream with that key.
The rendered config is copied to `/trace/config.toml` so the exact provider block is preserved with the run.

Then the adapter runs the task:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  codex exec --sandbox danger-full-access --skip-git-repo-check "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

`exec` runs the prompt headless in one shot and never stops for an approval.
`danger-full-access` drops the sandbox so the agent can act freely, which is codex's equivalent of tomo's all-allow policy, safe here because the container is itself the sandbox.
`--skip-git-repo-check` lets it run in `/work`, which is a plain tree rather than a git repo.
The whole thing is wrapped in `/usr/bin/time -v` so the harness reads peak resident memory back out of `/trace/time.txt`.
The exit status is written to `/trace/exit_code`, and the adapter itself always exits 0, because the checker grades the files in `/work`, not the agent's return code.

The image is pinned by a build argument:

```dockerfile
FROM tomolab-base
ARG CODEX_VERSION=latest
RUN npm install -g @openai/codex@${CODEX_VERSION}
COPY adapter.sh /usr/local/bin/adapter
```

`CODEX_VERSION` defaults to `latest`, and the base image already carries Node 22, which the npm launcher needs.

Build and run it with:

```bash
go run ./cmd/lab build codex
go run ./cmd/lab run codex
```

## Architecture

codex runs a standard agent loop: it sends the conversation to the model, the model answers with prose or with function calls, the CLI executes those calls locally, appends the results, and loops until the model stops calling tools and yields a final message.
For a real task that loop turns over many times.
For `Hi!` it turns over once, because the model has nothing to do but greet back.

The wire is the OpenAI Responses API, not chat completions.
That is the whole reason the proxy exists on this path.
codex builds a Responses request and posts it at the proxy; the proxy records it, rewrites it into a `POST /v1/chat/completions`, forwards that to the chat-only deepseek upstream, then folds the chat reply back into a Responses-shaped stream for codex.
In the trace tap that translated call shows up tagged `/v1/chat/completions (from responses)`, which is the proxy's own marker that this record started life as a Responses request.

codex advertises eight tools on every request.
The captured schema, in the order the proxy recorded it, is `exec_command`, `write_stdin`, `update_plan`, `request_user_input`, `view_image`, `get_goal`, `create_goal`, and `update_goal`.

- `exec_command` is how codex touches the world: it runs a shell command in the workspace, and file edits ride on top of it.
  The agent prompt tells the model to edit files with `apply_patch`, invoked as a command array like `{"command":["apply_patch", "*** Begin Patch ..."]}`, so patching flows through `exec_command` rather than being a separate tool.
- `write_stdin` feeds input to a running command, for programs that read from stdin after they start.
- `update_plan` is the plan tool: the model posts an ordered list of short steps, each `pending`, `in_progress`, or `completed`, and the CLI renders it.
  The prompt insists on exactly one `in_progress` step at a time and forbids plans for trivial single-step work.
- `get_goal`, `create_goal`, and `update_goal` are the goal tools, a longer-horizon track of objectives that sits above the step plan.
- `request_user_input` asks the human a question, and `view_image` attaches an image to the context.
  In a headless `exec` run with no human present, `request_user_input` has no one to answer, so a run that needs it would stall rather than pass.

On the `Hi!` run none of these fire.
The trace records `tool_calls: 0`, `plan_calls: 0`, and `subagents: 0`, so the eight tools are offered and none is used.

## System prompt

The proxy captured two distinct system prompts for codex, both on the `responses` wire, both seen across 127 requests, and each advertising the same 8 tools.
codex sends them as a two-message preamble: a large agent prompt of about 20902 characters, then a second system message of about 3185 characters that carries the run's permissions and the installed skills.
The full text of both is on the [codex prompt page](/prompts/codex/).

The first prompt opens by naming the tool and its posture:

> You are a coding agent running in the Codex CLI, a terminal-based coding assistant. Codex CLI is an open source project led by OpenAI. You are expected to be precise, safe, and helpful.

It then spends most of its length on process rules.
It directs the model to send a short preamble before tool calls, to keep a high quality `update_plan` and never pad it with filler steps, and to keep going until the task is fully resolved:

> You are a coding agent. Please keep going until the query is completely resolved, before ending your turn and yielding back to the user. Only terminate your turn when you are sure that the problem is solved.

It pins the edit mechanism to one tool and warns against guessing the name:

> Use the `apply_patch` tool to edit files (NEVER try `applypatch` or `apply-patch`, only `apply_patch`)

The second prompt is the run's live policy, injected fresh rather than baked into the agent text.
It states the sandbox and approval mode this run is under, which is exactly the pair the adapter set:

> `sandbox_mode` is `danger-full-access`: No filesystem sandboxing - all commands are permitted. Network access is enabled.
> Approval policy is currently never.

The rest of the second message is a `<skills_instructions>` block listing the built-in skills codex ships with, such as `imagegen`, `openai-docs`, and `skill-creator`, each with a `SKILL.md` source path under `/root/.codex/skills/.system/`.

Both prompts are recovered from the trace proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model rather than a copy from the tool's source.
The agent prompt's opening and its `apply_patch` and planning rules match the prompt Codex publishes in its public repository, which is the cross-check that this is codex's own text and not something the lab injected.

## Hi! end to end

The `00-hello` scenario hands codex the single prompt `Hi!` and passes if a baseline greeting round trip completes.
codex passed it on the first attempt.

The trace tap shows two records for the whole run: a `GET /zen/`, which is codex's startup health probe against the provider, and one `POST /v1/chat/completions (from responses)`, the single completion.
So codex made exactly one model call to finish, which the orchestration counters confirm with `model_calls: 1` and `requests: 2`.

The request codex built is not just `Hi!`.
It carries four messages, in the order `system`, `system`, `user`, `user`.
The two system messages are the agent prompt and the permissions prompt described above.
Before the user's `Hi!`, codex injects its own user message, an `<environment_context>` block that tells the model where it is running:

```text
<environment_context>
  <cwd>/work</cwd>
  <shell>bash</shell>
  <current_date>2026-07-10</current_date>
  <timezone>Etc/UTC</timezone>
  <filesystem><workspace_roots><root>/work</root></workspace_roots>
  ...
```

Only after that context does the real `Hi!` arrive as the fourth message.

At the proxy every completion is forced onto greedy decoding, so this request went out with `temperature=0`, `top_p=1`, `seed=7`, and `stream=True`.
That is the lab's determinism lever, applied the same way to every tool, so a rerun means the same thing.
The `model` field on the wire is `deepseek-v4-flash-free`, the shared free model every agent in the sweep is judged on.

The numbers on that one call:

- prompt tokens 7554, completion tokens 42, total 7596, with 7424 of the prompt served from cache.
  The large prompt count is the two system messages plus the environment context, and most of it is cached, which is why a greeting costs so little fresh compute.
- time to first byte 914 ms, total 2092 ms, over a single timed completion.
- peak resident set 93464 KB for the agent process, and an install footprint of 433811 KB for the image slice.

The model answered with no tool calls, so the loop ran once and yielded.
The final message codex printed to stdout was 31 bytes:

```text
Hey! How can I help you today?
```

The checker saw a completed greeting round trip and marked the run passed, attempts 1.
