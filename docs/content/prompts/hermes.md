---
title: "hermes"
description: "A review of the system prompts hermes sent to the model, recovered from the trace proxy: a broad agent prompt and a tiny title generator, stored raw and linked."
weight: 60
---

Recovered with `lab prompts hermes` from the trace proxy (newest capture 20260711T110400Z).
Every tool routes through the proxy, which records each completion after normalizing it to the chat-completions shape, so this is the exact text that reached the model, not a copy from the tool's source.
The prompts are stored raw under [`prompts/hermes/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/hermes); this page reviews what each one does and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Agent prompt

- 1,566 tokens
- tools (19): `terminal`, `patch`, `read_file`, `write_file`, `search_files`, `execute_code`, `memory`, `delegate_task`, `todo`, `vision_analyze`, `image_generate`, `text_to_speech`, and more
- raw: [prompts/hermes/1-agent.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/hermes/1-agent.md)

hermes presents itself as a general assistant, and its toolbox is the widest-ranging in spirit: alongside the usual shell, file, and search tools it carries vision, image generation, and text-to-speech.
The prompt stays mid-weight by leaning on the tool schema for signatures and using its prose to set the assistant's manner and its rules for delegating work.
Worth noting from the traces: on a bare `Hi!` hermes fires more than twenty HTTP requests, but only one is a real completion.
The rest are capability probing, hitting Ollama- and OpenAI-style discovery endpoints like `/api/tags`, `/props`, and `/version`, most of which the proxy answers with a 404 before the single scored call goes through.

## Title generator

- 66 tokens
- no tools
- raw: [prompts/hermes/2-title-generator.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/hermes/2-title-generator.md)

The second prompt is the smallest capture in the whole set: a two-sentence instruction to name a conversation in three to seven words and return only the title.
It is the tool-free side call hermes makes to label a thread, and it costs almost nothing next to the agent prompt.
