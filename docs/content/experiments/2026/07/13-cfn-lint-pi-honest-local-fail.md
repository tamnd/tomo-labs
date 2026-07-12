---
title: "cfn-lint: pi fails it the honest way too"
linkTitle: "cfn-lint pi"
description: "A second rival on the cfn-lint task. pi never leaves the repo, never fetches the pull request, and fails exactly where tomo failed: its source change does not produce the arbitrary graded wording. A short confirm, with the caveat that the free-tier rate limit cut the run short."
date: 2026-07-13T01:21:00+07:00
weight: 995
---

This is a single run: pi, on `aws-cloudformation__cfn-lint-3798`, the same task tomo failed and opencode passed.
It is a confirm, not a headline, and it is short on purpose.
It exists to answer one question left by the pair before it: when a tool does honest local work on this task, without fetching the answer, does it fail the way tomo did?
pi says yes.

The two reports it follows are worth reading first: [tomo, which fixed the issue as written and failed](/experiments/2026/07/13-cfn-lint-tomo-issue-literal/), and [opencode, which passed by fetching the merged pull request](/experiments/2026/07/13-cfn-lint-opencode-answer-lookup/).

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13 01:21 (GMT+7) |
| Tool | pi, pinned image on the swebench-live suite |
| Model | `hy3-free`, which resolves to `tencent/hy3-20260706:free` on the OpenCode Zen free tier |
| Harness | tomo-labs, run at `LAB_CONCURRENCY=1` |
| Task | `aws-cloudformation__cfn-lint-3798`, the cfn-lint repo at base commit `d5c3da9`, graded in a Python 3.12 venv on the host |
| Verdict | FAIL, "hidden tests not satisfied". 45,814 tokens, 12 model calls, 169.5 MB peak memory. Rate-limited once mid-run (see the caveat) |

```bash
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run pi aws-cloudformation__cfn-lint-3798 --suite swebench-live
```

## What pi did

pi stayed in the repository the whole run.
Its trace shows no web fetches, and in particular no request to the pull request that opencode leaned on.
It read `src/cfnlint/jsonschema/_keywords.py`, the file that actually needed to change, but it did not rewrite the generic validator messages there.
At the end of the run that file still emits the old wording: a grep for the graded format, "expected maximum item count: N, found: M", finds zero matches in pi's source.

So pi failed for the same structural reason tomo did.
The grade demands an arbitrary maintainer wording that appears nowhere in the checked-out repo, and pi, working only from the repo, never produced it.
It is the [tomo failure](/experiments/2026/07/13-cfn-lint-tomo-issue-literal/) again, from a different tool: honest local work against a target that only lives in the answer pull request.

## The caveat

This run is a degraded confirm, and it would be dishonest to present it as a clean one.
pi was rate-limited once during the run, and the run ended early, at twelve model calls and 45,814 tokens, well short of a tool that pushes a task to exhaustion.
The free-tier `hy3-free` bucket was being drained across the day, and pi's 429 is the same rate limit that later stopped this lab from running further rivals on the model.

So the strong claim, "pi tried as hard as it could and still failed", is not one this run supports.
The claim it does support is narrower and still useful: in the calls it did make, pi did not attempt the web and did not reach the graded source change.
Its direction was the honest-local one, the losing one on this particular task, and nothing in the run suggests it was about to fetch the pull request.

## What the three runs say together

- tomo, hy3-free: FAIL, 212,648 tokens. Implemented the issue's literal message, locally.
- pi, hy3-free: FAIL, 45,814 tokens, rate-limited. Honest local work, no web, source wording never changed.
- opencode, hy3-free: PASS, 940,995 tokens. Fetched the fixed source on `main` and `pull/3798.diff`, then copied the exact wording.

The pattern is clean even with pi's run cut short.
The only tool that passed is the one that went to GitHub and read the fix.
Both tools that stayed in the repo failed, because the graded wording is not in the repo to be found.
This is the evidence behind keeping cfn-lint in the suite as a marker and not teaching tomo to chase it: the task rewards fetching the answer, which is not the skill this lab is trying to grow in tomo.

## Reproduce it

```bash
go run ./cmd/lab build pi
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run pi aws-cloudformation__cfn-lint-3798 --suite swebench-live
go run ./cmd/lab inspect pi aws-cloudformation__cfn-lint-3798 --suite swebench-live
```

The free-tier rate limit is a property of the day the run was captured, not of the task, so a rerun after the daily reset may get further before it lands on the same verdict.
