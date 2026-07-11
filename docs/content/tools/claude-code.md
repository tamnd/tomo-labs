---
title: "claude-code"
description: "Anthropic's Claude Code CLI, run headless through the Claude Agent SDK and pointed at the lab's trace proxy as if it were the Anthropic API."
weight: 40
---

claude-code is Anthropic's Claude Code CLI, the `claude` binary you install from npm, driven here in its headless single-shot mode.
Anthropic builds it, and in the lab it runs through the Claude Agent SDK that ships inside the same package.
It speaks the Anthropic Messages API on the wire, so the adapter points it at the trace proxy as if that were the Anthropic API, and the proxy shims each Messages call into the chat-completions shape the shared deepseek model speaks.
The tool talks its native wire and never learns that the model behind the endpoint is not a Claude model.

## What it is

The tool under test is the `claude` command from the npm package `@anthropic-ai/claude-code`.
It is the same CLI people run interactively in a terminal, but the lab uses it in headless print mode: one prompt in, the agent works, one result out, then it exits.
Inside, it runs the Claude Agent SDK agent loop, the same loop that powers the interactive REPL, with a large built-in tool schema for reading and writing files, running shell commands, searching the web, spawning subagents, and tracking a task list.
Everything the lab needs from it is reachable without a login flow, because the adapter feeds credentials and the endpoint through environment variables.

## Command surface

The lab uses one invocation of `claude` and a handful of flags, all headless.

Run one prompt non-interactively and print the result:

```bash
claude -p "your prompt here"
```

`-p` (long form `--print`) is the headless switch.
It takes Claude Code out of the interactive REPL and into a single batch run that prints the final result and exits.
The prompt can be passed as an argument or piped in on stdin.

Choose how the result is printed:

```bash
claude -p "your prompt here" --output-format text
```

`--output-format` accepts `text`, `json`, and `stream-json`.
`text` is a plain transcript, which is what the lab wants on stdout.
`json` returns a structured payload with the result, a session id, and cost fields; `stream-json` emits one event per line and needs `--verbose`.

Drop the approval prompts for a fully unattended run:

```bash
claude -p "your prompt here" --dangerously-skip-permissions --output-format text
```

`--dangerously-skip-permissions` bypasses every tool-approval prompt.
Anthropic's own guidance is to use it only in an isolated environment such as a CI container, which is exactly what each run here is.
The safer alternatives Claude Code offers, `--allowedTools` to pre-approve a set and `--permission-mode` to pick a session-wide policy, are not used in the lab, because the throwaway container already provides the isolation the dangerous flag assumes.

Model selection is by environment variable rather than a flag in the lab.
Claude Code reads `ANTHROPIC_MODEL` for the main agent and `ANTHROPIC_SMALL_FAST_MODEL` for the cheap background model, and the adapter sets both to the one shared model.
A `--model` flag exists on the CLI, but the adapter routes the choice through the environment so every request rides `LAB_MODEL`.
Guardrail flags like `--max-turns` and `--max-budget-usd` exist too, but the captured runs do not set them, so they are not part of this tool's surface here.

## How the lab drives it

The whole claude-code-specific glue is one `adapter.sh`, the container entrypoint, plus a `Dockerfile`.
The harness mounts `/work` as the agent's cwd, `/scenario` read-only for `prompt.txt`, and `/trace` for output, and passes `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

First the adapter points Claude Code at the trace proxy as if it were the Anthropic API.
Claude Code joins `ANTHROPIC_BASE_URL` with the path `/v1/messages`, so the adapter strips the trailing `/v1` the harness passes:

```bash
export ANTHROPIC_BASE_URL="${LAB_BASE_URL%/v1}"
export ANTHROPIC_AUTH_TOKEN="${OPENCODE_API_KEY}"
export ANTHROPIC_MODEL="${LAB_MODEL}"
export ANTHROPIC_SMALL_FAST_MODEL="${LAB_MODEL}"
```

`ANTHROPIC_AUTH_TOKEN` sets the raw bearer token Claude Code sends, which is the variable meant for a custom endpoint rather than a real Anthropic key.
Both model variables point at `LAB_MODEL`, so the cheap side tasks Claude Code farms out to the small fast model ride the same model it is graded on.

