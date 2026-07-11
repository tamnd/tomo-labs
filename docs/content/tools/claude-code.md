---
title: "claude-code"
description: "Anthropic's Claude Code CLI, run headless through the trace proxy as if it were the Anthropic API, carrying the largest baseline prompt of any wired tool."
weight: 40
---

claude-code is Anthropic's Claude Code CLI, the `claude` binary you install from npm, driven here in its headless single-shot mode.
Anthropic builds it, and the lab runs the exact same agent loop that powers the interactive REPL, just with one prompt in and one result out.
It speaks the Anthropic Messages API on the wire, so the adapter points it at the lab's trace proxy as if that endpoint were the Anthropic API.
The proxy translates each Messages call into the chat-completions shape the shared deepseek model speaks, and back again, so claude-code talks its native dialect and never learns the model behind the endpoint is not a Claude model.
The one thing that sets it apart on the leaderboard: it carries by far the largest baseline system prompt of any wired tool, about 19k tokens, and nearly all of it is served from cache.

## Overview

The tool under test is the `claude` command from the npm package `@anthropic-ai/claude-code`, pinned in the Dockerfile to version `2.1.207`.
It is the same CLI people run interactively in a terminal, but the lab uses it in headless print mode (`-p`): one prompt in, the agent works, one result out, then it exits.
Inside, it runs the Claude Agent SDK loop with a large built-in tool schema for reading and writing files, running shell commands, searching the web, spawning subagents, and tracking a task list.
Everything the lab needs is reachable without a login flow, because the adapter feeds the endpoint and credentials through environment variables.

The defining trait for the lab: claude-code ships the largest standing prompt of any wired tool.
Its 00-hello run sends about 19,122 prompt tokens before the user has said anything of substance, and 19,072 of those come straight back from cache.
That is the whole product working as designed: a big, stable system prompt that the model provider caches so repeat turns stay cheap.

At a glance:

| Aspect | Value |
| --- | --- |
| Runtime under test | `claude` binary from npm `@anthropic-ai/claude-code` |
| Install source | `npm install -g @anthropic-ai/claude-code@2.1.207` |
| Version captured | `2.1.207` (build `2.1.207.552` in the request billing header) |
| Base image | `tomolab-base` (Node 22) |
| Wire dialect | Anthropic Messages API, `POST /v1/messages` |
| Proxy translation | Messages request shimmed to `POST /v1/chat/completions`, tagged `(from messages)` |
| How the lab invokes it | `claude -p "$prompt" --dangerously-skip-permissions --output-format text` |
| Agent loop | Claude Agent SDK loop, bounded by native tool-calling, one process |
| Working directory | `/work` (agent cwd, writable, graded by the checker) |
| Baseline prompt size | ~19,122 prompt tokens, ~19,072 cached (largest of the wired set) |

The features and tools below are grounded in the recovered system prompt and the adapter, not in Anthropic's marketing.
The prompt ships a 24-tool schema on every request:

| Tool group | Tools | What they do |
| --- | --- | --- |
| File and shell | `Read`, `Write`, `Edit`, `NotebookEdit`, `Bash` | Read and write files, edit in place, run shell commands in `/work` |
| Search and web | `WebFetch`, `WebSearch` | Fetch a URL, run a web search |
| Subagents | `Agent`, `ReportFindings` | Fan out to `Explore`, `Plan`, `general-purpose`, or specialized agents; report back |
| Task tracking | `TaskCreate`, `TaskUpdate`, `TaskGet`, `TaskList`, `TaskOutput`, `TaskStop` | The built-in todo/planning surface: pending, in_progress, completed |
| Scheduling | `CronCreate`, `CronDelete`, `CronList`, `ScheduleWakeup`, `Workflow` | Recurring and deferred work (never fires in a one-shot greeting) |
| Skills and worktrees | `Skill`, `EnterWorktree`, `ExitWorktree`, `SendMessage` | Invoke a named skill, isolate work in a git worktree, message a running agent |

In a plain greeting none of these fire.
The 00-hello run makes one model call, zero tool calls, and never opens its task list, which is exactly what a well-behaved agent should do for "Hi!".

