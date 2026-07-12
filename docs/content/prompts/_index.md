---
title: "Prompts"
linkTitle: "Prompts"
description: "The system prompt every wired agent actually sent, recovered verbatim from the trace proxy with lab prompts, one page per tool and versioned so drift shows up in the diff."
weight: 30
featured: true
---

Every agent under test routes its model traffic through the trace proxy.
The proxy records each completion after normalizing it to the chat-completions shape, so a tool's system prompt lands in the request tap no matter which wire the tool speaks.
That makes the trace, not the tool's source, the ground truth for what actually reached the model.

`lab prompts <tool>` reads that tap across every captured run, unions the distinct system prompts, and ranks the agent's working prompt (the one carrying a tool schema) first.
Most tools splice volatile context into the prompt each run: the date, the working directory, a session id, a build number.
Those renderings are grouped as one prompt, with a count of how many collapsed together, so the page shows the base prompt rather than a hundred near-copies.

The pages below are generated straight from that command, one per tool, and checked into the repo.
Regenerate a page with `lab prompts <tool> --json` and any change a tool made to its prompt between versions shows up in the diff.

- [tomo](/prompts/tomo/)
- [codex](/prompts/codex/)
- [opencode](/prompts/opencode/)
- [claude-code](/prompts/claude-code/)
- [openclaw](/prompts/openclaw/)
- [hermes](/prompts/hermes/)
- [gemini-cli](/prompts/gemini-cli/)
- [pi](/prompts/pi/)
- [kilocode](/prompts/kilocode/)
- [aider](/prompts/aider/)
- [copilot](/prompts/copilot/)
