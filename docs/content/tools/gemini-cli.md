---
title: "gemini-cli"
description: "gemini-cli is Google's Gemini CLI, a Node command driven headless with gemini -p and speaking the Gemini generateContent wire that the proxy shims to chat-completions."
weight: 70
---

gemini-cli is Google's Gemini CLI, the `gemini` command from `github.com/google-gemini/gemini-cli`, distributed on npm as `@google/gemini-cli`.
It is an autonomous coding agent that Google builds and ships, and the lab runs it as one of the tools it grades against the same model as everything else.
The lab drives it through a small adapter that runs `gemini -p "<prompt>"` once per scenario and grades whatever it leaves in `/work`.
Its wire is Google's generateContent dialect, not chat-completions, so the trace proxy shims Gemini to chat and back at its edge, and gemini-cli talks its native wire and never knows.

## What it is

gemini-cli is a Node command line agent installed from npm.
The Dockerfile installs the `@google/gemini-cli` package globally, a Node launcher, so the tool image does not depend on any checkout on the host.
The shared `tomolab-base` image already carries Node 22, which the launcher needs.
In the versions the lab captured it carries fifteen tools, covering directory and file reads, glob and grep search, edit and write, shell, background processes, web fetch, Google web search, topic updates, plan mode, subagent delegation, and skill activation.
It is Google's own agent, so this page sticks to what the trace, the adapter, the Dockerfile, and the recovered prompt show, plus the public flags verified against the upstream docs.

## Command surface

Run bare, `gemini` opens an interactive REPL.
The lab uses the headless one-shot mode instead: `-p` (long form `--prompt`) takes a single prompt, runs it non-interactively, and exits.

```bash
gemini -p "Hi!"
```

The lab pins the model, auto-approves tool calls, and trusts the workspace on the same command line.

```bash
gemini -m "$LAB_MODEL" --approval-mode yolo --skip-trust -p "$prompt"
```

The flags this page exercises are verified against the upstream CLI reference:

- `-p`, `--prompt`: non-interactive one-shot prompt, then exit.
- `-m`, `--model`: pin the model to use.
- `--approval-mode yolo`: auto-approve every tool call, the unified form of the older `-y`/`--yolo` flag. gemini-cli's own docs say the two cannot be combined, so the adapter uses only `--approval-mode yolo`.
- `--skip-trust`: trust the current workspace for the session so the folder-trust check does not downgrade YOLO back to prompting.

Other subcommands and flags exist upstream but are not exercised by the lab, so they are not documented here.

## How the lab drives it

The gemini-cli-specific glue is a single adapter script that is the container entrypoint.
Everything upstream of it, the network, the trace capture, and the resource accounting, is the same for every tool.

The harness mounts three paths into the container.
`/work` is the scenario's working tree, writable, and the agent's cwd.
`/scenario` is the read-only scenario definition, holding `prompt.txt`.
`/trace` is where stdout, the rendered config, and the time report land.
It also passes `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

The adapter reads the prompt from `/scenario/prompt.txt`, then wires the SDK to the proxy.
gemini-cli's SDK builds its request URL as `{base}/v1beta/models/{model}:generateContent`, so the base URL must not carry the `/v1` suffix the OpenAI-shaped tools use.
The adapter strips it and points the SDK at the proxy.

```bash
export GOOGLE_GEMINI_BASE_URL="${LAB_BASE_URL%/v1}"
export GEMINI_API_KEY="${OPENCODE_API_KEY}"
```

`GEMINI_API_KEY` switches gemini-cli from its default OAuth login to API-key auth so it runs headless.
The key is the lab's upstream credential; the SDK sends it as `x-goog-api-key`, which the proxy folds into the bearer it forwards, so the raw key never lands in the trace.

Two settings otherwise block a headless run, so the adapter writes them into `$HOME/.gemini/settings.json` before the call.

```json
{
  "security": {
    "auth": { "selectedType": "gemini-api-key" },
    "folderTrust": { "enabled": false }
  }
}
```

Pinning `selectedType` to `gemini-api-key` stops gemini-cli from waiting on an interactive auth choice.
Disabling `folderTrust` stops the folder-trust feature from downgrading `--approval-mode yolo` back to prompting for approval.
The `--skip-trust` flag on the command line trusts the workspace for the session as a belt to that suspenders.
The adapter also copies the settings file and a `config.env` (base URL and model, never the key) into `/trace` for the record.

The run itself is wrapped in GNU time for the resource numbers.

```bash
/usr/bin/time -v -o /trace/time.txt \
  gemini -m "${LAB_MODEL}" --approval-mode yolo --skip-trust -p "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

