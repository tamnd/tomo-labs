---
title: "kata's first live numbers: the hy3 A/B against oi"
linkTitle: "kata first live numbers"
description: "The new kata engine ran its first real workloads against tomo-oi on hy3-free, like for like: same binary, same fence parsing, only the loop policy differs. Core-14 came out level on passes (13/14 each) with kata 28% leaner on total tokens and faster on wall clock. gitingest-94 went 5/5 for both arms, and kata's one expensive rep is the first live trace of the round-budget governor firing and the run still converging to a pass. briefcase-2085 stayed a coin flip for both arms and surfaced a new fence costume from a different upstream provider, fixed in tomo #80."
date: 2026-07-17T19:30:00+07:00
---

The kata engine merged this morning (tomo #79) as the from-scratch loop that carries the campaign's proven mechanisms: oi's fence parsing and pace constants, plus the two levers oi does not have, a bounded reproduce-first finish guard and a whole-turn round budget.
Today it ran its first real workloads.
The M0 gate smoke on deepseek-v4-flash-free hit that model's daily quota mid-run (reset is midnight UTC), so the day's live work moved to hy3-free, which has its own quota.
That turned out to be the more interesting arm anyway: hy3 is the model behind the exp 0065 reproduce-first lever and the 260K briefcase runaway, exactly the behavior kata was built to bound.

## Setup

Both tools ran from the same images built today: tomo-kata pinned at ec25e2d, tomo-oi at ef3262a.
The two pins are the same tree except for the kata engine itself, and both engines share pkg/fence, so the A/B isolates loop policy.
Model hy3-free through the zen proxy, pass@1, greedy, no retries.
All numbers below are from result.json in the run dirs; fresh is prompt minus cached plus completion.

## Core-14

| arm | pass | total tokens | fresh | cached | reqs | wall sum |
|---|---|---|---|---|---|---|
| tomo-kata | 13/14 | 101,283 | 45,795 | 55,488 | 62 | 246s |
| tomo-oi | 13/14 | 140,229 | 51,141 | 89,088 | 69 | 317s |

Level on passes, and the fails are different tasks: kata missed 07-refactor-dedupe (left clamp defined twice), oi missed 03-bugfix-fizzbuzz (printed mean 25.0 for expected 18).
Both are single-rep coin flips on a weak model, not engine walls; 07-refactor-dedupe is the same scenario that flaked for oi in the deepseek baseline.
kata spent 0.72x of oi's total tokens, 0.90x of its fresh tokens, and finished in 0.78x of its wall time.

## gitingest-94, five reps each

| arm | pass | total per rep | reqs per rep |
|---|---|---|---|
| tomo-kata | 5/5 | 56,627 / 40,060 / 42,554 / 245,408 / 48,898 | 16 / 14 / 13 / 30 / 13 |
| tomo-oi | 5/5 | 48,689 / 45,267 / 93,339 / 57,892 / 63,007 | 14 / 14 / 22 / 17 / 17 |

Both arms sweep the task that used to sit at 3/5 for oi on this model before the M0 estate landed.
kata's typical rep is cheaper than oi's (median 48.9K vs 57.9K total), but its rep four ballooned to 245K.

That rep is the reason to keep it: it is the first live trace of the kata governor doing its job.
At message 49 of the transcript the loop injected both the no-edit nudge and the round-budget nudge into one user turn, and the model stopped broadening, wrote the fix, and passed at request 30, well under the hard stop of 48.
The round budget was the missing convergence bound the swebench sweep identified; this is it working on a real runaway in flight, converting what would have kept spending into a pass.

## briefcase-2085, three reps each

| arm | rep 1 | rep 2 | rep 3 |
|---|---|---|---|
| tomo-kata | FAIL 87,756 / 15 reqs | FAIL 2,637 / 1 req | FAIL 186,162 / 21 reqs |
| tomo-oi | PASS 94,382 / 16 reqs | FAIL 55,380 / 12 reqs | FAIL 113,396 / 18 reqs |

The task stays convergence-bound and flippy for both arms, consistent with exp 0061.
No kata rep ran away to the 260K+ the unbounded reproduce prompt produced in exp 0065; the worst rep stopped at 186K under the round budget.
oi's single pass in six briefcase runs today reads as the coin landing right once, not as a policy gap between the arms; more reps would be needed to claim a real difference.

kata's rep two is the finding.
It ended after one round because hy3 came back through a different upstream provider (Novita in the trace) wearing a costume the shared fence parser had never seen: the code wrapped in its own four-backtick fence pair inside the hash tool-call tags.
parseHashToolCall cuts the body at the first fence line to drop the stray trailing fence of the known shapes, so on this variant it cut at the opening fence and lost the code, and the turn ended on nothing run.
The fix (tomo #80) reads an optional fence opener after the language line and takes the code up to its close, with the new shape in the dialect tests.
The lesson generalizes: a free model's dialect is not fixed per model, it changes with the upstream provider the gateway routes to, so costume salvage has to keep meeting the model where it is per round, not per model id.

## Ops notes

Two traps cost time today and are worth writing down.
The lab must run from /private/tmp, not /tmp, when the checkout lives under the mac's tmp dir: the podman machine mounts /private and /Users but not the host /tmp symlink, so container mounts fail with statfs errors while every file plainly exists on the host.
And the deepseek-v4-flash-free quota is a per-model calendar-day bucket resetting at midnight UTC; the morning baseline had already spent most of today's, so the gate smoke got seven real tomo passes and then forty-nine rate-limited zero-token results, which were pruned from the run dirs rather than counted as fails.

## Next

The M0 gate smoke and the three flip runs rerun on deepseek-v4-flash-free after the quota reset, at the same pins as today.
The fence fix in tomo #80 is not in those pins and does not need to be: the gate runs on deepseek's markdown dialect, which the fix does not touch.
After the gate, the tool pins bump past #80 so future hy3 runs carry the fenced-body salvage.
