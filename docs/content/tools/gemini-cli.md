---
title: "gemini-cli"
description: "gemini-cli is Google's Gemini CLI, a Node command driven headless with gemini -p and speaking the Gemini generateContent wire that the proxy shims to chat-completions."
weight: 70
---

gemini-cli is Google's Gemini CLI, the `gemini` command from `github.com/google-gemini/gemini-cli`, distributed on npm as `@google/gemini-cli`.
The lab runs it as one of the coding agents it grades against the same shared model as everything else.
It drives the tool through a small adapter that runs `gemini -p "<prompt>"` once per scenario and grades whatever the agent leaves in `/work`.
Its wire is Google's `generateContent` dialect, not chat-completions, so the trace proxy translates Gemini to chat and back at its edge, and gemini-cli talks its native wire and never knows.

## Overview

gemini-cli is a Node command line agent installed from npm.
The Dockerfile builds on the shared `tomolab-base` image and runs `npm install -g @google/gemini-cli@${GEMINI_CLI_VERSION}`, a Node launcher, so the tool image does not depend on any checkout on the host.
The base already carries Node 22, which the launcher needs.
The version the lab captured is pinned in the Dockerfile build arg.

```dockerfile
FROM tomolab-base
ARG GEMINI_CLI_VERSION=0.52.0-nightly.20260710.ga4c91ce19
RUN npm install -g @google/gemini-cli@${GEMINI_CLI_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

Run bare, `gemini` opens an interactive REPL.
The lab uses the headless one-shot mode instead: `-p` runs a single prompt non-interactively and exits.
The adapter pins the model, auto-approves tool calls, and trusts the workspace on the same command line.

```bash
gemini -m "$LAB_MODEL" --approval-mode yolo --skip-trust -p "$prompt"
```

At a glance:

| Property | Value |
| --- | --- |
| Runtime | Node 22 launcher on `tomolab-base`, run under podman |
| Install source | npm, `@google/gemini-cli` (global install) |
| Version captured | `0.52.0-nightly.20260710.ga4c91ce19` (Dockerfile build arg) |
| Wire dialect | Google Gemini `generateContent`, shimmed to chat-completions at the proxy |
| Model | `deepseek-v4-flash-free`, the shared free model, passed with `-m` |
| How the lab invokes it | `gemini -m "$LAB_MODEL" --approval-mode yolo --skip-trust -p "$prompt"` |
| Where it writes | `/work` (the agent's cwd), trace to `/trace` |
| Auth | API key via `GEMINI_API_KEY`, sent as `x-goog-api-key` |

The captured request advertises fifteen tools, grounded in the recovered prompt and the wire body:

| Tool | Purpose |
| --- | --- |
| `list_directory` | list files and subdirectories at a path |
| `read_file` | read file content, with line ranges and truncation |
| `glob` | find files by glob pattern, newest first |
| `grep_search` | regex search across file contents, capped at 100 matches |
| `replace` | exact-string edit with surrounding context |
| `write_file` | write content to a file |
| `run_shell_command` | run a shell command as `bash -c` |
| `list_background_processes` | list background shells the agent started |
| `read_background_output` | read a background process log |
| `web_fetch` | fetch and process URL content |
| `google_web_search` | web search via the Gemini API |
| `update_topic` | publish a narrative topic update for multi-step work |
| `enter_plan_mode` | read-only plan phase for ambiguous or cross-cutting work |
| `invoke_agent` | delegate a subtask to a named sub-agent |
| `activate_skill` | pull a named skill's instructions into context on demand |

Honest read from the sweep: gemini-cli passes 5 of 14 real scenarios.
It is the only wired tool that does not plan, and it makes only 2 to 3 requests per scenario, so it rarely retries and drops the multi-step tasks.
The wiring itself is sound.
The Gemini to chat-completions translation works end to end, and the greeting scenario that exercises the simplest slice of the wire passes cleanly.
The gap is behavioral, not a broken adapter: the agent answers in too few turns and does not carry the harder tasks to completion.

## Say Hi!

The 00-hello scenario hands gemini-cli the single prompt `Hi!` and checks that a greeting round trip completes.
This walks the newest run, `20260711T045225Z`, end to end.

The adapter reads the prompt from the read-only scenario mount.

```bash
prompt="$(cat /scenario/prompt.txt)"
```

It points gemini-cli at the proxy as a Gemini endpoint.
The SDK builds its URL as `{base}/v1beta/models/{model}:generateContent`, so the base must not carry the `/v1` suffix the OpenAI-shaped tools use.
The adapter strips it, and sets the upstream credential as the Gemini API key.

```bash
export GOOGLE_GEMINI_BASE_URL="${LAB_BASE_URL%/v1}"
export GEMINI_API_KEY="${OPENCODE_API_KEY}"
```

For this run the effective config recorded in `/trace/config.env` is:

```text
GOOGLE_GEMINI_BASE_URL=http://tomolab-proxy-1:8080
model=deepseek-v4-flash-free
```

Two settings otherwise block a headless run, so the adapter writes them before the call.

```json
{
  "security": {
    "auth": { "selectedType": "gemini-api-key" },
    "folderTrust": { "enabled": false }
  }
}
```

Pinning `selectedType` to `gemini-api-key` stops gemini-cli from waiting on an interactive auth choice.
Disabling `folderTrust` stops the folder-trust feature from downgrading `--approval-mode yolo` back to prompting.
Then the adapter runs the agent once, non-interactively, from `/work`, wrapped in GNU time.

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  gemini -m "${LAB_MODEL}" --approval-mode yolo --skip-trust -p "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

gemini-cli builds a three-message `generateContent` request rather than two.
First a system message with the agent prompt, whose first line is "You are Gemini CLI, an autonomous CLI agent specializing in software engineering tasks."
Then a user message it injects itself, a `<session_context>` block that opens "This is the Gemini CLI. We are setting up the context for our chat." and records the date Saturday, July 11, 2026, the OS `linux`, and the temp dir `/root/.gemini/tmp/work`.
Then the real user message `Hi!`.
The request also carries all fifteen tool schemas, sent on the Gemini wire.

At the proxy the request lands as a `POST /v1/chat/completions`, tagged `(from gemini)` to mark that it arrived on the Gemini wire and was shimmed to chat, with model `deepseek-v4-flash-free`.
The proxy forces greedy decoding, so the body shows `temperature=0`, `top_p=1`, `seed=7`, and `stream=true` with `stream_options.include_usage=true`.
The full proxy tap for the run is two records, a `GET /zen/` health touch and the one `POST /v1/chat/completions (from gemini)`.

That one request gets one upstream completion, and gemini-cli answers without calling a tool.

| Metric | Value |
| --- | --- |
| passed | `true` (attempt 1 of max 3) |
| requests | 2 (`/zen/` touch + one completion) |
| model_calls | 1 |
| tool_calls | 0 |
| plan_calls | 0 |
| subagents | 0 |
| planned | `false` |
| prompt tokens | 7820 |
| completion tokens | 71 |
| total tokens | 7891 |
| cached tokens | 7808 |
| avg ttfb | 909 ms |
| avg total | 2188 ms |
| timed completions | 1 |
| wall clock | 0:03.10 |
| peak RSS | 243804 KB (about 238 MB) |
| install footprint | 211299 KB (about 206 MB) |

The large 29187-char system prompt shows up as a heavy prompt count that the greeting reply barely adds to, and almost all of it lands cached (7808 of 7820 prompt tokens).

The reply that reached the user, from `stdout.log`, is verbatim:

```text
Hey! How can I help you today? If you've got a project to build, code to debug, or questions about the Gemini CLI, I'm ready.
```

The checker graded the run a pass.
It never reads the model's prose; it confirms the greeting round trip completed, records `check: "baseline greeting round trip completed"`, and marks `passed: true` on the first attempt with `exit_code: 0`.

## Architecture

Enough here to reimplement the setup from scratch.

### Container

The image is `tomolab-base` plus the npm package and the adapter.
The Dockerfile installs `@google/gemini-cli` globally at the pinned `GEMINI_CLI_VERSION`, copies `adapter.sh` to `/usr/local/bin/adapter`, and makes the adapter the container entrypoint.
Everything upstream of the adapter, the network, the trace capture, and the resource accounting, is the same for every tool.

### Mounts

The harness mounts three paths into the container:

| Mount | Mode | Contents |
| --- | --- | --- |
| `/work` | read-write | the scenario's working tree and the agent's cwd |
| `/scenario` | read-only | the scenario definition, holding `prompt.txt` |
| `/trace` | read-write | stdout, stderr, rendered config, time report, exit code |

### Harness environment

The harness passes four variables:

| Variable | Role |
| --- | --- |
| `LAB_BASE_URL` | proxy base URL; the adapter strips a trailing `/v1` for the Gemini SDK |
| `LAB_MODEL` | shared model id, passed with `-m` and read by the proxy to translate |
| `OPENCODE_API_KEY` | upstream credential, exported as `GEMINI_API_KEY` |
| `LAB_MAX_TURNS` | the harness turn budget for the agent loop |

### Adapter, step by step

The adapter is the only gemini-cli-specific glue in the lab.

1. Read the prompt.

```bash
prompt="$(cat /scenario/prompt.txt)"
```

2. Point the SDK at the proxy as a Gemini endpoint, and set the key.
The SDK sends the key as `x-goog-api-key`, which the proxy folds into the bearer it forwards, so the raw key never lands in the trace.

```bash
export GOOGLE_GEMINI_BASE_URL="${LAB_BASE_URL%/v1}"
export GEMINI_API_KEY="${OPENCODE_API_KEY}"
```

3. Write `settings.json` so headless auth is settled and folder trust cannot downgrade YOLO.

```bash
mkdir -p "$HOME/.gemini"
cat >"$HOME/.gemini/settings.json" <<'JSON'
{
  "security": {
    "auth": { "selectedType": "gemini-api-key" },
    "folderTrust": { "enabled": false }
  }
}
JSON
```

4. Record the effective wiring into the trace, never the key itself.

```bash
cat >/trace/config.env <<CFG
GOOGLE_GEMINI_BASE_URL=${GOOGLE_GEMINI_BASE_URL}
model=${LAB_MODEL}
CFG
cp "$HOME/.gemini/settings.json" /trace/settings.json 2>/dev/null || true
```

5. Pin cwd to `/work` and run the one-shot, wrapped in GNU time for the resource numbers.
`-p` is the headless one-shot mode, `--approval-mode yolo` auto-approves every tool call so the agent can act without stopping for approval (the container is the sandbox), `--skip-trust` trusts the workspace so YOLO is not downgraded, and `-m` pins the shared model, which rides in the request URL and is what the proxy reads to translate the call.

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  gemini -m "${LAB_MODEL}" --approval-mode yolo --skip-trust -p "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
status=$?
echo "$status" >/trace/exit_code
exit 0
```

