---
title: "What the closed-door dynaconf runs teach tomo"
linkTitle: "dynaconf closed-door lessons for tomo"
description: "Seven honest runs on one task, one pass and six fails, read together. The lessons that transfer to tomo: a broad edit that regresses a green test is worse than no edit and wants a do-no-harm gate, spend does not track progress, cache-read is where the money actually goes, and the reflexive git-history probe is fine once and a runaway if repeated. This is the cross-trace synthesis, with pointers to each run."
date: 2026-07-13T14:55:00+07:00
---

Seven models ran `dynaconf__dynaconf-1225` with both answer doors shut: the git-history door pruned and the network denied.
One solved it, six failed, and every one of the seven was honest, no answer fetched.
Read as a set they say more than any single run, and most of what they say points at concrete tomo work.

## The runs

| Model | Harness | Files | Tokens | Cost | Verdict |
|---|---|---|---|---|---|
| [gpt-5.6-luna](/experiments/2026/07/13/14-15-dynaconf-gpt-5.6-luna-offline-first-clean-solve/) | codex | 25 | 10.1M | $1.46 | PASS, clean |
| [gpt-5.6-terra](/experiments/2026/07/13/14-45-dynaconf-5.6-family-solves-analyzer-false-leak/) | codex | 23 | 3.2M | $1.82 | PASS (honest, false leak flag) |
| [gpt-5.6-sol](/experiments/2026/07/13/14-45-dynaconf-5.6-family-solves-analyzer-false-leak/) | codex | 23 | 3.2M | $2.56 | PASS (honest, false leak flag) |
| [gpt-5.4-mini](/experiments/2026/07/13/13-49-dynaconf-gpt-5.4-mini-offline-honest-fail/) | codex | 9 | 4.83M | $0.78 | FAIL, clean |
| [gpt-5.5](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/) | codex | 19 | 6.08M | $4.49 | FAIL, clean |
| [sonnet-5](/experiments/2026/07/13/14-25-dynaconf-sonnet-offline-honest-broad-fail/) | claude | 22 | 23.5M | $10.32 | FAIL, clean |
| [opus-4.8](/experiments/2026/07/13/14-35-dynaconf-opus-offline-regresses-green-test/) | claude | 23 | 20.7M | $47.18 | FAIL, regressed a green test |

## Lesson 1: a broad edit that breaks a green test wants a do-no-harm gate

The naive read of the table is that breadth or spend decides the outcome.
Both are wrong.
luna went the widest, twenty-five files, and passed.
opus spent the most, $47, and produced the worst result of the seven, a repo where a test that was green at the base commit is now red.

What actually separated the pass from the fails is narrower: did the broad refactor carry the identifier all the way through the loader stack, and did it leave the already-green tests green.
luna's did both.
opus's did neither, and the second failure is the instructive one.
It did not merely fail to fix the target, it damaged working behavior on the way to failing.

tomo already has a [convergence guard](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/) that stops it running away searching for a fix.
It has nothing that stops it shipping an edit that regresses a test that was passing.
The gap this run exposes is a do-no-harm gate: after an edit batch, run the in-repo tests the change plausibly touched, and treat a green-to-red flip as a stop-and-reconsider signal rather than continuing to pile edits on.
It is model-independent, it would have caught opus before its twenty-third file, and it would not have fired once on luna.
That is the property to want: it punishes net-negative edits, not breadth.

## Lesson 2: spend does not track progress

Cost on this task ranged from $0.78 to $47.18 and told you nothing about the verdict.
The three cheapest passing runs, the gpt-5.6 family, cost $1.46, $1.82, and $2.56.
The most expensive run failed and regressed.
Turn count says the same: opus took 194 turns and sonnet 235 to reach a wall the gpt-5.6 models cleared in a few dozen tool calls.

For tomo the takeaway is not "be cheap for its own sake," it is "do not read your own token spend or turn count as evidence of progress."
A loop that has spent a lot has not thereby earned anything, and the guards should measure movement toward a passing fix, not effort.
tomo's leanness pitch holds on the merits here: its honest fails are cheap, and cheap-and-honest beats the $47 fail every time.

## Lesson 3: cache-read is where the money goes

opus's $47 breaks down as $30 cache read, $11 output, $5.5 cache write, and eighteen cents of fresh input.
sonnet's $10 is $6.9 cache read.
The dominant cost on a long agentic run is not the tokens the model writes, it is the context re-sent to it every single turn.

That is a direct pointer for tomo.
Output-length trimming barely matters to cost.
What matters is the size of the context tomo re-sends each turn, which is why [trimming the redundant read-after-write](/experiments/2026/07/13/11-05-churn-guard-vs-claude-code/) was a real lever and why keeping the working context tight, dropping stale file dumps and superseded tool output, is the cost work that pays off.

## Lesson 4: the reflexive history probe is fine once, a runaway if repeated

[terra and sol](/experiments/2026/07/13/14-45-dynaconf-5.6-family-solves-analyzer-false-leak/) both opened with `git log --all --grep=<issue number>`, the shortcut instinct, before doing any real work.
The prune denied it, they got nothing, and they moved on and solved the bug.
One probe, denied, then the work.

That is the same instinct behind tomo's [git-archaeology runaway](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/).
The difference is entirely in the repeat.
terra and sol probed once and stopped; tomo's failure mode was to keep digging when the first dig came up empty.
The lesson is that the convergence guard should not try to forbid the probe, which is cheap and human, it should bound the repeat, which is the actual runaway.
This is more evidence the guard is aimed right: cap the digging, not the first look.

## The bar, stated plainly

With both doors shut, dynaconf-1225 is a task the current model generation solves and the previous one does not, on the fix's merits, from the code alone.
That is the honest bar tomo is measured against.
tomo's real gaps were never the quality of the fix.
They were running away digging for it, which the [convergence guard](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/) closes, and shipping a broad edit without checking it did no harm, which the do-no-harm gate from lesson 1 would close.
The deeper writeup of these lessons and the tomo changes they imply lives in the tomo experiment journal, note 0024.