## Say Hi!

The `00-hello` scenario hands the agent a one-word greeting and checks that a greeting round trip completed.
Here is that run for claude-code end to end, from the newest trace (`20260711T044957Z`).

Step 1, the adapter reads the prompt.
The harness has mounted `/scenario` read-only, so the adapter pulls the task text straight from the file:

```bash
prompt="$(cat /scenario/prompt.txt)"   # "Hi!"
```

Step 2, the adapter points claude-code at the proxy.
claude-code appends `/v1/messages` to `ANTHROPIC_BASE_URL`, so the adapter strips the trailing `/v1` the harness passes, then wires the auth token and both model slots to the one shared lab model:

```bash
export ANTHROPIC_BASE_URL="${LAB_BASE_URL%/v1}"
export ANTHROPIC_AUTH_TOKEN="${OPENCODE_API_KEY}"
export ANTHROPIC_MODEL="${LAB_MODEL}"
export ANTHROPIC_SMALL_FAST_MODEL="${LAB_MODEL}"
```

`ANTHROPIC_AUTH_TOKEN` is the raw bearer token variable meant for a custom endpoint, not a real Anthropic key.
Both model variables point at `LAB_MODEL`, so even the cheap side tasks claude-code farms out to the small fast model ride the model it is graded on.

Step 3, the adapter runs claude-code once, headless.
`cd /work` pins the cwd, `-p` runs one prompt and prints the result, `--dangerously-skip-permissions` drops every approval prompt (the container is the sandbox), and `--output-format text` keeps stdout a plain transcript.
GNU time wraps the call so the harness can read peak memory back:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  claude -p "$prompt" --dangerously-skip-permissions --output-format text \
  >/trace/stdout.log 2>/trace/stderr.log
```

Step 4, claude-code builds one Anthropic Messages request.
It does not send the user's prompt bare: it wraps the run's context in a `<system-reminder>` block and folds in the current date.
The wire carries three messages with roles `system`, `user`, `system`, plus the 24-tool schema:

| Position | Role | Size | Content |
| --- | --- | --- | --- |
| 1 | `system` | 5,762 chars | Agent prompt: billing header, identity, harness, memory, environment, context rules |
| 2 | `user` | 309 chars | `<system-reminder>` context block wrapping the literal `Hi!` |
| 3 | `system` | 7,411 chars | Agent-types and skills context for the `Agent` and `Skill` tools |

The user message is not just "Hi!":

```text
<system-reminder>
As you answer the user's questions, you can use the following context:
# currentDate
Today's date is 2026-07-11.

      IMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.
</system-reminder>

Hi!
```

Step 5, the proxy captures three records for the run.
Two are a reachability preflight against the base URL, one is the model call:

| seq | Method and path | Status | ttfb | total | Note |
| --- | --- | --- | --- | --- | --- |
| 1 | `GET /zen/` | 200 | 669 ms | 672 ms | reachability preflight, no model call |
| 2 | `HEAD /zen/` | 200 | 0 ms | 278 ms | reachability preflight, no model call |
| 3 | `POST /v1/chat/completions (from messages)` | 200 | 1,115 ms | 3,378 ms | the one completion, translated from Messages |

The `(from messages)` tag marks a request that arrived as Anthropic Messages and was shimmed into chat-completions before recording.

Step 6, decoding is forced at the proxy, not by the tool.
The recorded completion request carries forced-decoding params so every tool is judged under one regime:

```json
{
  "model": "deepseek-v4-flash-free",
  "max_tokens": 32000,
  "temperature": 0,
  "top_p": 1,
  "seed": 7,
  "stream": true,
  "stream_options": { "include_usage": true }
}
```

Step 7, the model answers in a single call and claude-code stops there.
The headline is the prompt size and the cache:

| Metric | Value |
| --- | --- |
| Requests | 3 (2 preflight, 1 completion) |
| Model calls | 1 |
| Tool calls | 0 |
| Plan calls | 0 |
| Subagents | 0 |
| Prompt tokens | 19,122 |
| Cached tokens | 19,072 (99.7% of the prompt) |
| Completion tokens | 185 |
| Total tokens | 19,307 (largest baseline of any wired tool) |
| ttfb / total | 1,115 ms / 3,378 ms |
| Peak RSS | 295,808 KB (~289 MB) |
| Install footprint | 333,078 KB (~325 MB) |
| Wall time | 4 s |
| Attempts | 1 |

Nearly the entire 19k-token prompt is a cache hit, so the big standing prompt is cheap after the first pass, which is the whole point of it.

Step 8, the completion prints to stdout verbatim:

```text
Hi there! 👋 Great to meet you. I'm your Claude Code assistant, ready to help with software engineering tasks.