stdout goes to `/trace/stdout.log`, which is the reply the checker and this page read, and stderr goes to `/trace/stderr.log`.
The adapter always exits 0 so the harness grades the result rather than treating a non-zero agent exit as an infrastructure error; the agent's real exit lands in `/trace/exit_code`.

### The agent loop and the wire

gemini-cli runs an agent loop bounded by the turn budget (`LAB_MAX_TURNS`).
The loop sends the conversation to the model, and when the model asks for a tool via native tool-calling, gemini-cli runs it and feeds the result back, until the model answers without a tool call or the budget runs out.
For 00-hello the loop made exactly one model call and zero tool calls, recorded as `model_calls: 1`, `tool_calls: 0`.

The wire is where the proxy does real work.
The SDK posts a Gemini-shaped `generateContent` request to `{base}/v1beta/models/{model}:generateContent`, which the free deepseek model does not understand, since that model is chat-completions only.
So the proxy's Gemini shim translates the `generateContent` request into a chat-completions call on the way out, tees it into the trace, forwards it upstream, and translates the chat response back into a Gemini stream on the way in.
It records the normalized chat form in the trace, tagged `(from gemini)` so the token and byte measurement is the same one every other tool gets.
This is the reason the trace shows `POST /v1/chat/completions (from gemini)` even though gemini-cli only ever spoke Gemini.

