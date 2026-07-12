---
title: "opencode"
description: "A review of the system prompts opencode sent to the model, recovered from the trace proxy: a full agent prompt and a strict title generator, stored raw and linked."
weight: 30
---

Recovered with `lab prompts opencode` from the trace proxy (newest capture 20260711T110419Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompts are stored raw under [`prompts/opencode/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/opencode); this page reviews what each one does and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 2,010 tokens
- tools (10): `bash`, `edit`, `glob`, `grep`, `read`, `skill`, `task`, `todowrite`, `webfetch`, `write`
- raw: [prompts/opencode/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/opencode/1-agent.md)

opencode's prompt reads like a working style guide: keep replies short, prefer the search tools, follow the project's own conventions, and never invent a URL.
It exposes a broad, general tool set, ten tools covering file work, search, a task tracker, and web fetch, and it spends its budget teaching the model when to reach for each rather than restating their signatures.
The result is a mid-weight prompt that sits well below the heaviest agents while still driving a full toolbox.

## Title generator

- 503 tokens
- no tools
- raw: [prompts/opencode/2-title-generator.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/opencode/2-title-generator.md)

Beside the agent, opencode runs a small side model that names each thread.
The prompt is emphatic that the model output only a title and nothing else, which is a common shape across these tools: a cheap, tool-free call whose whole job is to return one clean line.
