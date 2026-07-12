---
title: "openclaw"
description: "A review of the system prompt openclaw sent to the model, recovered from the trace proxy: the largest agent prompt in the field, stored raw and linked."
weight: 50
---

Recovered with `lab prompts openclaw` from the trace proxy (newest capture 20260711T110411Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompt is stored raw under [`prompts/openclaw/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/openclaw); this page reviews it and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 7,413 tokens
- tools (24): `apply_patch`, `exec`, `edit`, `read`, `write`, `web_fetch`, `web_search`, `memory_get`, `memory_search`, `sessions_spawn`, `subagents`, `cron`, and more
- raw: [prompts/openclaw/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/openclaw/1-agent.md)

openclaw carries the largest prompt of any wired agent, and reading it explains why.
It is a personal-assistant brief that tries to cover the whole surface in prose: a broad toolbox of twenty-four tools spanning shell, patching, memory, web, cron, and multi-session orchestration, each introduced and hedged with when-to-use guidance.
The design bets that a thorough single briefing makes the model steadier, and the cost of that bet is a fixed prompt many times the size of tomo's, paid on every call.
On the `Hi!` baseline that fixed weight is what puts openclaw near the top of the token table before any work happens.
