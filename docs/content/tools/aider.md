---
title: "aider"
description: "aider, the whole-file-editing pair programmer, driven headless through aider --message against the lab's trace proxy on the same fixed model as every other agent."
weight: 100
---

aider is a terminal coding assistant written in Python by Paul Gauthier.
It normally runs as an interactive chat in your terminal, but it also has a headless one-shot mode, `aider --message`, that takes a single instruction, does the work, and exits.
That mode is how the lab drives it: one adapter script, one Dockerfile, and aider's model pointed at the trace proxy.
aider is the one tool in the suite that does not use native tool-calling.
It edits code through a whole-file format written in prose, so its requests carry no tool schema, and it speaks OpenAI chat-completions through litellm, which the proxy records on its plain chat path.
This page is grounded entirely in the wired image, the adapter, and the newest `00-hello` trace; it claims only what those show.

## Overview

aider is a pair programmer you run in your terminal.
You give it a change to make and it reads the relevant files, reasons about the edit, and writes back whole updated files, applying the result to the working tree itself.
It routes to any OpenAI-compatible endpoint through litellm, so it is not tied to one vendor: a model named `openai/<id>` selects litellm's OpenAI handler.
In the lab it runs the same fixed model every other tool runs, so the only variable under study is the agent itself.

The Dockerfile installs aider from PyPI, not from a source checkout, so the image never depends on a clone of the repo:

```dockerfile
FROM tomolab-base
ARG AIDER_VERSION=0.86.2
RUN pip3 install --no-cache-dir --break-system-packages aider-chat==${AIDER_VERSION}
COPY adapter.sh /usr/local/bin/adapter
```

The PyPI package is `aider-chat`, the installed binary is `aider`, and the captured version is `0.86.2`.
The base image marks its Debian Python as externally managed, so the install passes `--break-system-packages` to write into the image's own site-packages, which is what a throwaway container wants.
aider carries the heaviest install of any wired tool, 622 MB, most of it litellm and its provider dependencies, but it is light on memory at run time: the `00-hello` run peaks at 242 MB resident, on the low end of the suite.

### At a glance

| Property | Value |
| --- | --- |
| Runtime | Python 3, from the shared `tomolab-base` image |
| Install source | PyPI package `aider-chat`, binary `aider` |
| Version captured | `0.86.2` (Dockerfile `AIDER_VERSION`) |
| Wire dialect | OpenAI chat-completions through litellm, no tool schema |
| Edit strategy | whole-file listings in prose, not native tool-calling |
| How the lab invokes it | `aider --model openai/$LAB_MODEL --no-git --yes-always --message "$prompt"` |
| Provider config | `OPENAI_API_BASE` and `OPENAI_API_KEY` env vars read by litellm |
| Where it writes | `/work` for edits, `/trace` for config, history, stdout, and the time report |
| Peak memory (00-hello) | 242 MB, on the low end of the suite |
| Install footprint | 622 MB, the heaviest of any wired tool |

### How aider differs from the rest of the suite

Every other wired agent gives the model a set of function tools and dispatches whatever it calls.
aider does not.
Its coder prompt tells the model to reply with the entire content of each file it wants to change, wrapped in a fenced block under the filename, and aider parses that text and applies it.
So there is no tool schema on the wire, no `tool_choice`, and the count of tool calls in the trace is structurally zero.
That design has two visible consequences in the `00-hello` run.
First, aider is edit-first: it treats even a bare `Hi!` as a change request and writes a file (`solution.py`, 1662 bytes) rather than just greeting back.
Second, it makes a second kind of model call, a summarizer, to compress the running chat history so a long session stays inside the context window; both prompts are on the [prompts page](/prompts/aider/).

## Say Hi!

The `00-hello` scenario is the smallest run in the suite.
The prompt is `Hi!` and the checker asks only that a greeting round trip completed.
Here is the run end to end, from the newest trace (`20260711T100526Z`).

The adapter reads the prompt from the read-only scenario mount, then exports the two env vars litellm reads, pointing the base URL at the trace proxy, not the real upstream:

```bash
export OPENAI_API_BASE="${LAB_BASE_URL}"
export OPENAI_API_KEY="${OPENCODE_API_KEY}"
```

It records those to `/trace/config.json` so the run captures exactly what aider was told, then runs aider once, non-interactively:

```bash
cd /work
/usr/bin/time -v -o /trace/time.txt \
  aider \
    --model "openai/${LAB_MODEL}" \
    --no-git --yes-always \
    --no-show-model-warnings --no-check-update --no-analytics \
    --chat-history-file /trace/aider.chat.history.md \
    --input-history-file /trace/aider.input.history \
    --message "$prompt" \
  >/trace/stdout.log 2>/trace/stderr.log
```

aider builds its turn through litellm: the coder system prompt, the user message, and the supplied files, all on the chat wire with no tools.
Because aider sends no `max_tokens` of its own, the proxy's floor fills one in (`max_tokens` 32000), which is what stops a runaway free model from streaming without bound; the sampling knobs come from the lab's forced decoding (`temperature` 0, `top_p` 1, `seed` 7, `stream` true).

### Why a bare hello made 5 requests and 4 model calls

At the proxy the run lands as five records: one health probe and four completion POSTs, none of them carrying tools.

| seq | Method and path | Role | Messages | Tools |
| --- | --- | --- | --- | --- |
| 1 | `GET /zen/` | health probe | none | none |
| 2 | `POST /zen/v1/chat/completions` | coder | 8 | 0 |
| 3 | `POST /zen/v1/chat/completions` | coder | 10 | 0 |
| 4 | `POST /zen/v1/chat/completions` | summarizer | 2 | 0 |
| 5 | `POST /zen/v1/chat/completions` | summarizer | 2 | 0 |