What are you working on today? I can help with:

- **Writing or debugging code** — just show me the files
- **Reviewing changes** — use `/code-review` on your working diff
- **Researching a topic** — use `/deep-research` for multi-source investigation
- **Architecting a plan** — describe what you want to build and I'll map it out
- **Running tests or builds** — I can execute commands and iterate
- **Setting up the project** — use `/init` to bootstrap a CLAUDE.md

Or if you have something specific in mind, just ask away!
```

Step 9, the checker grades the working tree, not the prose, and records `baseline greeting round trip completed`.
The run passed on the first attempt, exit code 0.

## Architecture

Enough to reimplement from scratch.

The container.
The image builds on `tomolab-base`, which already carries Node 22, the runtime the CLI needs.
The Dockerfile npm-installs the pinned CLI and drops the adapter in as the entrypoint:

```dockerfile
FROM tomolab-base
ARG CLAUDE_CODE_VERSION=2.1.207
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

The image is independent of any checkout on the host: the tool comes entirely from npm, and the version arg pins the exact build (`2.1.207`).

Mounts.
The harness gives the container three directories:

| Mount | Mode | Purpose |
| --- | --- | --- |
| `/work` | writable | the scenario's working tree and the agent's cwd; the checker grades what lands here |
| `/scenario` | read-only | the scenario definition, `prompt.txt` |
| `/trace` | writable | stdout, stderr, the rendered config, the GNU time report, exit code |

Harness environment.
The harness passes four variables, the same for every tool:

| Variable | Meaning |
| --- | --- |
| `LAB_BASE_URL` | the trace proxy URL, ending in `/v1` |
| `LAB_MODEL` | the shared model id, `deepseek-v4-flash-free` |
| `OPENCODE_API_KEY` | the bearer token the proxy accepts |
| `LAB_MAX_TURNS` | the per-run turn ceiling (claude-code bounds itself by native tool-calling here) |

The adapter, step by step.
It reads the prompt, translates the harness environment into the variables claude-code understands, seeds a config file to skip onboarding, then runs the CLI once under GNU time.

First it points claude-code at the proxy as if it were the Anthropic API.
claude-code joins `ANTHROPIC_BASE_URL` with `/v1/messages`, so the trailing `/v1` from `LAB_BASE_URL` is stripped:

```bash
export ANTHROPIC_BASE_URL="${LAB_BASE_URL%/v1}"
export ANTHROPIC_AUTH_TOKEN="${OPENCODE_API_KEY}"
export ANTHROPIC_MODEL="${LAB_MODEL}"
export ANTHROPIC_SMALL_FAST_MODEL="${LAB_MODEL}"
```

Then it sets the sandbox escape hatch.
claude-code refuses `--dangerously-skip-permissions` when it runs as root, unless it is told it is already inside a sandbox.
Every tool here runs as root in a throwaway container, which is that sandbox:

```bash
export IS_SANDBOX=1
```

Then it keeps the run offline except for the model, so nothing muddies the trace:

```bash
export DISABLE_AUTOUPDATER=1
export DISABLE_TELEMETRY=1
export DISABLE_ERROR_REPORTING=1
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

Then it pre-seeds a settings file at `$HOME/.claude.json`, pinning HOME's config so the first-run onboarding and folder-trust prompts a headless run can never answer are already dismissed, and copies it into the trace:

```bash
cat >"$HOME/.claude.json" <<JSON
{"hasCompletedOnboarding": true, "bypassPermissionsModeAccepted": true}
JSON
cp "$HOME/.claude.json" /trace/config.json 2>/dev/null || true
```

Finally it pins the cwd to `/work` and runs the task once, headless, wrapped in GNU time for peak memory, with stdout and stderr captured and the exit code recorded:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  claude -p "$prompt" --dangerously-skip-permissions --output-format text \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?
echo "$status" >/trace/exit_code
exit 0
```

