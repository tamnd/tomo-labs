---
title: "codex"
description: "OpenAI's Codex CLI, installed from npm and driven headless through codex exec over the Responses wire, which the proxy shims to chat-completions for the shared free model."
weight: 20
---

codex is the OpenAI Codex CLI, a terminal coding agent from github.com/openai/codex, installed here from the npm package `@openai/codex`.
The lab runs it non-interactively through its `codex exec` mode: one prompt in, one final message out, no approval prompts.
codex only speaks the OpenAI Responses wire, so the adapter points it at the trace proxy with `wire_api = "responses"`, and the proxy translates each Responses request into a chat-completions call for the shared free deepseek model and folds the answer back.
codex talks its native wire the whole time and never learns that a translation happened.

## Overview

codex is a coding agent that runs in a terminal, edits files, runs shell commands, and iterates until a task is done.
It ships as a prebuilt binary through npm, so the image installs it with `npm install -g @openai/codex@0.145.0-alpha.4` and needs no codex checkout on the host.
It has an interactive TUI for a human at a keyboard and a headless `exec` subcommand for automation.
The lab only uses the headless path, because a benchmark run has no human to answer approval prompts.
Its model, provider, sandbox policy, and approval policy are read from `config.toml` under `$CODEX_HOME`, which defaults to `~/.codex`.

The pinned version comes straight from the Dockerfile build argument and is confirmed on the wire by codex's own startup banner, which prints `OpenAI Codex v0.145.0-alpha.4`.

### At a glance

| Property | Value |
| --- | --- |
| Runtime | Node 22 launcher plus prebuilt `codex` binary, run under podman |
| Install source | npm, `npm install -g @openai/codex@${CODEX_VERSION}` |
| Version captured | `0.145.0-alpha.4` (Dockerfile `ARG CODEX_VERSION`, echoed by the startup banner) |
| Wire dialect | OpenAI Responses API (`wire_api = "responses"`) |
| How the lab invokes it | `codex exec --sandbox danger-full-access --skip-git-repo-check "$prompt"` |
| Config it reads | `$HOME/.codex/config.toml`, written fresh by the adapter |
| Working directory | `/work`, pinned with `cd /work` before launch |
| Where it writes | edits and files land in `/work`; stdout, config, and `time -v` report land in `/trace` |
| Provider endpoint | `http://tomolab-proxy:8080/v1`, the trace proxy |
| Autonomy | `--sandbox danger-full-access`, approval policy `never` |

### Features and tools

codex advertises eight tools on every request.
The set is recovered verbatim from the trace, in the order the proxy recorded it.

| Tool | What it does | Grounded in |
| --- | --- | --- |
| `exec_command` | Runs a shell command in the workspace; file edits ride on top of it via `apply_patch` | Prompt "Tool Guidelines", captured schema |
| `write_stdin` | Feeds input to a running command that reads stdin after it starts | Captured schema |
| `update_plan` | Posts an ordered step list, each `pending`, `in_progress`, or `completed`, rendered by the CLI | Prompt "Planning" and "`update_plan`" sections |
| `request_user_input` | Asks the human a question; has no one to answer in a headless run | Captured schema |
| `view_image` | Attaches an image to the model context | Captured schema |
| `get_goal` | Reads the longer-horizon goal track that sits above the step plan | Captured schema |
| `create_goal` | Opens a new goal | Captured schema |
| `update_goal` | Advances or closes a goal | Captured schema |

Editing is not a separate tool.
The agent prompt tells the model to edit files with `apply_patch`, invoked as a command array through `exec_command`:

```json
{"command":["apply_patch","*** Begin Patch\n*** Update File: path/to/file.py\n@@ def example():\n- pass\n+ return 123\n*** End Patch"]}
```

Beyond tools, the recovered prompt bakes in a full working posture: a preamble before tool calls, a planning discipline, root-cause editing conventions, a testing philosophy keyed to the approval mode, and a detailed final-answer formatting spec.
Those are codex's own defaults, not lab settings, and are analyzed in the System Prompts section.

## Say Hi!

The `00-hello` scenario hands codex the single prompt `Hi!` and passes if a baseline greeting round trip completes.
codex passed it on the first attempt.

**Step 1, the adapter reads the prompt.**
The adapter is the container entrypoint.
It reads the scenario prompt off the read-only mount:

```bash
prompt="$(cat /scenario/prompt.txt)"
```

For this scenario that file holds exactly `Hi!`.

**Step 2, point codex at the proxy.**
The adapter writes `~/.codex/config.toml` and defines a custom provider named `lab` whose `base_url` is the trace proxy, then copies the rendered file into the trace for the record:

```toml
model = "deepseek-v4-flash-free"
model_provider = "lab"

[model_providers.lab]
name = "lab"
base_url = "http://tomolab-proxy:8080/v1"
env_key = "OPENCODE_API_KEY"
wire_api = "responses"
```