The translation works end to end, which is what the passing scenarios confirm.
The limit the sweep exposes is behavioral: gemini-cli does not plan, and in most scenarios it answers in only 2 to 3 requests, so it rarely retries and drops multi-step work, passing 5 of 14.

## System Prompts

This is gemini-cli's own baked-in system prompt, not lab-injected.
It was recovered verbatim by `lab prompts gemini-cli`, which reads the text the proxy captured after normalizing each completion to the chat shape, so it is the exact text that reached the model, not a copy from the tool's source.
The full text is on the [prompts/gemini-cli](/prompts/gemini-cli/) page.

The recovery ran across 19 captured runs (newest `20260710T134116Z`) and found two entries:

| Prompt | Wire | Size | Requests | Tools |
| --- | --- | --- | --- | --- |
| Prompt 1 | `gemini` | 29187 chars | 154 | 15 |
| Prompt 2 | `gemini` | 29187 chars | 1 | 15 |

Both are the same base prompt.
The only difference is the order of the two skills in the `<available_skills>` block: `skill-creator` before `antigravity-support` in Prompt 1, the reverse in Prompt 2.
So this is one prompt with a non-semantic reordering, and the working prompt is the one that served nearly every run, Prompt 1.
Much of the text matches what the public repo ships for Gemini CLI, including the YOLO framing, the Research to Strategy to Execution lifecycle, and the sub-agent and skill scaffolding, which is the cross-check that the recovery is faithful.