The adapter always `exit 0`s so the harness collects the trace even when the CLI itself fails; the real exit code lives in `/trace/exit_code`.

How claude-code reaches the proxy.
There is no proxy config inside the tool.
`ANTHROPIC_BASE_URL` is all it takes: claude-code treats the proxy as the Anthropic API, sends Messages requests to it, and the auth token in `ANTHROPIC_AUTH_TOKEN` rides along as the bearer.
The preflight `GET /zen/` and `HEAD /zen/` are claude-code checking the endpoint is reachable before it sends the first real call.

The agent loop.
claude-code hands the model a system prompt and the 24-tool schema, reads back either prose or tool calls, executes the tool calls locally in `/work`, feeds the results back as new messages, and repeats until the model stops calling tools and produces a final answer.
The loop is bounded by native tool-calling and by the turn ceiling; in headless print mode the whole loop runs to completion in one process before `-p` prints the result.
For a plain greeting the loop terminates after one model call with no tool use.

The wire, and how the proxy translates it.
claude-code sends the Anthropic Messages request shape: a `messages` array of role-tagged turns, a separate tool schema, and Anthropic-specific headers including the `x-anthropic-billing-header` that stamps `cc_version`.
The deepseek model behind the proxy speaks chat completions, not Messages, so the proxy's anthropic shim translates in both directions:

1. It maps the Messages request into a chat-completions call, injecting forced decoding (`temperature=0`, `top_p=1`, `seed=7`, `stream=true`).
2. It forwards that call upstream to the shared model.
3. It tees the streamed chat response, recording usage and latency as it copies, and maps the stream back into the Messages stream claude-code expects, flushing as it goes so a streaming reply keeps streaming.

Because the shim normalizes every completion to the chat-completions shape before recording, the trace tags the completion path as `POST /v1/chat/completions (from messages)`.
The tool talks its native dialect throughout, and its token usage and latency are captured with no cooperation from the tool.

## System Prompts

Up front: this is claude-code's own baked-in system prompt, Anthropic-authored text that ships inside the CLI, recovered verbatim by `lab prompts claude-code`.
It is not lab-injected.
The lab adds nothing to it except the endpoint and the model env; every word below is what claude-code itself sent.
Because the proxy records each completion after normalizing it to chat-completions, this is the exact text that reached the model, not a transcription from source.
The full verbatim capture is on the prompt page at [/prompts/claude-code/](/prompts/claude-code/).

What was captured.
Recovered across 26 runs (newest `20260710T133549Z`), the capture groups into three distinct prompts, all on the `messages` wire, all carrying the same 24-tool schema:

| Prompt | Size | Requests | Role in the wire | What it is |
| --- | --- | --- | --- | --- |
| 1, agent prompt | 7,445 chars | 131 | trailing `system` message | agent types for the `Agent` tool plus the user-invocable skills list |
| 2, agent prompt | 5,738 chars | 131 | leading `system` message | the core working prompt: identity, harness, memory, environment, context |
| 3, agent prompt | 775 chars | 6 | mid-conversation `system` turn | a gentle reminder to use the task tools, injected only after work is underway |

Prompt 2 is the working prompt and the largest single block; together with prompt 1's agent-types and skills catalog it is what pushes claude-code's baseline to about 19k tokens, the biggest of any wired tool.

Identity and wire.
Prompt 2 opens with a billing header the tool prepends, then declares the agent:

```text
x-anthropic-billing-header: cc_version=2.1.205.ca0; cc_entrypoint=sdk-cli;You are a Claude agent, built on Anthropic's Claude Agent SDK.
You are an interactive agent that helps users with software engineering tasks.
```

The `cc_version` moves with the pinned build; the newest 00-hello run stamps `cc_version=2.1.207.552`, so this span is volatile and grouping collapses those renderings into one prompt.

