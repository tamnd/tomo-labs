---
title: "tomo"
description: "A review of the system prompts tomo sent to the model, recovered from the trace proxy: a tiny warm agent prompt and a separate JSON planner, stored raw and linked."
weight: 10
---

Recovered with `lab prompts tomo` from the trace proxy (newest capture 20260711T110434Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompts are stored raw under [`prompts/tomo/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/tomo); this page reviews what each one does and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 326 tokens
- tools (9): `fetch`, `memory_read`, `memory_write`, `plan`, `read_file`, `shell`, `skill_read`, `time`, `write_file`
- raw: [prompts/tomo/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/tomo/1-agent.md)

This is the smallest working prompt in the field, and that is the whole point tomo is making.
It sets a persona in two lines, a personal agent living on the user's own machine, talking over a chat channel, direct and brief, and then spends the rest of its budget on behaviour rather than on restating what the tools do.
The nine tools are declared in the tool-calling channel, not described again in prose, so the prompt never pays twice for the same schema.
On the `Hi!` baseline that keeps tomo's fixed cost near the floor while agents that inline their whole tool manual pay several times as much before doing any work.

## Planner

- 223 tokens
- no tools
- raw: [prompts/tomo/2-planner.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/tomo/2-planner.md)

tomo runs a separate, tool-free model when it decides a job is worth planning.
The prompt asks for only a JSON array of steps, each a goal with a list of earlier steps it depends on, and forbids prose so the output parses cleanly.
Small scenarios rarely trigger it, so it barely shows up here; the planner is there for the jobs that need one.
