---
title: "hermes"
description: "hermes is Nous Research's Hermes Agent, driven headless with hermes chat -q and speaking native chat-completions to the trace proxy."
weight: 60
---

hermes is Hermes Agent by Nous Research, wired into the lab as one more agent under study.
The public project is real and verifiable: it lives at [github.com/NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent), is MIT licensed, and presents itself as a self-improving personal agent with persistent memory, skills, and subagents.
The lab drives it through a small adapter that runs `hermes chat -q "<prompt>"` once per scenario, points its model base URL at the trace proxy, and grades whatever it leaves in `/work`.
Its wire is native chat-completions, so the proxy records and forwards its requests without translating a dialect.
This page is grounded in the captured run: the trace, the adapter, the Dockerfile, the README, and the recovered prompt, not a reading of Hermes' own source.

## What it is

hermes presents as a general personal agent that also does coding work, which the captured system prompt states in its first line: "You are Hermes Agent, an intelligent AI assistant created by Nous Research."
The prompt points at `https://hermes-agent.nousresearch.com/docs` as its own authoritative reference and describes memory that persists across sessions, skills it can save and patch, and subagents it can delegate to.
It carries 19 tools in the versions the lab captured, covering a terminal, file read and write, a patch tool, code execution, a process tool, search over files and past sessions, a todo planner, subagent delegation, three skill tools, a memory tool, and several media tools.
The lab installs it as the npm package `hermes-agent`, which the adapter README describes as an unofficial bridge whose postinstall step pip installs the upstream Python Hermes Agent, so the real agent is a Python runtime the bridge launches.
That install path is what the Dockerfile and README show; this page does not claim provenance for the npm bridge beyond what those two files state.

## Command surface

The lab uses one entry point, the single-query mode.
`hermes chat -q "<prompt>"` runs one task non-interactively and exits instead of opening the chat REPL.

```bash
hermes chat -q "Hi!" --yolo
```

The adapter configures the model through `hermes config set` before the run rather than passing model flags on the command line.

```bash
hermes config set model.provider custom
hermes config set model.base_url "$LAB_BASE_URL"
hermes config set model.default "$LAB_MODEL"
hermes config set model.api_key "$OPENCODE_API_KEY"
```

The `--yolo` flag auto-approves every action so the shell scenarios can run headless.
The flags this page can verify from the adapter are `-q` for the one-shot prompt and `--yolo` for auto-approval, plus the `config set model.*` keys above.
Other subcommands and flags are not exercised by the lab, so they are not documented here.

## How the lab drives it

The hermes-specific glue is a single adapter script that is the container entrypoint.
Everything upstream of it, the network, the trace capture, and the resource accounting, is the same for every tool.

The harness mounts three paths into the container.
`/work` is the scenario's working tree, writable, and the agent's cwd.
`/scenario` is the read-only scenario definition, holding `prompt.txt`.
`/trace` is where stdout, the rendered config, and the time report land.
It also passes `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.
The adapter reads the prompt from `/scenario/prompt.txt` and does not reference `LAB_MAX_TURNS`, so the turn budget is left to Hermes' own default.

The adapter points Hermes at the proxy in two ways at once.
It exports `OPENAI_API_KEY` and `OPENAI_BASE_URL`, and it also sets the same values into Hermes' config with `hermes config set`.

```bash
export OPENAI_API_KEY="${OPENCODE_API_KEY}"
export OPENAI_BASE_URL="${LAB_BASE_URL}"
hermes config set model.provider custom
hermes config set model.base_url "${LAB_BASE_URL}"
hermes config set model.default "${LAB_MODEL}"
hermes config set model.api_key "${OPENCODE_API_KEY}"
```

`LAB_BASE_URL` is the trace proxy, so setting `model.base_url` routes the model calls straight through the proxy, which forwards to the real upstream with the real key.
The adapter comment notes that the custom provider does not read `OPENAI_API_KEY`, so the key has to live in the config too; without `model.api_key` the proxy forwards an empty bearer and the upstream rejects the one real chat call with 401.
The adapter then copies `$HOME/.hermes/config.yaml` to `/trace/config.yaml` for the record.

It sets `HERMES_YOLO_MODE=1` and passes `--yolo`, which auto-approves every action so the agent acts autonomously.
The README calls this Hermes' equivalent of tomo's all-allow policy; the container is the sandbox, so the agent is let loose and the lab measures whether it can finish.

The run itself is wrapped in GNU time for the resource numbers.

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  hermes chat -q "$prompt" --yolo \
  >/trace/stdout.log 2>/trace/stderr.log
```

