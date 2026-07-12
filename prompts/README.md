# System prompt captures

This directory holds the verbatim system prompt every wired agent actually sent to the model, recovered from the trace proxy.

Every agent under test routes its model traffic through the proxy, which records each completion after normalizing it to the chat-completions shape.
So a tool's system prompt lands in the request tap no matter which wire the tool speaks: native chat, the OpenAI Responses API, the Anthropic Messages API, or Google's Gemini wire.
That makes the trace, not a copy from the tool's source, the ground truth for what reached the model.

`lab prompts <tool> --json` reads that tap across every captured run, unions the distinct prompts, and ranks the working agent prompt first.
The files here are the canonical rendering of each distinct prompt, one file per prompt, stored raw.
Raw beats embedded: a prompt often carries its own fenced code blocks, so pasting it into a rendered page breaks the page, and a diff over a raw file is the cleanest way to see a prompt change between tool versions.
The human review lives under [docs/content/prompts](../docs/content/prompts/), one page per tool, and links back to these files.

Prompt sizes below are counted in tokens with the `o200k_base` encoding, since tokens track what the model pays to read a prompt more closely than characters do.

## Prompts by tool

Ordered by the size of the working agent prompt, the fixed cost a tool pays before it does any work.
tomo and pi sit at the floor; openclaw and gemini-cli carry the largest prompts.

| tool | prompts | agent prompt (tokens) | tools exposed |
| --- | --- | --- | --- |
| [aider](./aider/) | 2 | 260 | 0 |
| [tomo](./tomo/) | 2 | 326 | 9 |
| [pi](./pi/) | 1 | 540 | 4 |
| [claude-code](./claude-code/) | 2 | 1,285 | 24 |
| [hermes](./hermes/) | 2 | 1,566 | 19 |
| [opencode](./opencode/) | 2 | 2,010 | 10 |
| [kilocode](./kilocode/) | 1 | 2,381 | 13 |
| [codex](./codex/) | 2 | 4,365 | 8 |
| [copilot](./copilot/) | 1 | 5,091 | 17 |
| [gemini-cli](./gemini-cli/) | 1 | 5,872 | 15 |
| [openclaw](./openclaw/) | 1 | 7,413 | 24 |

aider is the outlier in the tools column: it edits through a prose whole-file format rather than native tool-calling, so its prompts carry no tool schema at all.

## Every captured prompt

Some tools send more than one system prompt.
Beyond the agent prompt, a tool may run a small side model to name a thread, plan a job, or state its sandbox permissions, and each of those carries its own prompt.

| tool | file | role | tokens | tools |
| --- | --- | --- | --- | --- |
| aider | [1-coder.md](./aider/1-coder.md) | coder | 260 | 0 |
| aider | [2-summarizer.md](./aider/2-summarizer.md) | summarizer | 175 | 0 |
| tomo | [1-agent.md](./tomo/1-agent.md) | agent | 326 | 9 |
| tomo | [2-planner.md](./tomo/2-planner.md) | planner | 223 | 0 |
| pi | [1-agent.md](./pi/1-agent.md) | agent | 540 | 4 |
| claude-code | [1-agent.md](./claude-code/1-agent.md) | agent | 1,285 | 24 |
| claude-code | [2-agent-types.md](./claude-code/2-agent-types.md) | agent types registry | 1,677 | 24 |
| hermes | [1-agent.md](./hermes/1-agent.md) | agent | 1,566 | 19 |
| hermes | [2-title-generator.md](./hermes/2-title-generator.md) | title generator | 66 | 0 |
| opencode | [1-agent.md](./opencode/1-agent.md) | agent | 2,010 | 10 |
| opencode | [2-title-generator.md](./opencode/2-title-generator.md) | title generator | 503 | 0 |
| kilocode | [1-agent.md](./kilocode/1-agent.md) | agent | 2,381 | 13 |
| codex | [1-agent.md](./codex/1-agent.md) | agent | 4,365 | 8 |
| codex | [2-permissions.md](./codex/2-permissions.md) | permissions | 688 | 8 |
| copilot | [1-agent.md](./copilot/1-agent.md) | agent | 5,091 | 17 |
| gemini-cli | [1-agent.md](./gemini-cli/1-agent.md) | agent | 5,872 | 15 |
| openclaw | [1-agent.md](./openclaw/1-agent.md) | agent | 7,413 | 24 |

## How these were captured

Run a sweep, then read the prompts back out of the traces:

```sh
lab run "" 00-hello          # or a full sweep
lab prompts codex --json     # structured, every distinct prompt
lab prompts codex --brief    # headers only
```

Most tools splice a little volatile context into the prompt each run: the date, the working directory, a session id, the name of the model.
Those renderings collapse to one prompt, and the file here is the rendering sent on the most runs.
This batch was captured on the `nemotron-3-ultra-free` model, because the default `deepseek-v4-flash-free` was rate-limited at capture time.
The model shows up only in the one line of each prompt that names it, so the rest of every file is what any run would have sent.
