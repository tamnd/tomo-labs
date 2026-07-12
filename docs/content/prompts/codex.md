---
title: "codex"
description: "A review of the system prompts the Codex CLI sent to the model, recovered from the trace proxy: a large agent prompt and a separate sandbox-permissions preamble, stored raw and linked."
weight: 20
---

Recovered with `lab prompts codex` from the trace proxy (newest capture 20260711T110347Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompts are stored raw under [`prompts/codex/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/codex); this page reviews what each one does and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 4,365 tokens
- tools (8): `create_goal`, `exec_command`, `get_goal`, `request_user_input`, `update_goal`, `update_plan`, `view_image`, `write_stdin`
- raw: [prompts/codex/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/codex/1-agent.md)

Codex writes a long, careful prompt.
It spells out its capabilities, a house style for edits and commit hygiene, and a detailed policy for when to ask before acting, and it leans on a small exec-centered tool set rather than a broad file API.
Most of the token weight is the behavioural contract, not tool descriptions, which is why the prompt is large even though it exposes only eight tools.
That fixed weight is the price of its guardrails, and it rides on every call the agent makes.

## Permissions preamble

- 688 tokens
- tools (8): `create_goal`, `exec_command`, `get_goal`, `request_user_input`, `update_goal`, `update_plan`, `view_image`, `write_stdin`
- raw: [prompts/codex/2-permissions.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/codex/2-permissions.md)

Codex sends its sandbox and approval state as a separate system block, wrapped in a `<permissions instructions>` tag.
In the lab the sandbox is `danger-full-access` with approvals set to never, so this block tells the model it may run any command with network access and should not stop to ask.
Splitting it out keeps the sandbox facts in one auditable place and lets the same agent prompt ride on top of any permission regime.