stdout goes to `/trace/stdout.log`, which is the reply this page reads, and stderr goes to `/trace/stderr.log`.

The image installs a version pinned by build arg.
The Dockerfile builds on the shared `tomolab-base` image, sets `ENV PIP_BREAK_SYSTEM_PACKAGES=1` so the bridge's pip install lands on Debian bookworm's externally managed Python, and runs `npm install -g hermes-agent@${HERMES_VERSION}` with `ARG HERMES_VERSION=latest`.
The base already carries Node 22 and Python 3.11, which the bridge and the Python agent both need.
The adapter is copied in and set as the entrypoint.

## Architecture

hermes runs an agent loop over the model's native function-calling.
The loop sends the conversation to the model, and when the model asks for a tool, Hermes runs it and feeds the result back, until the model answers without a tool call.
For the 00-hello run the loop made exactly one model call and zero tool calls, which the trace records as `model_calls: 1` and `tool_calls: 0`.

Each request carries all 19 tool schemas, and the model picks one by name.
The tools the trace names are `clarify`, `cronjob`, `delegate_task`, `execute_code`, `image_generate`, `memory`, `patch`, `process`, `read_file`, `search_files`, `session_search`, `skill_manage`, `skill_view`, `skills_list`, `terminal`, `text_to_speech`, `todo`, `vision_analyze`, and `write_file`.

Planning is in-context through the `todo` tool rather than a separate orchestrator.
The coding prompt directs Hermes to track multi-step work with `todo`, and the trace records `plan_calls: 0` for the trivial greeting, so a one-step request skips it.
Subagents are their own tool, `delegate_task`, and the 00-hello run used none, recorded as `subagents: 0`.

Skills are three tools working together: `skills_list` enumerates them, `skill_view` reads one (the prompt tells Hermes to load `skill_view(name='hermes-agent')` for guidance on Hermes itself), and `skill_manage` creates or patches them.
The prompt frames this as a learning loop: after a complex task Hermes is told to save the approach as a skill, and to patch a skill the moment it finds it outdated.
Memory is a separate tool, injected into every turn, for durable facts about the user and environment; the prompt is explicit that task progress and stale artifacts go to `session_search` instead, not to memory.
Editing goes through `patch` and `write_file`, shell and builds through `terminal`, background work through `process`, and code runs through `execute_code`.

The two main prompt sizes suggest two modes, and the trace shows both.
The shorter prompt is a general assistant; the longer one adds a coding-agent block that turns Hermes into "a careful senior engineer" pairing inside a repo.
The trace shows which prompt shipped, not an internal switch, so this page describes only the observed prompts, not the mechanism that selects them.

hermes speaks native chat-completions.
The prompt page tags every hermes prompt as wire `chat`, and the proxy normalizes each request to the chat-completions shape before recording it.
Because Hermes already sends that shape, the proxy does not shim its dialect; it tees a copy into the trace and forwards it upstream.
The 00-hello trace confirms the completion path is `/zen/v1/chat/completions`, a plain chat call with no dialect translation.

## System prompt

The [prompts/hermes](/prompts/hermes/) page holds the verbatim text the proxy captured.
It was recovered with `lab prompts hermes` across 15 captured runs, and it is the exact text that reached the model, not a copy from Hermes' source.
This recovered trace is the ground truth for what the prompt says on this page.

