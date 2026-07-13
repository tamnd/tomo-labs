---
title: "The write-churn runaway, bounded, and tomo failing cheaper than claude-code"
linkTitle: "churn guard vs claude-code"
description: "The third and last of tomo's runaway shapes. A turn that keeps editing but never converges, writing scratch scripts or the same file over and over, now stops instead of burning a hundred rounds. The two tasks that showed it are read next to claude-code on the same model, where both fail and tomo fails on fewer tokens."
date: 2026-07-13T11:05:00+07:00
---

This closes the set.
tomo had three runaway shapes in the swebench-live sweep, and the first two are already fixed: a turn that [repeats calls](https://github.com/tamnd/tomo/pull/55) and a turn that [investigates without ever editing](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/).
The third is the mirror image of the second: a turn that keeps editing but never converges, so the fix never lands.
Two runs showed its two faces, and this is the bound that stops both, read next to claude-code failing the same two tasks the same expensive way.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13 (GMT+7) |
| Tools | tomo, guarded build (commit `77fe8ad`, PRs #58 and #59); claude-code, latest, both autonomous |
| Model | `deepseek-v4-flash-free` on the OpenCode Zen free tier, the same model drew every run |
| Harness | tomo-labs, `LAB_CONCURRENCY=1`, per-attempt wall ceiling 900s |
| Tasks | `python-control__python-control-1064` and `instructlab__instructlab-2540`, each at its base commit, graded on the host |
| Verdict | all four runs FAIL the hidden tests; the difference is the cost |

## The shape

The repeat guard ends a turn that repeats calls it already made.
The no-edit guard ends one that searches and reads round after round and never writes.
Neither sees a turn that writes constantly but never finishes, because every round writes something new.
Two captured runs are exactly that:

- python-control made 34 edits and still failed, because 33 of them were throwaway debug scripts (`reproduce_issue.py`, `debug_zpk.py`, `trace_tf_creation.py`) it wrote to watch the bug rather than fix it. One edit touched real source, never tested, then it ran 119 rounds into the wall.
- instructlab edited the same three source files 20 times over, one file fourteen times, thrashing on a change that would not take across 128 rounds.

## What changed in tomo

The new bound watches the volume of writes, not their novelty or their absence.
A real fix on these tasks takes a handful of edits, so a turn that has written many times over and still not ended is churning.
It nudges once, naming both shapes directly, scratch scripts written to watch the bug and the same file edited over and over, to settle on a single tested fix.
If the writing does not stop, the guard ends the turn the way the other two guards end their runaways.

The threshold is not guessed.
Replaying every captured run through the full governor, the healthiest real run wrote six files, so the nudge sits at twelve and the hard stop at sixteen, well clear of any productive run.
In that replay the churn bound fires on exactly these two tasks and touches nothing else: every run that finished on the model's own choice still does.

A second, smaller change (`#59`) came out of the same replay.
The no-edit bound was ending one run, fonttools, two rounds before its own first edit, because that run reads deeply, for 41 rounds, before it writes a correct fix at round 42.
The bound is lifted so a genuinely deep read-before-edit investigation reaches its change, while the git-archaeology runaway that never edits at all is still caught with wide margin.

## The two runs, guarded against the old runaway

The pre-guard runs drew a different free model, so the honest, model-independent number is the request count.

| task | build | model calls | wall |
|---|---|---|---|
| python-control | pre-guard | 120 | hit the 900s wall |
| python-control | guarded | 46 | 331s |
| instructlab | pre-guard | 129 | 499s |
| instructlab | guarded | 37 | 137s |

python-control drops from 120 model calls off the wall to 46 in 331 seconds.
instructlab drops from 129 to 37 and from 499 seconds to 137.

## Same task, same model, claude-code

To check these are fair losses and not tomo-specific ones, claude-code ran both tasks on the identical model.
It failed both, and it failed them the expensive way.

| task | tool | calls | tokens | wall |
|---|---|---|---|---|
| python-control | tomo (guarded) | 46 | 1,077,109 | 331s |
| python-control | claude-code | 66 | 2,518,928 | 398s |
| instructlab | tomo (guarded) | 37 | 637,376 | 137s |
| instructlab | claude-code | 37 | 874,442 | 214s |

Both tools fail both tasks, because neither is one-pass reachable on this model.
When the outcome is a loss for everyone, the tool that loses cheapest wins: on python-control tomo makes 30% fewer calls and spends 57% fewer tokens, and on instructlab it matches the call count while spending 27% fewer tokens and finishing in two-thirds the time.
claude-code failing both also confirms the tasks are a fair loss for the field, not a hole specific to tomo.

## The lesson

Three runaway shapes, three bounds, all grounded in real traces rather than a fixed round cap.
The governor now stops a spin, an investigation that never commits, and an edit storm that never converges, and it does each without cutting the productive runs, which the replay checks one by one.

What none of this does is turn a fair loss into a win.
On these hard tasks a free model cannot find the fix in one pass, and no amount of loop discipline changes that.
Failing cheaply is the right outcome when the task cannot be won, and tomo now does it better than the strongest rival on the same model.
Turning the loss itself into a win is the next problem, and it is a model-capability problem, which is why the next thread studies what a strong model actually does on these tasks.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_CONCURRENCY=1
go run ./cmd/lab run tomo        python-control__python-control-1064 --suite swebench-live
go run ./cmd/lab run claude-code python-control__python-control-1064 --suite swebench-live
go run ./cmd/lab run tomo        instructlab__instructlab-2540 --suite swebench-live
go run ./cmd/lab run claude-code instructlab__instructlab-2540 --suite swebench-live
```

The tasks, their graders, and their base commits are committed, and the tomo image is pinned to the guarded commit, so a rerun on the same model lands on the same shape, free-tier rate limits on the day permitting.
