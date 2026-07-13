---
title: "dynaconf: the guard that stops the runaway, and pi running straight into the wall"
linkTitle: "dynaconf tomo guard vs pi"
description: "The follow-up to tomo's git-archaeology runaway. tomo now bounds a turn that investigates without ever editing, so the same dynaconf run stops at 41 requests instead of 132 and 1.7 million tokens instead of four million. Read next to pi on the same task and the same model, which has no such bound and burns thirteen million tokens straight into the fifteen-minute wall."
date: 2026-07-13T10:05:00+07:00
weight: 990
---

This is the sequel to [tomo's dynaconf runaway](/experiments/2026/07/13-dynaconf-tomo-git-archaeology-runaway/), and it is the run that closes the loop on it.
That report found a real lever in tomo: a turn could investigate forever, running one distinct git command after another, and never trip the loop guard because every call looked new.
The fix landed in [tomo](https://github.com/tamnd/tomo) as a second bound that watches progress instead of novelty, and this is the same task rerun on the guarded build.
It is worth reading next to pi on the same task and the same model, because pi still does what tomo used to do, and the contrast is the whole point.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13 10:05 (GMT+7) |
| Tools | tomo, guarded build (commit `8a7383a`); pi, latest, both `--yolo`-equivalent autonomous on the swebench-live suite |
| Model | `deepseek-v4-flash-free` on the OpenCode Zen free tier, the same model drew both runs |
| Harness | tomo-labs, run at `LAB_CONCURRENCY=1`, per-attempt wall ceiling 900s |
| Task | `dynaconf__dynaconf-1225`, the dynaconf repo at its base commit, graded in a Python 3.12 venv on the host |
| Verdict | both FAIL the hidden tests; the difference is the cost, below |

## What changed in tomo

The original run failed in the most expensive way possible: 132 model calls, four million tokens, zero edits, killed at the wall.
The cause was a single blind spot.
tomo's loop guard ends a turn that spins on calls it already made, but it resets the moment a call is even slightly new, so 88 distinct git commands slipped past it while the run never wrote a line.

The new guard is orthogonal to that one.
It watches for progress, not novelty: a round that writes no file deepens a counter, and any file edit clears it.
After a stretch of investigation with nothing written it nudges the model once, naming the trap directly, that a repository checked out at the buggy commit holds no fixing commit to mine.
If the investigation still does not produce an edit, the guard ends the turn the way the repeat guard ends a spin, rather than running to the wall.

## The same run, guarded

On the rebuilt image the dynaconf run reproduced the exact shape: forty rounds, every one a bash, read, or plan call, not a single edit.
The nudge fired once, as designed, and the guard then stopped the turn at its bound.

| metric | original runaway | guarded build | change |
|---|---|---|---|
| model requests | 132 | 41 | 69% fewer |
| total tokens | 4,007,703 | 1,685,708 | 58% fewer |
| wall time | hit the 900s wall | 174s | about 5x faster |

The request count is the honest, model-independent number: the turn ends at the round-40 bound no matter which free model the run draws.
The task still fails, because bounding a runaway does not find dynaconf's fix, which is a harder problem.
What the guard delivers is exactly its promise: the same losing task now costs 58% fewer tokens and finishes five times faster instead of burning four million tokens to the wall.

## pi, same task, same model, no guard

To check that dynaconf is a fair loss and not a tomo-specific one, pi ran the identical task on the identical model.
It failed too, and it failed the way tomo used to.

| tool | verdict | requests | tokens | wall |
|---|---|---|---|---|
| tomo (guarded) | fail | 41 | 1,685,708 | 174s |
| pi (no guard) | fail | 129 | 13,077,875 | 900s (hit the wall) |

pi ran 129 requests and burned over thirteen million tokens, more than three times tomo's original runaway, and still hit the fifteen-minute wall with nothing to show.
This says two things at once.
dynaconf is genuinely not solvable in one pass on this model, since a strong rival cannot crack it either, so failing is the honest outcome for both.
And when the outcome is a loss for everyone, the tool that loses cheaply wins: tomo now fails on 87% fewer tokens and 5x faster than pi, on the same task and the same model, because it knows when to stop and pi does not.

## The lesson

The first dynaconf report ended by naming the lever.
This one is the lever pulled, merged, and measured live.
The enhancement does not turn a loss into a win, and it does not pretend to.
It does the thing a good agent must do that a runaway cannot: recognise that an investigation has stopped producing edits and stop paying for it.
Against a rival with no such discipline, on a task neither can win, that is the difference between failing at 1.7 million tokens and failing at 13.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_CONCURRENCY=1
go run ./cmd/lab run tomo dynaconf__dynaconf-1225 --suite swebench-live
go run ./cmd/lab run pi   dynaconf__dynaconf-1225 --suite swebench-live
go run ./cmd/lab inspect tomo dynaconf__dynaconf-1225 --suite swebench-live
```

The task, its grader, and the base commit are committed, and the tomo image is pinned to the guarded commit, so a rerun on the same model lands on the same shape, free-tier rate limits on the day permitting.