The proxy captured four distinct prompts, all on wire `chat`.
Three are agent prompts and one is a short side prompt.
The general agent prompt runs 7562 chars.
The coding-agent prompt runs 10561 chars, with a near-twin at 10567 chars that differs only in the workspace manifest it names (`go.mod` versus `package.json`), so the difference tracks the scenario's project, not the agent's behavior.

The prompt opens by fixing Hermes' identity and register.

```text
You are Hermes Agent, an intelligent AI assistant created by Nous Research. You are helpful, knowledgeable, and direct.
```

It sets a hard rule against fabricating results, which is the same finishing-the-job discipline the lab grades on.

```text
NEVER substitute plausible-looking fabricated output (made-up data, invented file contents, synthesised API responses) for results you couldn't actually produce. Reporting a blocker honestly is always better than inventing a result.
```

The memory guidance is unusually specific about what does not belong in memory, which is what keeps the injected memory block small.

```text
Do NOT save task progress, session outcomes, completed-work logs, or temporary TODO state to memory; use session_search to recall those from past transcripts.
```

The 10561-char prompt adds the coding-agent block that the shorter prompt does not carry.

```text
You are a coding agent pairing with the user inside their codebase. Operate like a careful senior engineer.
```

The fourth prompt is a 312-char side prompt whose purpose is visible in its text: it asks the model to generate a short 3 to 7 word title for a conversation, returning only the title.
That is a utility call Hermes makes to name the session, separate from the agent loop, which is why it appears on its own with no tools attached.
This section is recovered from traces, not copied from Hermes' source, so it reflects what Hermes actually sent.

## Hi! end to end

The 00-hello scenario hands hermes the single prompt `Hi!` and checks that a greeting round trip completes.

The adapter reads `Hi!` from `/scenario/prompt.txt`, configures the custom provider, and runs `hermes chat -q "Hi!" --yolo`.
Hermes builds the request around it: a system message with the general agent prompt, whose first line is "You are Hermes Agent, an intelligent AI assistant created by Nous Research.", then the user message `Hi!`.
The request also carries all 19 tool schemas, sent on the native chat wire.

Before that one chat call, Hermes probes the endpoint hard.
The proxy tap for this run holds 26 records, of which exactly one is the completion POST; the other 25 are capability probes such as `GET /zen/v1/models`, `GET /zen/api/tags`, `GET /zen/v1/props`, `GET /zen/version`, and `POST /zen/api/show`, as Hermes feels out the custom OpenAI-compatible endpoint.
The completion lands as a POST to `/zen/v1/chat/completions` with model `deepseek-v4-flash-free`.
The proxy forces greedy decoding, so the body shows `temperature=0`, `top_p=1`, `seed=7`, and `stream=True`.
It is a plain chat-completions call with two messages, roles system and user, and no dialect translation.

That one request gets one upstream completion, and Hermes answers without calling a tool.
The trace records `requests: 26`, `orchestration.model_calls: 1`, `tool_calls: 0`, `plan_calls: 0`, `subagents: 0`, and `planned: false`.
Tokens were 13586 prompt, 33 completion, 13619 total, with 7808 cached; the large prompt count is the 7562-char system prompt plus the 19 tool schemas, and the cache hit covers most of it.
Latency was 8136 ms to first byte and 9562 ms total, over one timed completion.

The reply that reached the user, from `stdout.log` at 803 bytes, is:

```text
Hi! How can I help you today?
```

The same log shows Hermes' terminal chrome around that line: a one-time warning that the tirith security scanner is enabled but not available so command scanning falls back to pattern matching, a Reasoning box ("The user is just saying 'Hi!' ... I'll respond warmly and helpfully."), and the Hermes reply panel.

The checker graded the run a pass.
It never reads the model's prose; it confirms the greeting round trip completed, records `check: "baseline greeting round trip completed"`, and marks `passed: true` on the first attempt.
The run ran under the podman runtime, and logged an install footprint of 226651 KB and a peak RSS of 124868 KB.