Record 1 is litellm checking the endpoint is reachable.
Records 2 and 3 are the coder: aider sends the "Act as an expert software developer" prompt, reads back a whole-file listing, and applies it, which is how `solution.py` appears in `/work`.
Records 4 and 5 are the summarizer, a separate model call with the "Briefly summarize this partial conversation" prompt that aider uses to keep chat history bounded.
The bare greeting is trivial, so the four calls are aider's own bookkeeping rather than four turns of real work.

### The numbers

| Metric | Value |
| --- | --- |
| Passed | true, on attempt 1 of 3 allowed |
| Proxy records | 5 (1 `GET`, 4 `POST`) |
| Model calls | 4 (2 coder, 2 summarizer) |
| Tool calls | 0, by design |
| Plan calls | 0 |
| Subagents | 0 |
| Prompt tokens | 1734 |
| Completion tokens | 3056 |
| Total tokens | 4790 |
| TTFB (avg) | 2192 ms |
| Total (avg per call) | 14156 ms |
| Peak RSS | 242 MB, on the low end of the suite |
| Install footprint | 622 MB, the heaviest of any wired tool |
| Wall clock | 0:59.24 |

aider is the slowest tool on this trivial scenario, 59 seconds, because it spends its time making four model calls and applying an edit rather than answering in one turn.
It still passes on the first attempt: the checker grades the greeting round trip, and aider completes it.

## Architecture

### The container

The image is built from `tomolab-base`, which already carries the Python 3 and pip aider needs.
The Dockerfile does one install step and installs the adapter as the entrypoint:

```dockerfile
FROM tomolab-base
ARG AIDER_VERSION=0.86.2
RUN pip3 install --no-cache-dir --break-system-packages aider-chat==${AIDER_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
```

There is no aider source in the image; `aider-chat` is a self-contained PyPI package and its litellm dependency handles the provider wire.

### Mounts

The harness mounts three directories into the container.

| Mount | Access | Purpose |
| --- | --- | --- |
| `/work` | read-write | The scenario's working tree and the agent's cwd; the tree the checker grades |
| `/scenario` | read-only | The scenario definition, holds `prompt.txt` |
| `/trace` | read-write | Where the config, chat and input history, stdout, stderr, exit code, and time report land |

aider's own bookkeeping files, the chat history and input history, are pointed at `/trace` rather than `/work` so they do not pollute the tree the checker inspects.

### Harness environment

The harness passes `LAB_BASE_URL` (exported as `OPENAI_API_BASE`), `LAB_MODEL` (qualified as `openai/$LAB_MODEL` and passed to `--model`), `OPENCODE_API_KEY` (exported as `OPENAI_API_KEY`), and `LAB_MAX_TURNS` (present for parity; aider's `--message` mode runs a single instruction to completion rather than a turn loop the lab caps).

### The adapter step by step

The adapter is the container entrypoint and the only aider-specific glue in the lab.
It reads the prompt, exports the two litellm env vars pointed at the proxy, records them to `/trace/config.json`, then runs aider once under `/usr/bin/time -v` so the harness can read peak resident set back from the GNU time report.

The flags matter:

- `--message` is the headless one-shot mode: a single instruction, then exit.
- `--yes-always` answers every confirmation with yes, aider's all-allow policy, so it creates and edits files without stopping to ask; the container is the sandbox.
- `--no-git` keeps aider out of version control, since `/work` is a plain tree rather than a repo, and stops it committing its own edits.
- `--no-show-model-warnings` skips the unknown-model prompt, since the free upstream model is not in litellm's metadata table.
- `--no-check-update` and `--no-analytics` keep it from phoning home.
- `--chat-history-file` and `--input-history-file` send aider's bookkeeping to `/trace`.

The adapter records aider's real exit status to `/trace/exit_code` and then always `exit 0`s, so a nonzero agent exit does not crash the container before the trace is written.

### How aider reaches the proxy

aider never knows it is being traced.
litellm reads `OPENAI_API_BASE` and `OPENAI_API_KEY`, and for a model named `openai/<id>` it sends ordinary chat-completions requests to that base URL, which is the proxy.
The proxy normalizes each completion to the chat-completions shape, tees the request body, streamed response, and token usage into `/trace`, and forwards to the real upstream with the real key.
Because aider drives edits through prose rather than tools, the requests carry no tool schema, so the proxy records them on its untagged chat path exactly as it records any other chat completion.

## System Prompts

aider's prompts are its own, recovered verbatim by `lab prompts aider`, not something the lab injects.
The lab injects nothing into the prompt; it only redirects litellm's base URL so the proxy can record what aider already sends.
The proxy captured two distinct prompts, and neither carries a tool schema, which is why `lab prompts` labels both as side prompts (its notion of an agent prompt is the one that ships tools, and aider ships none).
Full verbatim text, byte counts, and request counts are at [/prompts/aider/](/prompts/aider/).

| Prompt | Role | Size | Requests | Tools | Wire |
| --- | --- | --- | --- | --- | --- |
| 1, coder | the whole-file edit prompt aider works through | 1189 chars | 5 | 0 | chat |
| 2, summarizer | compresses chat history so a long session stays in context | 857 chars | 4 | 0 | chat |

Prompt 1 is the load-bearing one.
It tells the model to return the entire content of each file it wants to change, in a strict filename-then-fence format, and never to elide with "...":

```text
To suggest changes to a file you MUST return the entire content of the updated file.
You MUST use this *file listing* format:
```

Prompt 2 is the summarizer, written in the user's first person so aider can replay the condensed history back as context on the next turn:

```text
Phrase the summary with the USER in first person, telling the ASSISTANT about the conversation.
Write *as* the user.
The user should refer to the assistant as *you*.
Start the summary with "I asked you...".
```