`base_url` routes every request through the proxy, so requests, responses, and token counts are captured with no cooperation from codex.
`env_key = "OPENCODE_API_KEY"` names the environment variable codex reads the key from; the proxy forwards to the real upstream with it.
`wire_api = "responses"` is not a choice, it is the only wire recent codex speaks.

**Step 3, run codex once, headless.**
The adapter pins the cwd to `/work` and runs `codex exec` under GNU time:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  codex exec --sandbox danger-full-access --skip-git-repo-check "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

`exec` is the one-shot headless mode: a single prompt, run to completion, exit, never pausing for an approval.
`--sandbox danger-full-access` drops the sandbox so the agent can act freely, which is safe because the container is itself the sandbox.
`--skip-git-repo-check` lets it run in `/work`, which is a plain tree rather than a git repo.

**Step 4, codex builds the request.**
codex does not send just `Hi!`.
It builds a Responses request that carries four messages, in the order `system`, `system`, `user`, `user`, plus its eight tool schemas.
The two system messages are the agent prompt and the live permissions prompt.
Before the user's `Hi!`, codex injects its own user message, an `<environment_context>` block telling the model where it is running:

```text
<environment_context>
  <cwd>/work</cwd>
  <shell>bash</shell>
  <current_date>2026-07-11</current_date>
  <timezone>Etc/UTC</timezone>
  <filesystem><workspace_roots><root>/work</root></workspace_roots><permission_profile type="disabled"><file_system type="unrestricted" /></permission_profile></filesystem>
</environment_context>
```

Only after that context does the real `Hi!` arrive as the fourth message.

**Step 5, the proxy translates and forces decoding.**
The proxy records the Responses request, rewrites it into a `POST /v1/chat/completions`, and forwards it to the chat-only deepseek upstream.
In the trace the record is tagged `POST /v1/chat/completions (from responses)`, the proxy's own marker that this started life as a Responses request.
Every completion is forced onto greedy decoding, so the request went out with `temperature=0`, `top_p=1`, `seed=7`, and `stream=true`.
That is the lab's determinism lever, applied identically to every tool, so a rerun means the same thing.
The `model` field on the wire is `deepseek-v4-flash-free`, the shared free model every agent in the sweep is judged on.

**Step 6, one completion, zero tool calls.**
The model answered with prose and no tool calls, so the agent loop ran once and yielded.
The trace shows exactly two records for the whole run: a startup health probe and the single completion.

| Record | Method and path | Status | TTFB | Total |
| --- | --- | --- | --- | --- |
| seq 1 | `GET /zen/` | 200 | 700 ms | 704 ms |
| seq 2 | `POST /v1/chat/completions (from responses)` | 200 | 855 ms | 1982 ms |

The `GET /zen/` is codex's startup probe against the provider; the completion is the only model call.

**Step 7, the numbers.**

| Metric | Value |
| --- | --- |
| passed | true |
| attempts | 1 of 3 |
| requests | 2 |
| model_calls | 1 |
| tool_calls | 0 |
| plan_calls | 0 |
| subagents | 0 |
| prompt tokens | 7562 |
| completion tokens | 44 |
| total tokens | 7606 |
| cache | none reported |
| avg TTFB | 855 ms |
| avg total | 1982 ms over 1 timed call |
| peak RSS | 90056 KB (~88 MB) |
| install footprint | 435049 KB (~425 MB) |
| wall clock | 0:02.86 |

The large prompt count is the two system messages plus the environment context; the completion is a one-line greeting, which is why the round trip costs so little.

**Step 8, the reply.**
The final message codex printed to stdout, verbatim:

```text
Hey there! 👋 How can I help you today?
```

**Step 9, the grade.**
The exit status is written to `/trace/exit_code` (`0`), and the adapter itself always exits 0, because the checker grades the files and output, not the agent's return code.
The checker saw a completed greeting round trip and marked the run passed with check `baseline greeting round trip completed`, attempts 1.

## Architecture

Enough detail here to reimplement the integration from scratch.

### The container

The image is the shared base plus the CLI and the adapter.

