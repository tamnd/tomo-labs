---
title: "pi"
description: "pi is the earendil-works coding agent CLI, driven headless with pi -p and pointed at the trace proxy through a custom lab provider."
weight: 80
---

pi is a coding agent CLI built by earendil-works, published on npm as `@earendil-works/pi-coding-agent` with the binary `pi`, and documented at pi.dev.
Its design keeps the core small and pushes everything else into user extensions, so out of the box the agent gets a four-tool set and a plain agent loop.
The lab drives it through a small adapter that runs `pi -p "<prompt>"` once per scenario, registers a custom provider whose base URL is the trace proxy, and grades whatever pi leaves in `/work`.
pi speaks OpenAI chat-completions natively, so the proxy records and forwards its requests without translating a dialect.

## What it is

pi is installed from npm into the tool image, so the lab does not depend on any pi checkout on the host.
The Dockerfile runs `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`, the install pi's own docs prescribe, on top of the shared base that already carries Node 22.
The captured system prompt states pi's own framing in its first line: an expert coding assistant operating inside pi, a coding agent harness, that reads files, executes commands, edits code, and writes new files.
In the lab it runs with the four built-in tools and nothing else, because the adapter loads no extension, skill, or package.
pi ships no built-in permission system, so in headless mode it runs with the process's own permissions and the container is the only sandbox.

## Command surface

The lab uses one entry point, pi's headless print mode.
`pi -p "<prompt>"` runs a single prompt non-interactively, prints the result, and exits, instead of opening the interactive TUI.

```bash
pi -p "Hi!"
```

The adapter selects the proxied model and trusts the working tree in the same call.

```bash
pi -p "$prompt" --model "lab/deepseek-v4-flash-free" -a
```

`--model lab/<model>` names the model as `provider/model`, where `lab` is the custom provider the adapter registers.
`-a` trusts the project-local tree for the run, so nothing stops on a trust check before pi starts working.

pi gives the model four tools by default: `read`, `bash`, `edit`, and `write`.
There is no separate `grep`, `find`, or `ls` tool in the default set: the prompt tells the model to run those through `bash` instead, so file listing and search go over the shell tool rather than dedicated tools.
Capabilities beyond the four defaults come from skills, prompt templates, extensions, or pi packages, none of which the lab loads.

The flags this page can verify from the adapter are `-p` for the one-shot prompt, `--model` for provider and model selection, and `-a` for trusting the tree.
Other subcommands and flags are not exercised by the lab, so they are not documented here.

## How the lab drives it

The pi-specific glue is a single adapter script that is the container entrypoint.
Everything upstream of it, the network, the trace capture, and the resource accounting, is the same for every tool.

The harness mounts three paths into the container.
`/work` is the scenario's working tree, writable, and the agent's cwd.
`/scenario` is the read-only scenario definition, holding `prompt.txt`.
`/trace` is where stdout, the rendered config, and the time report land.
It also passes `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

pi reads its model config from `~/.pi/agent/models.json`, so the adapter writes that file to register a custom provider named `lab`.

```json
{
  "providers": {
    "lab": {
      "baseUrl": "${LAB_BASE_URL}",
      "api": "openai-completions",
      "apiKey": "$OPENCODE_API_KEY",
      "models": [
        {
          "id": "${LAB_MODEL}",
          "name": "${LAB_MODEL}",
          "contextWindow": 128000,
          "maxTokens": 8192
        }
      ]
    }
  }
}
```

The provider's `baseUrl` is `LAB_BASE_URL`, which is the trace proxy, and its `api` is `openai-completions`, pi's OpenAI chat-completions client.
That is how pi's traffic reaches the proxy instead of the real upstream: it points its chat-completions client at the proxy's address, and the proxy tees and forwards each call.
The `apiKey` is left as the literal string `$OPENCODE_API_KEY`, not the expanded value, so pi interpolates the env var itself at run time.
The adapter copies this same file to `/trace/config.json`, and because the key stayed a literal, the real key never lands in the trace copy.
The proxy forwards upstream with the key it holds.

The models config declares `maxTokens: 8192`, so pi caps its own output budget at that value, and `contextWindow: 128000`.

The run itself is the one-shot print call, wrapped in GNU time for the resource numbers.

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  pi -p "$prompt" --model "lab/${LAB_MODEL}" -a \
  >/trace/stdout.log 2>/trace/stderr.log
```

The run happens in `/work`, the exact tree the checker inspects.
stdout goes to `/trace/stdout.log`, which is the reply the checker and this page read, and stderr goes to `/trace/stderr.log`.
Because pi has no sandbox and does not pause to approve a shell command or a file write, its built-in tools run freely, pi's equivalent of tomo's all-allow policy, with the container as the sandbox.

The Dockerfile installs pi with `ARG PI_VERSION=latest`, so the default build takes the current npm release rather than a pinned version, though the arg lets a build pin an exact version when needed.
The result is copied onto the shared `tomolab-base` image alongside the adapter, which is the entrypoint.

