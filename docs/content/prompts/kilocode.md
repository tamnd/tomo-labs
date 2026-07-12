---
title: "kilocode"
description: "A review of the system prompt kilocode sent to the model, recovered from the trace proxy: an opencode-lineage agent prompt with a terse house style, stored raw and linked."
weight: 90
---

Recovered with `lab prompts kilocode` from the trace proxy (newest capture 20260711T110403Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompt is stored raw under [`prompts/kilocode/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/kilocode); this page reviews it and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 2,381 tokens
- tools (13): `bash`, `edit`, `read`, `write`, `glob`, `grep`, `task`, `todowrite`, `skill`, `webfetch`, `background_process`, `kilo_local_recall`, `suggest`
- raw: [prompts/kilocode/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/kilocode/1-agent.md)

Kilo Code is an opencode fork, and the family resemblance is all over the prompt.
It insists on a terse, technical voice, forbids conversational filler, and pushes hard for answers under four lines, then backs that with a full thirteen-tool workbench including a local-recall tool and a background-process runner.
The prompt spends its budget on manner and on when-to-use rules, and it names the model it is running under in one line near the end, the single span that drifts between runs.
It sits in the middle of the token table: heavier than its opencode ancestor, well short of the largest agents.