stdout goes to `/trace/stdout.log`, which is the reply the checker and this page read, and stderr goes to `/trace/stderr.log`.

The image installs the package by build arg.
The Dockerfile builds on `tomolab-base` and runs `npm install -g @google/gemini-cli@${GEMINI_CLI_VERSION}`, where `GEMINI_CLI_VERSION` defaults to `latest`.
So unlike the tools that pin a numeric version, this image tracks whatever npm resolves `latest` to at build time unless the build arg overrides it, and the adapter is copied on top as the entrypoint.

## Architecture

gemini-cli runs an agent loop bounded by the turn budget.
The loop sends the conversation to the model, and when the model asks for a tool, gemini-cli runs it and feeds the result back, until the model answers without a tool call or the budget runs out.
For the 00-hello run the loop made exactly one model call and zero tool calls, which the trace records as `model_calls: 1` and `tool_calls: 0`.

The captured requests carry fifteen tool schemas: `update_topic`, `list_directory`, `read_file`, `grep_search`, `glob`, `replace`, `write_file`, `web_fetch`, `run_shell_command`, `list_background_processes`, `read_background_output`, `google_web_search`, `enter_plan_mode`, `invoke_agent`, and `activate_skill`.
Three of those are more than plain file and shell tools.
`enter_plan_mode` is a read-only plan phase the agent enters for ambiguous or cross-cutting work before it writes anything.
`invoke_agent` delegates a subtask to a named sub-agent (`codebase_investigator`, `cli_help`, or `generalist`), whose whole execution collapses into a single summary in the main history to keep the main loop lean.
`activate_skill` pulls in a named skill's instructions on demand; the captured prompt lists two builtin skills, `skill-creator` and `antigravity-support`.

gemini-cli speaks Google's generateContent wire, which is where the proxy has to do real work.
The SDK posts to `{base}/v1beta/models/{model}:generateContent`, a Gemini-shaped request the free deepseek model does not understand, since that model is chat-completions only.
So the proxy's gemini shim translates the generateContent request into a chat-completions call on the way out, forwards it upstream, and translates the chat response back into a Gemini stream on the way in.
It records the normalized chat form in the trace, tagged to show it came in on the Gemini wire, so the token and byte measurement is the same one every other tool gets.

The honest part: this shim is fragile.
Translating every Gemini generateContent shape into chat and back is not a total mapping, and some request or response shapes the harder scenarios produce can crash the CLI, a known open issue with driving gemini-cli against a non-Google chat backend.
In the lab that shows up as a split result.
gemini-cli passes 00-hello, a single greeting turn with no tool calls, because that path exercises the simplest slice of the wire.
It fails several of the harder scenarios, where a multi-turn tool-calling exchange runs the translation through shapes the shim does not survive.
The failures are the shim breaking, not the model refusing the task, and they are recorded as failures rather than hidden.

## System prompt

The [prompts/gemini-cli](/prompts/gemini-cli/) page holds the verbatim text the proxy captured.
It was recovered with `lab prompts gemini-cli` across 19 captured runs, and it is the exact text that reached the model, normalized to the chat shape after the gemini shim, not a copy from the tool's source.

