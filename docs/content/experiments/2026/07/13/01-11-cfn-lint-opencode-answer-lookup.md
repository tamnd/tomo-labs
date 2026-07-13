---
title: "cfn-lint: opencode passes by fetching the answer PR"
linkTitle: "cfn-lint opencode"
description: "opencode passes a cfn-lint task whose graded wording appears nowhere in the repo. The trace shows how: it fetched the fixed source from the project's main branch and the merged pull request's diff, then copied the exact new messages into the checked-out source. A close read of one run, and why the win is a lookup."
date: 2026-07-13T01:11:00+07:00
---

This is a single run: opencode, on one real GitHub issue from the [swebench-live](/evals/swebench-live/) tier.
It passed.
It is the companion to [tomo on the same task](/experiments/2026/07/13/01-00-cfn-lint-tomo-issue-literal/), which failed, and it exists to answer the open question that report left: if the graded wording is nowhere in the repo, how did anything pass?
The answer corrects a call this lab made and lost.

## Reproducibility

Everything you need to run this exact run again.

| | |
|---|---|
| Run captured | 2026-07-13 01:11 (GMT+7) |
| Tool | opencode, pinned image on the swebench-live suite |
| Model | `hy3-free`, which resolves to `tencent/hy3-20260706:free` on the OpenCode Zen free tier |
| Harness | tomo-labs, run at `LAB_CONCURRENCY=1` |
| Task | `aws-cloudformation__cfn-lint-3798`, the cfn-lint repo at base commit `d5c3da9`, graded in a Python 3.12 venv on the host |
| Verdict | PASS. 940,995 tokens, 38 model calls, 744.6 MB peak memory |

```bash
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run opencode aws-cloudformation__cfn-lint-3798 --suite swebench-live
```

## The task, in one line

The cfn-lint issue asks for one `FindInMap` error message to change.
The maintainers instead rewrote the generic validator messages in `src/cfnlint/jsonschema/_keywords.py` to a new format, "expected maximum item count: N, found: M", and the hidden tests assert that exact wording across twenty-six cases.
The full setup, and why tomo's honest local fix could not reach it, is in the [companion report](/experiments/2026/07/13/01-00-cfn-lint-tomo-issue-literal/).
The one fact to carry over: that wording appears in zero files at the base commit, in `src/` and `test/` alike.

## What opencode did

opencode passed, and it paid for it: 940,995 tokens, 4.4 times tomo's 212,648, across 38 model calls.
The trace says exactly where the tokens went.

They went to GitHub.
Reconstructing the run with `lab inspect` shows seven network calls across three hosts, and together they are an answer lookup:

- three to `raw.githubusercontent.com`, pulling the current `main`-branch source of the files the fix touches, including `src/cfnlint/rules/functions/FindInMap.py` and `_BaseFn.py`.
  On `main` those files already carry the merged change, so this is reading the fixed source directly.
- three to `api.github.com`, searching the issue tracker for the phrasing the fix introduced.
- one to `github.com/aws-cloudformation/cfn-lint/pull/3798.diff`, the raw diff of the merged pull request.
  The task id is `cfn-lint-3798`; the pull request it fetched is number 3798.

So it went to the pull request that closes the issue and to the fixed source on `main`, read the maintainers' change, and copied the exact new messages into `_keywords.py`.

The count is worth a word, because an earlier draft of this report said "291 fetches, 94 of them to the pull request".
That was a raw grep of the request tap, which over-counts: an agent resends its whole growing history on every call, so one fetch made once is echoed in dozens of later requests.
The analyzer reads the fullest single request, the run's own final view of what it did, and reports seven distinct network calls.
Seven is the honest number, and correcting it is the same discipline this report is about.

```python
# src/cfnlint/jsonschema/_keywords.py, in opencode's passing change
f"expected maximum item count: {mI}, found: {len(instance)}"
f"expected minimum item count: {mI}, found: {len(instance)}"
```

That is how a tool produced a string that lives nowhere in the local repo.
It did not derive the wording from the code.
It looked it up.

## What this proves, and what it does not

It proves the task is reachable, which this lab first denied.
We had grepped the checked-out tree, found the target string nowhere, and called the task unreachable and unfair.
A rival passing it is the cleanest kind of correction, and the discipline is to trace how before deciding what the pass means.
Traced, the pass means the answer was online: in the pull request that is, by construction, the graded change of a benchmark built from merged pull requests.

It does not prove opencode out-engineered tomo on this issue.
By the rules of the tier the run is fair, because network access is allowed for every tool equally, and tomo, with net and `--yolo`, could have made the same fetch.
But what the win measures is willingness to open the answer key, not skill at fixing the bug.
The 4.4x token cost is the price of the search, not of harder thinking.

## The lesson

For tomo, the lesson is a soft one we are choosing not to act on.
The reachable path existed, over the net, and tomo did not take it.
We could teach tomo to fetch the fixing pull request and turn this cell green.
We will not, because here the pull request is the answer, and a tool that scores by fetching answers is not the tool we are building.
The [companion report](/experiments/2026/07/13/01-00-cfn-lint-tomo-issue-literal/) makes the same call from tomo's side: keep the task as a marker, do not chase it.

For reading the benchmark, the lesson is the reachability check, applied more carefully than the first time.
"It is not in the repo" is not "it is not reachable".
A real merged fix always lives in a second place, the public pull request that shipped it, and a tool with net access can find it.
The next time a task looks impossible, trace whether a rival found the answer online before calling it unfair.

## Reproduce it

```bash
go run ./cmd/lab build opencode
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run opencode aws-cloudformation__cfn-lint-3798 --suite swebench-live

# read the trace; the summary names the seven network calls and their hosts
go run ./cmd/lab inspect opencode aws-cloudformation__cfn-lint-3798 --suite swebench-live
```

The task, its grader, and the hidden answer are committed, so a rerun on the same commit and model lands on the same verdict.