Then it sets the sandbox escape hatch:

```bash
export IS_SANDBOX=1
```

Claude Code refuses `--dangerously-skip-permissions` when it runs as root, unless it is told it is already inside a sandbox.
Every tool here runs as root in a throwaway container, which is that sandbox, so `IS_SANDBOX=1` lets the skip-permissions flag through.

Next it keeps the run offline except for the model, by disabling the autoupdater, telemetry, error reporting, and other nonessential traffic, and it pre-seeds `~/.claude.json` so the first-run onboarding and folder-trust prompts a headless run can never answer are already dismissed:

```bash
cat >"$HOME/.claude.json" <<JSON
{"hasCompletedOnboarding": true, "bypassPermissionsModeAccepted": true}
JSON
```

Finally it runs the task, wrapped in GNU time so the harness can read peak memory back:

```bash
/usr/bin/time -v -o /trace/time.txt \
  claude -p "$prompt" --dangerously-skip-permissions --output-format text \
  >/trace/stdout.log 2>/trace/stderr.log
```

The `Dockerfile` builds `tomolab-tool-claude-code` on top of `tomolab-base`, so it shares the same toolchain every other tool runs against, including the Node 22 the CLI needs.
It installs the CLI from npm with a build arg:

```dockerfile
FROM tomolab-base
ARG CLAUDE_CODE_VERSION=latest
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}
```

The arg defaults to `latest`, so a build pins whatever version was current at build time.
The captured runs on this page ran Claude Code version `2.1.205.ca0`, which the tool stamps into its own request header, so the exact build is readable straight from the trace.

## Architecture

Claude Code runs the Claude Agent SDK loop.
The CLI hands the model a system prompt and a tool schema, reads back either prose or tool calls, executes the tool calls locally in `/work`, feeds the results back as new messages, and repeats until the model stops calling tools and produces a final answer.
In headless print mode that whole loop runs to completion in one process before `-p` prints the result.

The trace shows the schema is large: 24 tools reach the model on every request.
The captured set is `Agent`, `Bash`, `CronCreate`, `CronDelete`, `CronList`, `Edit`, `EnterWorktree`, `ExitWorktree`, `NotebookEdit`, `Read`, `ReportFindings`, `ScheduleWakeup`, `SendMessage`, `Skill`, `TaskCreate`, `TaskGet`, `TaskList`, `TaskOutput`, `TaskStop`, `TaskUpdate`, `WebFetch`, `WebSearch`, `Workflow`, and `Write`.
`Agent` spawns subagents for fan-out work, `Bash`, `Read`, `Edit`, `Write`, and `NotebookEdit` do the actual file and shell work, and `WebFetch` and `WebSearch` reach the network.
`TaskCreate`, `TaskUpdate`, `TaskGet`, `TaskList`, `TaskOutput`, and `TaskStop` are the built-in planning surface: the agent keeps an explicit task list and moves items through pending, in_progress, and completed as it works, and the system prompt nudges it to use them on multi-step work.
The scheduling tools (`CronCreate`, `CronDelete`, `CronList`, `ScheduleWakeup`, `Workflow`) and `SendMessage` are part of the same schema but do not fire in a one-shot greeting.

On the wire, Claude Code sends the Anthropic Messages API request shape: a `messages` array of role-tagged turns, a separate tool schema, and Anthropic-specific headers.
The free deepseek model behind the proxy speaks chat completions, not Messages, so the proxy's anthropic shim translates in both directions.
It maps the Messages request into a chat-completions call, forwards it upstream, and maps the streamed chat response back into the Messages stream Claude Code expects, flushing as it copies so a streaming reply keeps streaming.
Because the shim normalizes every completion to the chat-completions shape before recording it, the trace tags Claude Code's completion path as `/v1/chat/completions (from messages)`, which marks a request that arrived as Messages and was translated.
The tool talks its native dialect throughout, and its token usage and latency are captured with no cooperation from the tool.

## System prompt

