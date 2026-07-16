---
title: "The oi dialect zoo: a cheap model dresses run this code in four fences"
linkTitle: "oi dialect zoo, four fences"
description: "A cheap model told to write a Markdown code fence keeps its shell or python command but wraps it in a different costume from turn to turn, and tomo's oi engine only read the Markdown one. On gitingest-94 with deepseek-v4-flash-free, a slice of tomo-oi runs died on the very first request with a valid action dropped as if it were empty prose. Reading every one-request failure's raw output named four shapes: an XML tool-call tag, an HTML pre/code block, a language-named tag, and an invented tool with no code inside. Three carry a real command and now get salvaged when no Markdown fence is present, and the fourth is a lost tool call that the finish guard now nudges once. With both in, gitingest-94 went from roughly two passes in eight to seven in eight and the one-request hard drops disappeared."
date: 2026-07-16T11:11:00+07:00
---

Running the real upstream Open Interpreter against tomo's oi engine on the same task, model, and harness turned up a concrete reliability gap.
Closing it took one general fix, not the finish-guard patch it first looked like.

Upstream Open Interpreter 0.4.2 passes gitingest-94 on deepseek-v4-flash-free in four model calls.
tomo-oi passes the same task on the same model too, but a slice of its runs died on the first request with the model having emitted a perfectly good action that the harness threw away.
The cause was not a hallucination and not the finish guard.
It was dialect drift.
This cheap model, told to write a Markdown code fence, keeps its shell or python command but dresses the fence in a different costume from turn to turn, and tomo only read the Markdown one.

## Setup

Same lab, same container, same trace proxy, same graded checker, same model, one paid attempt each, pass@1.
The model is deepseek-v4-flash-free through opencode zen.
Upstream Open Interpreter runs as its own tool image, open-interpreter 0.4.2 from PyPI under the image's system Python with litellm pinned to skip pip backtracking, and the agent's shell blocks still resolve the task venv so grading is unchanged.
tomo-oi runs from the current local tomo binary copied into a tomolab-base image, so the engine under test carries every fix in this session.

    # upstream Open Interpreter 0.4.2 as its own tool image
    lab probe gitingest-94 \
      --engine openinterpreter --model deepseek-v4-flash-free --grade \
      --out /tmp/oi-upstream

    # tomo's oi engine from the local binary
    lab probe gitingest-94 \
      --engine oi --model deepseek-v4-flash-free --grade \
      --out /tmp/oi-tomo

Upstream OI is far leaner because its kernel is stateful.
It reads a file once and keeps it in memory across blocks.
tomo-oi executes each block in a fresh process and only the working tree persists, a deliberate box-safety tradeoff, so it re-reads and costs more tokens.
That gap is not what this experiment changes.
What it changes is the reliability of tomo-oi getting an action to run at all on a model whose fence syntax wanders.

## The result

| Engine | gitingest-94 | Calls | Tokens | Note |
|---|---|---|---|---|
| upstream OI 0.4.2 | PASS | 4 | 7,746 | stateful kernel |
| tomo-oi before fix | FAIL (one draw) | 1 | 1,031 | action dropped |
| tomo-oi after fix | PASS 7 of 8 runs | 3-11 | 14k-69k | drops closed |

The samples are small and this model is stochastic, so the run counts are directional, not precise.
What is solid is that the one-request hard drops that lost a valid action are reproduced before the fix and gone after it, the salvage firing is visible in the traces as a language-named block followed by an execute result, and every shape is locked by a unit test.

## The four costumes

Reading every one-request failure's raw output named four shapes on this one task.

The first is an XML tool-call tag that names the language and carries the command as a code payload.

    <tool_call><tool_name>execute</tool_name><tool_args>
      <language>sh</language><code>...</code></tool_args></tool_call>

The second is an HTML pre/code block with the language on the first line, with the narration folded into a details section around it.

    <pre><code>shell
    ...
    </code></pre>

