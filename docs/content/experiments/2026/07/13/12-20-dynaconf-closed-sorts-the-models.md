---
title: "With the leak closed, dynaconf sorts the models"
date: 2026-07-13T12:20:00+07:00
description: "The same leak-free dynaconf task passes on gpt-5.6-sol and gpt-5.5 and fails on gpt-5.4, so the fix that removed the git shortcut left a task that actually measures the model."
---

The [previous report](../13-dynaconf-sol-answer-leak-closed) proved two things about dynaconf-1225.
The gold commit used to be reachable in the work tree, so a model could pass by copying it.
After we stripped the future history, gpt-5.6-sol reran and passed by writing a real parser instead.

That answered whether the task was still solvable.
It did not answer whether the task still tells strong models from weak ones, which is the whole reason to keep it.
So we ran the closed checkout on two more subscription models the user asked about, gpt-5.5 and gpt-5.4, both at high reasoning, pass@1, no retry.

## The result splits

| model | verdict | how it solved | tool calls | output tok | total tok | list cost |
|---|---|---|---|---|---|---|
| gpt-5.6-sol | PASS | surgical, `parse_conf.py` only | 34 | 9,238 | 1,399,161 | $1.2951 |
| gpt-5.5 | PASS | `Insert` class plus a loader refactor across 13 files | 109 | 23,885 | 6,171,346 | $4.4752 |
| gpt-5.4 | FAIL | same broad refactor, regressed passing tests | 109 | 23,456 | 3,575,561 | $1.9903 |

Every one of them was denied the shortcut.
`git cat-file -e da0054e` fails in all three work trees, so none of them could reach the gold commit.
The verdicts are earned.

## What each model did

gpt-5.6-sol stayed inside `dynaconf/utils/parse_conf.py`.
It added a quoted-string helper and an index parser, touched one file, and stopped.
Thirty-four tool calls, one file, green.

gpt-5.5 reached the same passing state by a longer road.
It wrote the same kind of `Insert` MetaValue, then kept going and threaded a new `identifier` argument through every loader, base, cli, and the validator, thirteen files in all.
The extra work did not break anything, so it passed, but it cost 109 tool calls and 6.17M tokens to get there, more than four times the sol run's bill.

gpt-5.4 attempted that same broad refactor and got it wrong.
Its `identifier` plumbing changed loader behavior enough to regress three tests that were green at the start and leave five of the target tests red.
The grader is strict about pass_to_pass, so a fix that trades a green test for the target is not a fix.

```
FAIL: hidden tests not satisfied
  FAIL_TO_PASS not green: test_load_using_settings_loader ...
  PASS_TO_PASS regressed: test_load_using_settings_loader_with_environments ...
9 failed in 0.16s
```

## Why this matters

Before the strip, any model that thought to run `git diff base..fix` would have passed regardless of skill, so the task measured nothing.
After the strip it separates three models cleanly: two solve it, one cannot, and the two that solve it do so at very different cost.

The lever is not correctness alone, it is convergence.
sol converges: it finds the one file that owns the bug and edits only that.
gpt-5.5 is correct but sprawls, spending four times the tokens to reach the same green.
gpt-5.4 sprawls and diverges, refactoring so widely that it breaks the code it was supposed to leave alone.

That is the failure mode to watch for in tomo.
The gap between a strong run and a weak one on this task is not "found the fix" versus "did not."
It is a surgical edit versus a runaway refactor that regresses working tests, which is exactly the churn our guards are meant to catch.

One footnote on the numbers.
The analyzer counted 20 writes for gpt-5.5 and only 3 for gpt-5.4, even though both touched 13 files.
gpt-5.4 edited through shell commands rather than apply_patch, and the codex trace encodes those edits in a form the analyzer does not yet decode, so its write count is undercounted.
That is a known gap in the trace parser, tracked separately, and it does not affect the verdicts, which come from the grader running the tests.
