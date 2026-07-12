---
title: "gemini-cli"
description: "A review of the system prompt gemini-cli sent to the model, recovered from the trace proxy after its Gemini-wire traffic was normalized to chat-completions, stored raw and linked."
weight: 70
---

Recovered with `lab prompts gemini-cli` from the trace proxy (newest capture 20260711T110355Z).
gemini-cli speaks Google's Gemini wire, which the proxy translates to a single chat-completions call, so the prompt below is what reached the model after normalization, not a copy from the tool's source.
The prompt is stored raw under [`prompts/gemini-cli/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/gemini-cli); this page reviews it and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 5,872 tokens
- tools (15): `read_file`, `write_file`, `replace`, `glob`, `grep_search`, `run_shell_command`, `google_web_search`, `web_fetch`, `invoke_agent`, `activate_skill`, `enter_plan_mode`, and more
- raw: [prompts/gemini-cli/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/gemini-cli/1-agent.md)

gemini-cli writes a long, prescriptive prompt built around explicit workflows.
It walks the model through mandated sequences, understand then plan then implement then verify, and pins down a strict tone for a command-line setting, so much of the weight is procedure rather than tool description.
That it lands as the second-largest prompt in the field is a deliberate style choice: heavy up-front scaffolding in exchange for predictable, step-by-step behaviour.
Because the tool normalizes from the Gemini wire, this capture is also a useful check that the translation preserves the whole prompt intact.