The third is a tag named after the language itself, for example a shell tag or an sh tag, with the command between open and close.

    <shell>...</shell>

The first three carry a real command.
The fourth carries no command at all.
It is the model inventing a tool it was never given, an AgenticSearch tool-call skeleton with no code inside.

## One salvage covers the first three

Every shape above is the same event.
The model wants to run a shell or python command and does not write it as a Markdown fence.
tomo's oi engine already carries a per-model dialect for exactly this reason, so the fix lives where that idea already is.

The default markdown dialect now runs the Markdown fence parser first, and only when it finds no fence falls back to a fenceless salvage that recovers an XML tool-call carrying a code payload with an optional language, an HTML pre/code block with the language on the first line, and a language-named tag such as a shell, sh, bash, python, or py tag with open and close matched so a stray tag in prose never swallows the rest of the reply.

Each is anchored on its wrapper, so a bare code element in a documentation snippet is not mistaken for an action.
The salvage is reached only when there is no Markdown fence, so a model that writes fences, which this model still does on most turns, is completely unaffected.
Nothing here is tuned to the task.
It is model-dialect robustness that any task this model runs benefits from.

## The invented tool gets a nudge, not a salvage

The invented-tool shape is different and gets a different answer.
There is no command to recover, so salvage correctly returns nothing.

The finish guard already nudges a turn that ends with a clean worktree and a reply that reads like acting.
It now counts a leftover tool-call skeleton, a stray tool-call, tool-name, parameter, or function tag, as such a reply.
That branch runs only after every dialect and the salvage came up empty, so the skeleton's presence means the model tried to call a tool and the call was lost, which is the hallucination the nudge is for.
The cost of a wrong guess is one nudge, and the clean-worktree gate means a real edit suppresses it entirely.

## The finish-guard markers, corrected

The first read of the earlier failing draw called it a prose hallucination, and the plan was to widen the finish guard's acting markers to catch descriptive narration.
That was half right.
The narration was real, but folded inside it, in a pre/code or a shell tag, was a genuine command the parser had dropped, so the true fix was the salvage, not the marker.

The marker widening still went in, but narrowed.
Only first-person investigative intent, phrases like "i need to find" or "let me look", is added, which only makes sense mid solve.
A bare diagnosis noun like "the fix would be" or "the issue is" is deliberately left out so an answer-only turn is never nudged.
The existing test that treats a plain "the fix would be to" answer as a plain answer still holds.

## Lessons

- The dropped action was not a hallucination. A slice of one-request failures were the model emitting a real runnable command inside a non-markdown fence, and the harness threw it away as empty prose.
- One salvage covers three costumes. The XML tool-call, the HTML pre/code, and the language-named tag are the same event, a command not written as a Markdown fence, so one fenceless salvage behind the existing dialect seam recovers all three.
- The salvage never fires on a model that behaves. It runs only when no Markdown fence is present, so a model that writes fences on most turns is completely unaffected, and the salvage is anchored on its wrapper so a documentation snippet is not mistaken for an action.
- The invented tool is a lost call, not a dropped one. There is nothing to salvage, so the finish guard reads the leftover tool-call skeleton as a lost action and nudges once, gated on a clean worktree so a real edit suppresses it.
- The result held on a live batch. gitingest-94 went from roughly two passes in eight to seven in eight on this model, the one-request hard drops disappeared, and every shape is locked by a unit test.

## Reproduce

1. Build the lab against the local tomo checkout so the oi engine under test carries the salvage and finish-guard changes.
2. Source the opencode zen key so deepseek-v4-flash-free is reachable before the probe.
3. Run the upstream arm with the openinterpreter tool image and the tomo arm with the oi engine, both on gitingest-94 with grading on, as above.
4. Re-run the tomo arm several times, since the model is stochastic, and read each one-request failure's raw stdout to see the non-markdown fence in the reply.
5. Every run writes the priced summary, the full request and response trace, and the readable transcript, so the salvage firing shows as a language-named block followed by an execute result.
