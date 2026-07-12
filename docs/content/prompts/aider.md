---
title: "aider"
description: "A review of the system prompts aider sent to the model, recovered from the trace proxy: a whole-file coder prompt and a chat summarizer, stored raw and linked."
weight: 100
---

Recovered with `lab prompts aider` from the trace proxy (newest capture 20260711T110347Z).
aider routes its model calls through litellm, which the proxy sees as ordinary chat-completions, so the prompts below are the exact text that reached the model, not a copy from the tool's source.
The prompts are stored raw under [`prompts/aider/`](https://github.com/tamnd/tomo-labs/tree/main/prompts/aider); this page reviews what each one does and links to the file.
Sizes are in tokens, counted with the `o200k_base` encoding.

## Coder prompt

- 260 tokens
- no tools
- raw: [prompts/aider/1-coder.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/aider/1-coder.md)

aider takes a different path from every other agent here: it exposes no tools at all.
Instead of a function schema it opens with "Act as an expert software developer" and teaches the model an edit format in prose, asking for whole-file or search-and-replace blocks it can parse out of the reply and apply itself.
That is why the prompt is one of the smallest in the set even though aider is a capable coding tool: the machinery lives in aider's own parser, not in the prompt.
One artifact from the `Hi!` run is worth flagging.
aider sends no output cap, so on the ceiling model the completion ran to the proxy's `max_tokens` floor and stopped at exactly 32,000 tokens, a runaway that reflects the floor rather than anything aider asked for.

## Summarizer

- 175 tokens
- no tools
- raw: [prompts/aider/2-summarizer.md](https://github.com/tamnd/tomo-labs/blob/main/prompts/aider/2-summarizer.md)

The second prompt is aider's chat summarizer, the side call it makes to compress a long conversation back into a short running history.
It asks for a terse, second-person summary written as if the user said it, so the digest can be fed back in as context.
Like the coder prompt it is tool-free and small, and it never touches the scored task.