## Architecture

pi runs a plain agent loop.
The loop sends the conversation to the model, and when the model asks for a tool, pi runs it and feeds the result back, until the model answers without a tool call.
For the 00-hello run the loop made exactly one model call and zero tool calls, which the trace records as `model_calls: 1` and `tool_calls: 0`.

The tool set is small and fixed: `read`, `bash`, `edit`, and `write`.
`read` reads file contents, `bash` runs shell commands and covers listing and search, `edit` makes precise text replacements including multiple disjoint edits in one call, and `write` creates or overwrites whole files.
The 00-hello request carried exactly these four tool schemas, matching pi's documented default set.

pi speaks native chat-completions.
The provider's `api` is `openai-completions`, and the how-it-works proxy normalizes each request to the chat-completions shape before recording it.
Because pi already sends that shape, the proxy does not shim its dialect; it tees a copy into the trace and forwards it upstream.
The 00-hello trace confirms the path is `/zen/v1/chat/completions`, a plain chat call with no dialect translation tag.

pi has no built-in plan or todo tool.
Its project philosophy is explicit about this: no plan mode, write plans to files or build it with extensions.
There is a plan-mode example extension bundled under the package's `examples/extensions/plan-mode`, but it is an example you opt into, not a built-in `--plan` flag.
That extension plans in prose: it restricts the model to read-only tools, extracts numbered steps from a `Plan:` section, and gates execution behind an interactive prompt before any change is made.
The honest consequence for the lab is that headless pi can plan but cannot write through that extension: the extension's execution step needs an interactive approval that a `-p` run never reaches.
The lab does not load the extension anyway, so pi runs with the four default tools, and the trace shows `plan_calls: 0` and `planned: false` for the hello run.

## System prompt

The [prompts/pi](/prompts/pi/) page holds the verbatim text the proxy captured.
It was recovered with `lab prompts pi` across 17 captured runs, and it is the exact text that reached the model, not a copy from pi's source.

The proxy captured one distinct prompt, on wire `chat`, 2433 chars, seen across 85 requests.
It is a single static rendering: pi sent the same prompt every run, which is a simpler picture than tomo's four distinct prompts that tracked versions.
The prompt is compact for what it does, and the tool list it advertises is small, four tools against tomo's nine.

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

The guidelines push search and listing onto `bash` rather than a dedicated tool, and push reading onto `read` rather than shelling out.

```text
- Use bash for file operations like ls, rg, find
- Use read to examine files instead of cat or sed.
```

The bulk of the remaining text is edit-tool discipline: keep `oldText` small but unique, match against the original file, and merge nearby changes into one call.
The prompt closes with a volatile date line and the current working directory, `/work`, which is worth ignoring when comparing prompt text.

This section is recovered from the traces, not copied from pi's source.
What matches the public repo is the four-tool default set: the docs state pi gives the model read, write, edit, and bash by default, and the captured prompt advertises exactly those four.

## Hi! end to end

The 00-hello scenario hands pi the single prompt `Hi!` and checks that a greeting round trip completes.

The adapter reads `Hi!` from `/scenario/prompt.txt` and runs `pi -p "Hi!" --model lab/deepseek-v4-flash-free -a`.
pi builds the request around it: a system message with the agent prompt, whose first line is "You are an expert coding assistant operating inside pi, a coding agent harness.", then the user message `Hi!`.
The request also carries the four tool schemas (`read`, `bash`, `edit`, `write`), sent on the native chat wire.

At the proxy the request lands as a POST to `/zen/v1/chat/completions` with model `deepseek-v4-flash-free`.
The proxy forces greedy decoding, so the body shows `temperature=0`, `top_p=1`, `seed=7`, and `stream=True`.
It is a plain chat-completions call with two messages, roles system and user, and no dialect translation.
The full proxy tap for the run is two records, a `GET /zen/` health touch and the one `POST /zen/v1/chat/completions`.

That one request gets one upstream completion, and pi answers without calling a tool.
The trace records `requests: 2`, `orchestration.model_calls: 1`, `tool_calls: 0`, `plan_calls: 0`, and `planned: false`.
Tokens were 1552 prompt, 54 completion, 1606 total, and 1536 of the 1552 prompt tokens came back cached, so almost the entire system prompt was served from the upstream cache.
Latency was high on this run: 10859 ms to first byte and 12110 ms total, over one timed completion, so nearly all the wall time was spent waiting for the model to start replying.

The reply that reached the user, from `stdout.log` at 120 bytes, is:

```text
Hello! How can I help you today? Feel free to ask me to read files, run commands, edit code, or anything else you need.
```

The checker graded the run a pass.
It never reads the model's prose; it confirms the greeting round trip completed, records `check: "baseline greeting round trip completed"`, and marks `passed: true` on the first attempt.
The run also logged an install footprint of 160225 KB and a peak RSS of 123380 KB.
