---
title: "claude-code"
description: "A review of the system prompts claude-code sent to the model, recovered from the trace proxy: an SDK agent prompt and a separate agent-types registry, stored raw and linked."
weight: 40
---

Recovered with `lab prompts claude-code` from the trace proxy (newest capture 20260711T110347Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompts are stored raw under [`prompts/claude-code/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/claude-code); this page reviews what each one does and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

claude-code sends its system context as more than one block, so the recovery splits into two files: the identity-and-behaviour prompt, and the registry of subagent types it can spawn.
Both ride on essentially every call, so a fair read of its floor adds them together.

## Agent prompt

- 1,285 tokens
- tools (24): `Agent`, `Bash`, `Edit`, `Read`, `Skill`, `Task*`, `Cron*`, `Web*`, `Workflow`, `Write`, and more
- raw: [prompts/claude-code/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/claude-code/1-agent.md)

This block opens with a billing header the client attaches, then declares the model a Claude agent built on the Agent SDK and lays out how it should work: read before editing, prefer the dedicated tools, confirm before hard-to-reverse actions.
It leans hard on native tool-calling, twenty-four tools in the schema channel, so the prose stays about behaviour rather than tool signatures.
The `cc_version` in the header is the one span that drifts between runs, which is exactly the kind of change a diff over this file is meant to surface.

## Agent types registry

- 1,677 tokens
- tools (24): same schema as the agent prompt
- raw: [prompts/claude-code/2-agent-types.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/claude-code/2-agent-types.md)

The second block is a catalogue of the subagents claude-code can delegate to, each with a one-line brief on when to use it and which tools it may touch.
It is the larger of the two prompts, so the delegation menu, not the base persona, is the heavier part of claude-code's fixed cost here.
Keeping it as its own block lets the agent prompt stay stable while the roster of subagents changes underneath it.
