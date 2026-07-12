---
title: "copilot"
description: "A review of the system prompt copilot sent to the model, recovered from the trace proxy: a seventeen-tool BYOK agent prompt, stored raw and linked."
weight: 110
---

Recovered with `lab prompts copilot` from the trace proxy (newest capture 20260711T110350Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompt is stored raw under [`prompts/copilot/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/copilot); this page reviews it and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 5,091 tokens
- tools (17): `bash`, `view`, `read`, `create`, `edit`, `glob`, `grep`, `task`, `web_fetch`, `skill`, `sql`, `session_store_sql`, `list_bash`, `read_bash`, `stop_bash`, `list_agents`, `read_agent`
- raw: [prompts/copilot/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/copilot/1-agent.md)

Running Copilot CLI against the shared proxy makes it a bring-your-own-key agent, and its prompt is built for a full coding session.
Seventeen tools cover the usual file and shell work plus a few that stand out: a SQL pair for a session store, a background-bash trio for long-running commands, and an agent-listing pair for spawning and reading subagents.
The prose sets a careful, permission-aware tone and spends real space on how to sequence those tools, which is what carries it into the upper half of the token table.
It is the heaviest of the three agents added in this round, and a good contrast with aider's tool-free approach at the other end of the field.
