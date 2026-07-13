---
title: "Paying five times more buys the same wrong fix"
linkTitle: "mini vs sol on python-control"
description: "Four real codex subscription runs, gpt-5.4-mini and gpt-5.6-sol on two tasks, priced through our new single source of truth. On python-control both models, cheap and flagship, converge on the identical edit and fail the identical three tests. The lever over tomo is not correctness. No model here solves the task. It is convergence and cost, and that is a lesson tomo can act on."
date: 2026-07-13T12:05:00+07:00
weight: 988
---

Four runs of the real `codex` CLI on its ChatGPT subscription, two models across two tasks, all priced through the [pricing table](/guides/) we now keep as a single source of truth.
The point is not which model is best.
The point is what the money buys on a task nobody solves.

## Reproducibility

| Run | Model | Task | Tokens | List price | Verdict |
|---|---|---|---|---|---|
| A | `gpt-5.4-mini` | python-control | 1,056,067 | $0.1939 | FAIL |
| B | `gpt-5.6-sol` | python-control | 971,213 | $0.9473 | FAIL |
| C | `gpt-5.4-mini` | dynaconf | 2,785,448 | $0.3998 | genuine edit, FAIL |
| D | `gpt-5.6-sol` | dynaconf | 827,769 | $0.9790 | leaked pass, see note |

All four are subscription runs, so no dollar changed hands.
The list price is what the same tokens would have cost on the metered API at each model's published rate, computed from the [pricing package](/guides/) so a subscription run lines up against tomo on its metered proxy.
Run D's pass is the one retired in the [dynaconf answer-leak report](/experiments/2026/07/13-dynaconf-sol-answer-leak-closed/): it read the fix out of git rather than reasoning it, and the harness that let it is now fixed.
That leaves the honest comparison on python-control, runs A and B.

## The task, in one line

`python-control` has a bug in `zpk`, its zero-pole-gain constructor.
At the base commit a handful of `frd` frequency-response cases fail because a timebase argument does not thread through.
The real fix threads it. The tempting fix does not.

## What both models did

The same thing.
Both A and B changed the `zpk` signature the same way, dropping the explicit `dt=None` in favor of catching it in the catch-all:

```python
# both gpt-5.4-mini and gpt-5.6-sol, independently
-def zpk(zeros, poles, gain, dt=None, **kwargs):
+def zpk(zeros, poles, gain, *args, **kwargs):
```

Both then failed the same three `frd` timebase cases, because folding `dt` into `*args` does not actually carry the timebase to where the frequency-response code reads it.
It is a plausible edit that type-checks, runs, and is wrong.
The cheap model reached it in a million tokens and the flagship reached it in a million tokens, and they landed on the same character-for-character change.

Five times the price bought the identical wrong fix.
On this task there is no rate you can pay codex that turns the cell green.

## Why this matters for tomo

tomo also fails python-control.
So the failure is not tomo being weak where the frontier is strong.
The whole reachable field, a $0.75-per-million mini and a $5-per-million flagship, makes the same mistake tomo does.
When we say "if tomo failed but other tools succeeded, that is a lesson," the honest reading of python-control is the opposite: nobody succeeds, so there is no correctness lesson to import here.

The lesson that is real is about cost.
Both codex models spent about a million tokens and stopped.
tomo, on tasks in this neighborhood, does not stop.
On the neighboring dynaconf task tomo ran [four million tokens of git archaeology](/experiments/2026/07/13-dynaconf-tomo-git-archaeology-runaway/) and never edited a file, where mini reached a genuine multi-file edit (run C) in 2.8M and sol read the answer in under a million.
The frontier models converge, right or wrong, and then they halt.
tomo, when it is wrong, keeps paying.

So the enhancement target is not "make tomo pass python-control," which no reachable model can do.
It is convergence: give tomo the same stopping discipline the frontier models show, so a wrong task costs a million tokens and a halt, not four million and a runaway.
That is a lever we own, it is measurable in tokens, and it is the thread the next tomo change picks up.

## Reproduce it

```bash
# price any run through the single source of truth
go run ./cmd/lab codex analyze \
  ~/.codex/sessions/2026/07/13/rollout-<python-control-mini>.jsonl
go run ./cmd/lab codex analyze \
  ~/.codex/sessions/2026/07/13/rollout-<python-control-sol>.jsonl

# the identical zpk edit, in each run's patch
go run ./cmd/lab codex analyze --patch ~/.codex/sessions/2026/07/13/rollout-<...>.jsonl | grep zpk
```
