---
title: "Tools"
linkTitle: "Tools"
description: "One research page per wired agent: what it is, its command surface, how the lab drives it, how it works inside, the system prompt it actually sent, and a Hi! run traced end to end."
weight: 20
featured: true
---

Eight agents run through the harness today.
Each one gets a page here that treats it as the object of study, not just a row in a table.

A tool page covers the same six things every time so two agents are easy to compare:

- What it is, and who builds it.
- Its command surface, down to the subcommands and flags the lab relies on.
- How the lab drives it, which is one adapter script and one Dockerfile.
- Its architecture, as far as the trace and the tool's own code show it.
- The system prompt it actually sent, recovered from the trace and checked against the tool's public source.
- A Hi! run traced end to end, from the request the tool built to the reply that reached the user.

The system prompt sections quote the real text the proxy captured.
The full verbatim prompts live under [prompts](/prompts/), one page per tool, regenerated with `lab prompts <tool>` so any drift shows up in the diff.

- [tomo](/tools/tomo/)
- [codex](/tools/codex/)
- [opencode](/tools/opencode/)
- [claude-code](/tools/claude-code/)
- [openclaw](/tools/openclaw/)
- [hermes](/tools/hermes/)
- [gemini-cli](/tools/gemini-cli/)
- [pi](/tools/pi/)
