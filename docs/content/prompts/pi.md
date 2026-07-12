---
title: "pi"
description: "A review of the system prompt pi sent to the model, recovered from the trace proxy: a compact four-tool agent prompt, stored raw and linked."
weight: 80
---

Recovered with `lab prompts pi` from the trace proxy (newest capture 20260711T110423Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompt is stored raw under [`prompts/pi/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/pi); this page reviews it and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 540 tokens
- tools (4): `bash`, `edit`, `read`, `write`
- raw: [prompts/pi/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/pi/1-agent.md)

pi keeps its prompt small by keeping its toolbox small: four tools, read, write, edit, and bash, and a short set of rules for using them well.
Most of the prose is precise guidance on the edit tool, how to match text exactly and how to batch disjoint edits into one call, which is where a lean agent earns its keep.
It is the second-smallest working prompt after tomo, and the two share the same lesson: a tight tool set is the cheapest way to a small fixed cost.