```dockerfile
FROM tomolab-base
ARG CODEX_VERSION=0.145.0-alpha.4
RUN npm install -g @openai/codex@${CODEX_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

`tomolab-base` already carries Node 22, which the npm launcher needs.
codex is a prebuilt binary shipped through npm, so the image is independent of any codex checkout on the host.
The entrypoint is `adapter.sh`, so starting the container runs one scenario and exits.

Build and run it with:

```bash
go run ./cmd/lab build codex
go run ./cmd/lab run codex
```

### Mounts

The harness mounts three directories into the container.

| Mount | Mode | Purpose |
| --- | --- | --- |
| `/work` | read-write | the scenario's working tree and the agent's cwd; the checker grades what lands here |
| `/scenario` | read-only | the scenario definition, holds `prompt.txt` |
| `/trace` | read-write | stdout, stderr, rendered config, `time -v` report, exit code |

### Harness environment

The harness passes four environment variables into the container.

| Variable | Role | Used by the adapter |
| --- | --- | --- |
| `LAB_BASE_URL` | proxy endpoint, becomes `base_url` in the provider block | yes, written into `config.toml` |
| `LAB_MODEL` | model id, becomes `model` in `config.toml` | yes |
| `OPENCODE_API_KEY` | upstream key, named by `env_key`; codex reads it, proxy forwards it | yes, by reference through `env_key` |
| `LAB_MAX_TURNS` | turn budget for agent loops | not referenced by this adapter; `exec` runs to natural completion |

### The adapter, step by step

The adapter is the only codex-specific glue in the lab.
Everything upstream of it, the network path, trace capture, and resource accounting, is identical to every other tool.

It reads the prompt off the read-only mount:

```bash
prompt="$(cat /scenario/prompt.txt)"
```

It writes the provider config that sends codex through the proxy and preserves a copy in the trace:

```bash
mkdir -p "$HOME/.codex"
cat >"$HOME/.codex/config.toml" <<TOML
model = "${LAB_MODEL}"
model_provider = "lab"

[model_providers.lab]
name = "lab"
base_url = "${LAB_BASE_URL}"
env_key = "OPENCODE_API_KEY"
wire_api = "responses"
TOML
cp "$HOME/.codex/config.toml" /trace/config.toml 2>/dev/null || true
```

Model selection is by config, not flags: `model` and `model_provider` at the top level point codex at the `lab` provider, and the provider block carries the endpoint, key variable, and wire.
There is no CLI `--model` or `--base-url` in play; the config file is the single source.

It pins the cwd, wraps the run in GNU time for peak memory, and captures both streams:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  codex exec --sandbox danger-full-access --skip-git-repo-check "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?

echo "$status" >/trace/exit_code
exit 0
```

`cd /work` makes the writable tree the agent's cwd.
`codex exec` runs the prompt headless in one shot.
`--sandbox danger-full-access` is codex's autonomy control in exec mode; the other levels are `read-only` and `workspace-write`, and `danger-full-access` removes all filesystem restrictions and lifts the network block that `workspace-write` imposes by default.
There is no `--ask-for-approval` because exec mode is non-interactive and never prompts, so the effective approval policy is `never`, which codex prints in its banner.
`--skip-git-repo-check` disables the guard that otherwise refuses to run outside a git repository.
`/usr/bin/time -v -o /trace/time.txt` lets the harness read peak resident set size back out of the report.
stdout is the final agent message, stderr is the progress stream; both are teed to files.
The exit status is recorded, but the adapter always exits 0, because grading is on the files and output, not the return code.

### How codex reaches the proxy

codex resolves the `lab` provider, reads the key from `OPENCODE_API_KEY`, and posts its Responses request to `LAB_BASE_URL`, which is `http://tomolab-proxy:8080/v1`.
On startup it first hits `GET /zen/` as a health probe, then sends the completion.
The proxy sits inline on the container network, so no codex cooperation is needed to capture traffic.

### The agent loop and native tool calling

codex runs a standard agent loop.
It sends the conversation to the model; the model answers with prose or with function calls; the CLI executes those calls locally, appends the results, and loops until the model stops calling tools and yields a final message.
codex's tool calling is native: it advertises the eight tools listed in the Overview on every request, and shell work and file edits both flow through `exec_command` (edits via `apply_patch`).
For a real task the loop turns over many times.
For `Hi!` it turns over once, because the model has nothing to do but greet back, so the trace records `tool_calls: 0`, `plan_calls: 0`, and `subagents: 0`.

### Wire dialect and proxy translation

The wire is the OpenAI Responses API, not chat completions, which is the whole reason the proxy exists on this path.
The prompt page tags every captured prompt `wire responses`, the adapter sets `wire_api = "responses"`, and the trace confirms it: the completion record is tagged `POST /v1/chat/completions (from responses)`.

The proxy's job on each call:

1. Receive codex's Responses request at `http://tomolab-proxy:8080/v1`.
2. Record it to the trace (`requests.jsonl`), tagging the path `(from responses)`.
3. Rewrite it into a `POST /v1/chat/completions`, mapping the Responses shape onto chat messages and tool schemas.
4. Force greedy decoding: `temperature=0`, `top_p=1`, `seed=7`, `stream=true`.
5. Forward to the chat-only deepseek upstream with the key from `OPENCODE_API_KEY`.
6. Tee the streamed chat reply into the trace (`resp-*.txt`, `usage.jsonl`, `latency.jsonl`) and fold it back into a Responses-shaped stream for codex.