Safety and refusal rules.
Right after the identity line the prompt draws the line on security work:

```text
IMPORTANT: Assist with authorized security testing, defensive security, CTF challenges, and educational contexts. Refuse requests for destructive techniques, DoS attacks, mass targeting, supply chain compromise, or detection evasion for malicious purposes. Dual-use security tools (C2 frameworks, credential testing, exploit development) require clear authorization context: pentesting engagements, CTF competitions, security research, or defensive use cases.
```

Tool-use policy and the harness contract.
A Harness section tells the model how its output and tools are handled, including the injected-context convention the Hi! run shows in practice:

```text
# Harness
 - Text you output outside of tool use is displayed to the user as Github-flavored markdown in a terminal.
 - Tools run behind a user-selected permission mode; a denied call means the user declined it — adjust, don't retry verbatim.
 - `<system-reminder>` tags in messages and tool results are injected by the harness, not the user. Hooks may intercept tool calls; treat hook output as user feedback.
 - Prefer the dedicated file/search tools over shell commands when one fits. Independent tool calls can run in parallel in one response.
 - Reference code as `file_path:line_number` — it's clickable.
```

Editing conventions.
A single line sets the house style for code changes:

```text
Write code that reads like the surrounding code: match its comment density, naming, and idiom.
```

Tone, register, and honesty rules.
The prompt tells the model to confirm irreversible actions and to report outcomes without hedging:

```text
For actions that are hard to reverse or outward-facing, confirm first unless durably authorized or explicitly told to proceed without asking; approval in one context doesn't extend to the next. Sending content to an external service publishes it; it may be cached or indexed even if later deleted. Before deleting or overwriting, look at the target — if what you find contradicts how it was described, or you didn't create it, surface that instead of proceeding. Report outcomes faithfully: if tests fail, say so with the output; if a step was skipped, say that; when something is done and verified, state it plainly without hedging.
```

The todo and planning tooling.
The `Task*` tools are the built-in planning surface, and prompt 3 is the mid-conversation nudge that reminds the model to use them once work is underway:

```text
The task tools haven't been used recently. If you're working on tasks that would benefit from tracking progress, consider using TaskCreate to add new tasks and TaskUpdate to update task status (set to in_progress when starting, completed when done). Also consider cleaning up the task list if it has become stale. Only use these if relevant to the current work. This is just a gentle reminder - ignore if not applicable.
```

That reminder only appears once real work exists; for a plain "Hi!" it never fires, which is why the greeting run makes zero plan calls.

The persistent memory system.
The prompt describes a file-based memory the agent writes to directly:

```text
# Memory

You have a persistent file-based memory at `/root/.claude/projects/-work/memory/`. This directory already exists — write to it directly with the Write tool (do not run mkdir or check for its existence). Each memory is one file holding one fact, with frontmatter:
```

Formatting and context management.
The prompt closes by telling the model that long conversations are summarized for it, and to act when it has enough information rather than survey options:

```text
When you have enough information to act, act. Do not re-derive facts already established in the conversation, re-litigate a decision the user has already made, or narrate options you will not pursue. If you are weighing a choice, give a recommendation, not an exhaustive survey
```

Volatile spans worth ignoring when diffing.
An Environment section is rendered fresh each run from the container, filled in with the lab's own values:

```text
# Environment
You have been invoked in the following environment: 
 - Primary working directory: /work
 - Is a git repository: false
 - Platform: linux
 - Shell: unknown
 - OS Version: Linux 7.0.12-201.fc44.aarch64
 - You are powered by the model deepseek-v4-flash-free.
```

These spans move every run and should be ignored when diffing the prompt: the `cc_version` build string, the working directory, the git-status line, the platform and OS version, the date folded into the `<system-reminder>`, and the model id.
The model line reads `deepseek-v4-flash-free`, the shared lab model, even though the surrounding text is claude-code's own Anthropic-authored prompt, because the adapter set `ANTHROPIC_MODEL` to the lab model and the tool reflected it back.
What is recovered from the trace rather than the docs is the exact wording and these filled-in per-run values, which is the point of capturing the prompt from the proxy instead of transcribing it.