The proxy captured Claude Code's real system prompt across 26 runs, recovered with `lab prompts claude-code`, so this is the text that reached the model rather than a copy from the tool's source.
It comes as two wire `messages`: a main agent prompt of about 7445 chars and a secondary context block of about 5738 chars, both carrying the same 24-tool schema.
Both are grouped from many near-identical per-run renderings that differ only in volatile spans, so the page shows the base prompt rather than a hundred copies.
The full verbatim capture is on the prompt page at [/prompts/claude-code/](/prompts/claude-code/).

The prompt opens by declaring the agent and its wire, right after a billing header the tool prepends:

```text
x-anthropic-billing-header: cc_version=2.1.205.ca0; cc_entrypoint=sdk-cli;You are a Claude agent, built on Anthropic's Claude Agent SDK.
You are an interactive agent that helps users with software engineering tasks.
```

That `cc_version=2.1.205.ca0` and the `sdk-cli` entrypoint are the per-run volatile spans: the version string moves when the pinned build moves, and grouping collapses those renderings into one prompt.

A Harness section tells the model how its output and tools are handled, including how injected context is marked:

```text
 - `<system-reminder>` tags in messages and tool results are injected by the harness, not the user. Hooks may intercept tool calls; treat hook output as user feedback.
```

This is the same convention the Hi! run below shows in practice, where the harness wraps the run's context in a `<system-reminder>` block.

The prompt also carries an Environment section that the proxy captured filled in with the lab's own values:

```text
 - Primary working directory: /work
 - Is a git repository: false
 - Platform: linux
 - You are powered by the model deepseek-v4-flash-free.
```

The working directory, the platform, and the model name are the other volatile spans, rendered fresh each run from the container's environment.
Note that the model line reads `deepseek-v4-flash-free`, the shared lab model, even though the surrounding prompt is Claude Code's own Anthropic-authored text, because the adapter set `ANTHROPIC_MODEL` to the lab model and the tool reflected it back.

The structure matches Claude Code's public behavior: the Agent SDK framing, the harness and permission notes, the memory and task-tracking guidance, and the `<system-reminder>` convention are all documented parts of the CLI.
What is recovered from the trace rather than the docs is the exact wording and the filled-in per-run values, which is the point of capturing it from the proxy instead of transcribing it.

## Hi! end to end

The `00-hello` scenario hands the agent a one-word greeting task and checks that a greeting round trip completed.
Here is that run for claude-code, from the trace.

Claude Code builds one Messages request.
It does not send the user's prompt bare: it injects a `<system-reminder>` context message, so the wire carries three messages with roles `system`, `user`, `system`.
The first system message is Claude Code's agent prompt, opening with the billing header and `You are a Claude agent, built on Anthropic's Claude Agent SDK.`
The user message is the `<system-reminder>` block, which begins `As you answer the user's questions, you can use the following context:` and folds in the current date, `Today's date is 2026-07-10.`
The second system message is the agent-types context, opening `Available agent types for the Agent tool:`.
All 24 tools ride along in the schema.

The proxy taps three records for the run: `GET /zen/`, `HEAD /zen/`, and `POST /v1/chat/completions (from messages)`.
The two `/zen/` records are Claude Code's reachability preflight against the base URL and carry no model call.
The one completion is the Messages request, tagged `(from messages)` because it arrived as Messages and the shim translated it.
Its `model` field is `deepseek-v4-flash-free`, the shared model.

Determinism is forced at the proxy, not by the tool.
The recorded completion request carries `temperature=0`, `top_p=1`, `seed=7`, and `stream=True`, so client-side sampling variance is gone and the greeting is judged under the one decoding regime every tool gets.

The model answers in a single call and Claude Code stops there.
The run made 1 model call, 0 tool calls, and 0 plan calls, so the agent did not open its task list for a plain greeting.
Tokens: 19110 prompt, 68 completion, 19178 total, with 19072 of the prompt tokens served from cache, so the large stable system prompt is almost entirely a cache hit after the first pass.
Latency on that one completion: 7389 ms to first byte and 15122 ms total.
The run used about 297776 KB peak resident set, on an install footprint of about 330456 KB.

The completion prints to stdout as a 167-byte transcript:

```text
Hello! 👋 How can I help you today? Whether you need help with coding, research, planning, or anything else, I'm here for you. Just let me know what you're working on!
```

The checker inspects the working tree, not the prose, and records `baseline greeting round trip completed`.
The run passed on the first attempt.
