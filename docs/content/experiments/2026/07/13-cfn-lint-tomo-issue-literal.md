---
title: "cfn-lint: tomo fixes the issue, the grade wants something else"
linkTitle: "cfn-lint tomo"
description: "tomo reads a cfn-lint issue, implements exactly the message it asks for, and fails the grade. The graded wording is a generic validator message the maintainers changed instead, and it appears nowhere in the checked-out repo. A close read of one tomo run, and the question it leaves open."
date: 2026-07-13T01:00:00+07:00
weight: 15
---

This is a single run: tomo, on one real GitHub issue from the [swebench-live](/evals/swebench-live/) tier.
It failed.
The reason is not a bug in tomo's code and not a bad task, exactly.
It is that the answer the grade wants is nowhere tomo could see it, and tomo did the honest thing with what it had.
The companion report, [opencode on the same task](/experiments/2026/07/13-cfn-lint-opencode-answer-lookup/), shows where the answer actually lived.

## Reproducibility

Everything you need to run this exact run again.

| | |
|---|---|
| Run captured | 2026-07-13 01:00 (GMT+7) |
| Tool | tomo, commit `4c13896f0233`, run with `--yolo` |
| Model | `hy3-free`, which resolves to `tencent/hy3-20260706:free` on the OpenCode Zen free tier |
| Harness | tomo-labs, run at `LAB_CONCURRENCY=1` |
| Task | `aws-cloudformation__cfn-lint-3798`, the cfn-lint repo at base commit `d5c3da9`, graded in a Python 3.12 venv on the host |
| Verdict | FAIL, "hidden tests not satisfied". 212,648 tokens, 26 model calls, 110.5 MB peak memory |

```bash
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo aws-cloudformation__cfn-lint-3798 --suite swebench-live
```

## The task

Every task in this tier is a real GitHub issue, handed to the tool with the project checked out at the commit just before the fix.
The tool changes the source so a hidden test suite it never sees goes green.

The cfn-lint issue, "Enhance E1011 Message", asks in plain words for one specific error message to change.
When a `FindInMap` nests too deeply, the reporter wants the error to read:

> FindInMap only supports up to two levels of nesting for map lookups

That is the whole ask: one message, spelled out verbatim.

The fix the maintainers merged does not do that.
It ignores the reporter's suggested sentence and rewrites the generic validator messages shared across the whole linter, in `src/cfnlint/jsonschema/_keywords.py`.
`maxItems`, `minItems`, `maxLength`, `minLength`, `maxProperties`, `minProperties`, `uniqueItems`, `uniqueKeys`: all move to one new format.

```
expected maximum item count: 2, found: 3
```

The hidden test suite asserts this exact wording across twenty-six cases.
Not the sentence from the issue, this format.

## What tomo did

tomo read the issue and did exactly what it said.
It added a `custom_msg` override on the `FindInMap` schema so the deep-nesting error reads the sentence the reporter asked for.
It is a clean, correct, local response to the text it was handed, produced without leaving the repository, for 212,648 tokens.

It failed, because the grade does not want that sentence.
It wants "expected maximum item count: N, found: M", and tomo never wrote that, because it had no reason to.

## Was the graded wording reachable?

This is the check that matters, the same discipline from the [faker](/experiments/2026/07/13-faker-yolo-autonomous-fix/) and [mesa](/experiments/2026/07/12-mesa-clear-agents/) write-ups: could the tool have found "expected maximum item count: N, found: M" from what it was given?

Inside the checked-out repository, no.
At the base commit `d5c3da9`, that string appears in zero files.
`git grep` across both `src/` and `test/` returns nothing.
The base `_keywords.py` still emits the old "is too long" phrasing, and no test at that commit asserts the new one.

So from local reading and reasoning alone, the target is an arbitrary maintainer word choice that cannot be recovered.
There is nothing in the repo that points at it, and the issue points the other way, at a different, FindInMap-specific sentence.

That leaves one open question, which the companion report answers: if the wording is nowhere in the repo, is it anywhere at all?
It is.
It is in the merged pull request on GitHub, and [opencode reached it by fetching that pull request](/experiments/2026/07/13-cfn-lint-opencode-answer-lookup/).

## The lesson for tomo

This one is deliberately not a lever.

tomo did the honest engineering thing: it read the issue, implemented what it asked, locally, at a quarter of the cost the winning rival paid.
The reason it lost is that the graded answer is a specific historical wording that lives only in the pull request that fixes the issue, which is the answer key of a benchmark built from merged pull requests.
Recovering it requires fetching that pull request, not understanding the code.

We keep the task in the suite, because our rule is not to drop tasks, and it is a useful marker of this failure mode.
We are not going to teach tomo to fetch the fixing pull request to turn this cell green.
A green bought that way measures willingness to open the answer key, not skill, and that is not the tool we are building.

The wider lesson is about reading the benchmark, and this lab got it wrong the first time: "it is not in the repo" is not the same as "it is not reachable".
The next report traces the second place a real merged fix always lives.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo aws-cloudformation__cfn-lint-3798 --suite swebench-live

# read the trace turn by turn
go run ./cmd/lab inspect tomo aws-cloudformation__cfn-lint-3798 --suite swebench-live
```

The task, its grader, and the hidden answer are committed, so a rerun on the same commit and model lands on the same verdict.