codex sees a well-formed Responses stream and never learns a translation happened.
The determinism knobs make a rerun mean the same thing across every tool in the sweep.

## System Prompts

The prompts below are codex's OWN baked-in system prompt, recovered verbatim by `lab prompts codex`, NOT lab-injected.
The lab injects only the user message (`Hi!` plus codex's own `<environment_context>`) and forces decoding.
Everything else in the system messages is text codex ships and sends on its own.
Full text is on the [codex prompt page](/prompts/codex/).

### What was captured

The proxy captured two distinct system prompts for codex, both on the `responses` wire, both seen across 127 requests over 26 runs, each advertising the same 8 tools.
codex sends them as a two-message preamble, `system` then `system`, ahead of the user turns.

| Prompt | Role | Recovered size | Size on the Hi! run | What it is |
| --- | --- | --- | --- | --- |
| Prompt 1 | agent prompt | ~20902 chars | 20751 chars | codex's working posture: identity, planning, editing, formatting |
| Prompt 2 | permissions and skills | ~3185 chars | 3234 chars | the run's live sandbox/approval policy plus the installed skills list |

Both are the working prompt in the sense that both are sent on every request; Prompt 1 is the durable agent text and Prompt 2 is the live per-run policy.
The small size differences between the recovered figures and this run come from volatile spans, described below.

### Prompt 1: the agent prompt

It opens by naming the tool and its posture:

```text
You are a coding agent running in the Codex CLI, a terminal-based coding assistant. Codex CLI is an open source project led by OpenAI. You are expected to be precise, safe, and helpful.
```

Most of its length is process rules.
It directs the model to send a short preamble before tool calls, to keep a high quality `update_plan` and never pad it with filler steps, and to keep going until the task is fully resolved:

```text
You are a coding agent. Please keep going until the query is completely resolved, before ending your turn and yielding back to the user. Only terminate your turn when you are sure that the problem is solved.
```

It pins the edit mechanism to one tool and warns against guessing the name:

```text
Use the `apply_patch` tool to edit files (NEVER try `applypatch` or `apply-patch`, only `apply_patch`)
```

The prompt breaks into distinct concerns.

| Section | What it governs |
| --- | --- |
| Identity and capabilities | who codex is, what the harness provides, what function calls it can emit |
| AGENTS.md spec | how to discover and obey repo-local instruction files, and their precedence |
| Responsiveness / preambles | short friendly notes before grouped tool calls, with worked examples |
| Planning | when to use `update_plan`, exactly one `in_progress` step, high vs low quality plan examples |
| Task execution | root-cause fixes, minimal focused changes, no unrelated bug fixing, no unrequested commits or license headers |
| Validating your work | testing philosophy keyed to the approval mode; under `never`, proactively run tests and lint |
| Ambition vs precision | be creative on greenfield work, surgical in an existing codebase |
| Presenting your work | brevity by default, natural teammate voice |
| Final answer formatting | detailed spec for headers, bullets, monospace, file references, tone |
| Tool guidelines | prefer `rg`, plus the `update_plan` usage contract |

### Prompt 2: permissions and skills

This is the run's live policy, injected fresh by codex rather than baked into the agent text.
It states the sandbox and approval mode, which is exactly the pair the adapter set:

```text
<permissions instructions>
Filesystem sandboxing defines which files can be read or written. `sandbox_mode` is `danger-full-access`: No filesystem sandboxing - all commands are permitted. Network access is enabled.
Approval policy is currently never. Do not provide the `sandbox_permissions` for any reason, commands will be rejected.
</permissions instructions>
```

The rest is a `<skills_instructions>` block listing the built-in skills codex ships, each with a `SKILL.md` source path under `/root/.codex/skills/.system/`: `imagegen`, `openai-docs`, `plugin-creator`, `skill-creator`, and `skill-installer`.

### Volatile spans to ignore when diffing

A few spans change per run and should be ignored when diffing prompts across captures.

| Span | Where | Example |
| --- | --- | --- |
| `<current_date>` | environment context (user message, not system) | `2026-07-11` |
| `<cwd>` and workspace roots | environment context | `/work` |
| session id | codex banner in stderr, not the wire prompt | `019f4f83-ef8b-7a10-ac57-3fc76b3d6807` |
| sandbox/approval wording | Prompt 2, reflects the run's flags | `danger-full-access`, approval `never` |

The environment context is a user message, so it is not part of the system prompt proper, but it is worth noting since its date and cwd shift every run.

Both prompts are recovered from the trace proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model rather than a copy from the tool's source.
The agent prompt's opening and its `apply_patch` and planning rules match the prompt Codex publishes in its public repository, the cross-check that this is codex's own text and not something the lab injected.