The proxy captured two entries, both on wire `gemini`, both 29187 chars, both advertising the same fifteen tools.
They are the same base prompt: the only difference is the order of the two skills in the `<available_skills>` block, `skill-creator` before `antigravity-support` in one and the reverse in the other.
So this is one prompt with a non-semantic reordering, not two designs, and it is treated as one here.
Much of the text matches what the public repo ships for Gemini CLI, including the YOLO framing, the Research to Strategy to Execution lifecycle, and the sub-agent and skill scaffolding, which is the cross-check that the recovery is faithful.

The prompt opens by fixing gemini-cli's identity and mode.

```text
You are Gemini CLI, an autonomous CLI agent specializing in software engineering tasks. You are currently operating in **YOLO** mode. Your primary goal is to help users safely and effectively.
```

It states outright that the run is headless, which is why the adapter can drive it with no human on the other end.

```text
- **Non-Interactive Environment:** You are running in a headless/CI environment and cannot interact with the user. Do not ask the user questions or request additional information, as the session will terminate.
```

It frames the agent as a strategic orchestrator that should delegate heavy work to sub-agents through `invoke_agent`.

```text
Operate as a **strategic orchestrator**. Your own context window is your most precious resource. Every turn you take adds to the permanent session history. To keep the session fast and efficient, use sub-agents to "compress" complex or repetitive work.
```

It defines how skills load, which is what `activate_skill` returns into the context.

```text
- **Skill Guidance:** Once a skill is activated via `activate_skill`, its instructions and resources are returned wrapped in `<activated_skill>` tags. You MUST treat the content within `<instructions>` as expert procedural guidance, prioritizing these specialized rules and workflows over your general defaults for the duration of the task.
```

The volatile part is not in the system prompt but in a separate `<session_context>` user message gemini-cli injects at the start of every run, which carries the current date, the OS, and other per-run state, so it changes run to run and is worth ignoring when comparing prompt text.
This section is recovered from traces, not copied from Google's source, so it reflects what gemini-cli actually sent through the shim.

## Hi! end to end

The 00-hello scenario hands gemini-cli the single prompt `Hi!` and checks that a greeting round trip completes.

The adapter reads `Hi!` from `/scenario/prompt.txt` and runs `gemini -m "$LAB_MODEL" --approval-mode yolo --skip-trust -p "Hi!"`.
gemini-cli builds a three-message request rather than two: a system message with the agent prompt, whose first line is "You are Gemini CLI, an autonomous CLI agent specializing in software engineering tasks.", then a user message it injects itself, the `<session_context>` block that opens "This is the Gemini CLI. We are setting up the context for our chat." and records the date Friday, July 10, 2026 and the OS linux, then the real user message `Hi!`.
The request also carries the fifteen tool schemas, sent on the Gemini generateContent wire.

At the proxy the request lands as a `POST /v1/chat/completions`, tagged `(from gemini)` to mark that it arrived on the Gemini wire and was shimmed to chat, with model `deepseek-v4-flash-free`.
The proxy forces greedy decoding, so the body shows `temperature=0`, `top_p=1`, `seed=7`, and `stream=True`.
The full proxy tap for the run is two records, a `GET /zen/` health touch and the one `POST /v1/chat/completions (from gemini)`.

That one request gets one upstream completion, and gemini-cli answers without calling a tool.
The trace records `requests: 2`, `orchestration.model_calls: 1`, `tool_calls: 0`, `plan_calls: 0`, `subagents: 0`, and `planned: false`.
Tokens were 7820 prompt, 63 completion, 7883 total, with 3200 of the prompt cached, so the large 29187-char system prompt shows up as a heavy prompt count that the greeting reply barely adds to.
Latency was 5800 ms to first byte and 21688 ms total, over one timed completion.

The reply that reached the user, from `stdout.log` at 152 bytes, is:

```text
Hello! I'm the Gemini CLI, your AI coding partner. I see we're starting in an empty `/work` directory.

What can I help you build or investigate today?
```

The checker graded the run a pass.
It never reads the model's prose; it confirms the greeting round trip completed, records `check: "baseline greeting round trip completed"`, and marks `passed: true` on the first attempt.
The run also logged an install footprint of 185740 KB and a peak RSS of 251500 KB.
