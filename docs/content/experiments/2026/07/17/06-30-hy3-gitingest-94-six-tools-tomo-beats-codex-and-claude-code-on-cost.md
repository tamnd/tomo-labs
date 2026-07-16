---
title: "hy3 gitingest-94, six tools: tomo passes and beats codex and claude-code on cost"
linkTitle: "hy3 gitingest-94 six tools"
description: "The same free model on the same task through six tools. Two of tomo's engines pass, and both cost a fraction of codex and claude-code."
date: 2026-07-17T06:30:00+07:00
---

The earlier three-tool run on this task compared pi, opencode, and tomo-oi.
This one widens the board to six tools on the same free model, hy3-free, the same
task, gitingest-94, and the same isolated container harness.
It adds tomo on three of its own engines and both heavy rival CLIs, so it answers
two questions at once: which tomo engine is best on a cheap model, and how tomo
stacks up against codex and claude-code there.

Priced at the DeepSeek reference rates the lab uses, fresh $0.27, cache-hit $0.07,
output $1.10 per million:

| tool | kind | result | reqs | total tokens | cost |
|---|---|---|---|---|---|
| pi | rival | PASS | 7 | 22,212 | $0.00320 |
| tomo-oi | tomo, oi engine | PASS | 9 | 29,027 | $0.00446 |
| tomo-cx | tomo, cx engine | PASS | 8 | 36,476 | $0.00472 |
| opencode | rival | FAIL | 7 | 52,110 | $0.00602 |
| codex | rival | PASS | 16 | 169,497 | $0.01670 |
| claude-code | rival | PASS | 9 | 199,518 | $0.02397 |
| tomo | tomo, default engine | FAIL | 4 | 11,522 | $0.00191 |

## Which tomo engine is best on hy3

Two of tomo's three engines solve the task, and the default one does not.

tomo-oi, the code-as-action loop, is the leanest tomo engine when it lands, about
29k tokens for the pass.
It is stochastic on this free model, passing in the majority of runs but swinging
between roughly 29k and 55k tokens when it does, the same free-model variance the
briefcase run showed.

tomo-cx, the codex-style loop, gave the steadiest single pass this sweep at 36k
tokens, slightly heavier than oi's best but without the swing.

tomo on its default engine failed.
The trace shows this is not a dialect or a stall problem.
hy3 spoke clean native tool calls through the structured path, the engine ran four
rounds and made edits, then stopped on a natural finish with a fix that missed the
hidden tests.
The default engine does not carry the extra convergence discipline that the oi and
cx engines use to keep going until the tests are green, so on a cheap model it
stops early and shallow.

So on this task the best tomo engine is oi on cost and cx on reliability, and the
default engine is the weak one.

## tomo versus codex and claude-code

Both passing tomo engines beat both heavy rival CLIs on cost while matching the
pass, by a wide margin.

codex passed but took sixteen rounds and 169k tokens, about 3.7 times the cost of
tomo-oi's pass for the same green.
claude-code passed at 200k tokens, about 5.4 times tomo-oi.
Both lean hard on caching to stay affordable, codex at 91 percent cache hit and
claude-code at 77 percent, and they still cost multiples of tomo because the round
count and the re-sent context dwarf the discount.

opencode, the other lightweight rival, failed the hidden tests at 52k tokens, so on
this task and model tomo passes where opencode does not.

pi is still the number to beat, a pass at 22k tokens.
tomo is now clearly in pi's band and far under the heavy CLIs, but leanest overall
here is still pi, and closing that last gap is a convergence question, fewer rounds
and less narration to the same fix.

## How it was run

    source ~/data/.local.env
    LAB_MODEL=hy3-free lab run <tool> cyclotruc__gitingest-94 --suite swebench-live

Free models route the trace proxy straight to opencode.ai/zen.
codex and claude-code speak their own wire shims, Responses and Anthropic Messages,
through the same proxy, and both routed to hy3 without a subscription bridge.
Every run is one attempt, pass at 1, no retry.