The prompt opens by fixing identity and mode.

```text
You are Gemini CLI, an autonomous CLI agent specializing in software engineering tasks. You are currently operating in **YOLO** mode. Your primary goal is to help users safely and effectively.
```

It states outright that the run is headless, which is why the adapter can drive it with no human on the other end.

```text
- **Non-Interactive Environment:** You are running in a headless/CI environment and cannot interact with the user. Do not ask the user questions or request additional information, as the session will terminate.
```

The bulk of the text is a long "Core Mandates" block covering security, context efficiency, and engineering standards.
Security rules forbid logging or committing secrets, forbid staging or committing without an explicit request, and wrap external tool output in `<untrusted_context>` tags to be treated as passive data.
The context-efficiency rules push parallel search and reading, conservative limits on `grep_search` and `read_file`, and fewer turns over smaller reads.
The engineering standards insist on matching workspace conventions, never suppressing warnings or bypassing the type system, verifying a library is present before using it, and adding or updating tests for every change.

A notable framing is the orchestrator model, which is what `invoke_agent` returns into the context.

```text
Operate as a **strategic orchestrator**. Your own context window is your most precious resource. Every turn you take adds to the permanent session history. To keep the session fast and efficient, use sub-agents to "compress" complex or repetitive work.
```

It lists three sub-agents (`codebase_investigator`, `cli_help`, `generalist`) and two skills (`skill-creator`, `antigravity-support`), and defines how a skill loads.

```text
- **Skill Guidance:** Once a skill is activated via `activate_skill`, its instructions and resources are returned wrapped in `<activated_skill>` tags. You MUST treat the content within `<instructions>` as expert procedural guidance, prioritizing these specialized rules and workflows over your general defaults for the duration of the task.
```

On planning, the prompt does describe an `enter_plan_mode` tool and a Research to Strategy to Execution lifecycle, but it scopes plan mode narrowly and tells the agent not to use it for simple work.

```text
If the request is ambiguous, broad in scope, or involves architectural decisions or cross-cutting changes, use the `enter_plan_mode` tool to safely research and design your strategy. Do NOT use Plan Mode for straightforward bug fixes, answering questions, or simple inquiries.
```

That scoping lines up with what the sweep sees: `plan_calls: 0` on the greeting, and no planning across the run, which is the behavior that leaves the harder multi-step scenarios unfinished.

The formatting and tone rules ask for GitHub-flavored Markdown, fewer than three lines of prose per response, and no conversational filler.
The observed 00-hello reply honors that, a single short greeting line.

The volatile span worth ignoring when diffing prompt text is not in the system prompt itself.
It is the separate `<session_context>` user message gemini-cli injects at the start of every run, which carries the current date, the OS, and the temp dir, so it changes run to run.
For the newest 00-hello run that block recorded Saturday, July 11, 2026 and `linux`.
When comparing prompt text across runs, treat the `<session_context>` block as noise and diff the system message.
