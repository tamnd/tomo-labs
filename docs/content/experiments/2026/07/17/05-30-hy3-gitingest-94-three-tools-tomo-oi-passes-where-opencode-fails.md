---
title: "One free model, three tools, one task: tomo-oi loses, then two harness fixes make it win where opencode cannot"
linkTitle: "hy3 three-tool A/B, tomo-oi fixed to pass"
description: "The reference column set up a fair fight on the ground the product cares about, so this slice runs it: one free model, hy3-free, through three tools, pi and opencode and tomo-oi, on one task, gitingest-94, in the same isolated harness. pi passes at 22,212 tokens, opencode fails at 52,110, and tomo-oi loses worst of all by quitting in a single round. That is not a weak model, because pi drove the same model to green, so it is two gaps in tomo-oi's own loop. One is an exploration-preamble stall, where hy3 announces it will start exploring and stops, and tomo-oi's acting-nudge did not recognise the phrasing. The other is that hy3 speaks a hash-suffixed tool-call dialect that tomo-oi parsed as empty, so hy3 ran nothing for nine rounds even though on round eight it wrote the exact one-line fix. Both are model-shape repairs, not task tuning, and with them tomo-oi passes at 29,027 tokens, solving a task opencode cannot, close behind pi. The fixes are in tomo PR 74."
date: 2026-07-17T05:30:00+07:00
---

The reference column ended by promising a fair fight on the ground the product cares about.
This is it: one free model, three tools, one task, everything else held equal.

The model is hy3-free.
The task is gitingest-94.
The tools are pi, opencode, and tomo-oi, each driving the same model through the same trace proxy, in the same isolated container, with the network off and the task's hidden tests as the grader.

Here is the first result.

| tool | result | reqs | tokens |
|---|---|---|---|
| pi | pass | 7 | 22,212 |
| opencode | fail | 7 | 52,110 |
| tomo-oi | fail | 1 | 886 |

tomo-oi lost, and it lost worst.
It quit after a single round and 886 tokens.
That cannot be blamed on the model, because pi took the same model to green, so the loss is in tomo-oi's loop, and the trace shows two separate gaps.

## The stall

hy3 opened the solve with one sentence and stopped.

    Let me start by exploring the repository structure and the relevant file.

The model stopped naturally at fourteen tokens with no code and nothing done, and the turn was over.
tomo-oi has a guard built for exactly this.
When a turn ends with no runnable block, it checks whether the reply reads like the model was in the middle of acting, and if so it nudges once instead of ending the run.
But the list of phrasings that count as acting held the verb-specific ones, let me look, let me find, let me check, and not the exploration-preamble ones, let me start, let me begin, exploring.
So the guard read hy3's opener as idle chatter, the nudge never fired, and the run ended on the plan.

The fix adds the preamble shapes to the list.
With it, tomo-oi nudges hy3 to act, and the run goes from one round to nine.
That did not pass the task on its own, because of the second gap, but it is the difference between engaging the model and abandoning it at the door.

## The dialect

Nine rounds in, tomo-oi still failed, and again the trace said why, and again it was not the model's reasoning.
hy3 does not write a Markdown fence.
It emits a tool call whose tags carry a hex message id.

    <tool_calls:6124c78e>
    <tool_call:6124c78e>shell
    <tool_call:6124c78e>![CDATA[
    cat /work/src/gitingest/parse_query.py
    ]]</parameter>
    </invoke>
    </tool_call:6124c78e>
    </tool_calls:6124c78e>

None of that is the clean tool call the existing salvage reads, so every block parsed as empty.
The proxy trace shows the same result round after round: language shell, code blank.
hy3 ran nothing, saw no output, wrote "the shell output isn't displaying", and eventually gave up.

The part that stings is that hy3 had already solved it.
On round eight, inside a heredoc that parsed as empty and never ran, it wrote the exact fix the task needs.

    old: if not url.startswith("https://"): url = "https://" + url
    new: if "://" not in url:                url = "https://" + url

Right diagnosis, right patch, dropped on the floor by the parser.

The fix is a dialect for this shape.
Take everything after the first hash opener, cut the trailing fence the model appends, strip the hash tags and the CDATA and wrapper noise, read the first language word, and take the rest as code.
It falls back to the fence parser for the rounds hy3 does write a clean fence, and the prompt hint steers hy3 toward the fence in the first place.
hy3 is routed to the dialect by its model id, the same way the JSON and XML speakers already are.
This is the whole idea of the campaign in one change: meet the model where it is, follow its dialect instead of fighting it, and add a new model's costume as one registry entry and one parse function.

## The fixed result

| tool | result | reqs | tokens |
|---|---|---|---|
| pi | pass | 7 | 22,212 |
| opencode | fail | 7 | 52,110 |
| tomo-oi | pass | 9 | 29,027 |

tomo-oi now passes gitingest-94 on hy3, solving a task opencode cannot, at 29,027 tokens against opencode's wasted 52,110, close behind pi's 22,212.
It is not yet leaner than pi here, because pi's native tool call gets hy3 to green in fewer rounds, and closing that gap is a later slice.
Passing at all was the wall, and it is down.

Both fixes are model-shape repairs, not task tuning.
The preamble markers help any model that narrates its plan and stops.
The dialect helps any model that speaks the hash-tagged tool call.
Neither one reads the task's hidden test.

## What it sets up

The three-tool A/B did its job on the first task it touched.
It found two concrete, general gaps by watching a free model that a rival drove to green and tomo-oi did not, and both are now closed in tomo PR 74.
The next slice widens the same A/B across more tasks, and runs tomo-oi against the other free models to see which of them arrive in a costume of their own.
