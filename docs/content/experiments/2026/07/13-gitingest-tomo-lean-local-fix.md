---
title: "gitingest: tomo fixes it the honest local way"
linkTitle: "gitingest tomo"
description: "tomo solves a real gitingest issue the way the benchmark intends: it reads the source, finds the one branch that only handles https, adds the http case, and verifies with the project's own tests. One source edit, no network, 242k tokens. A clean local pass, with the small waste the analyzer still catches."
date: 2026-07-13T01:26:00+07:00
weight: 994
---

This is a single run: tomo, on `cyclotruc__gitingest-94`, a real GitHub issue from the [swebench-live](/evals/swebench-live/) tier.
It passed.
It is the run to read next to the [cfn-lint pair](/experiments/2026/07/13-cfn-lint-opencode-answer-lookup/), because it is the opposite kind of pass: where opencode passed cfn-lint by fetching the answer online, tomo passes this one by reading the code in front of it and reasoning to the fix.
When the answer is derivable from the source, tomo's honest-local habit is the whole job, and here it is.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13 01:26 (GMT+7) |
| Tool | tomo, `--yolo`, pinned image on the swebench-live suite |
| Model | `north-mini-code-free` on the OpenCode Zen free tier |
| Harness | tomo-labs, run at `LAB_CONCURRENCY=1` |
| Task | `cyclotruc__gitingest-94`, the gitingest repo at base commit `2125765`, graded in a Python 3.12 venv on the host |
| Verdict | PASS, "fail_to_pass green, in-file pass_to_pass stable". 242,737 tokens (235,712 prompt, 7,025 completion), 27 model calls, 70s, 35.8 MB peak memory |

```bash
export LAB_MODEL=north-mini-code-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo cyclotruc__gitingest-94 --suite swebench-live --yolo
```

A note on the model.
The `hy3-free` bucket that graded the cfn-lint runs earlier in the day was exhausted by this point, so this run is on `north-mini-code-free`, the free model still answering.
That is why this report does not put its token count next to the cfn-lint runs: a different model is a different scale, and comparing across them would be the mistake this lab tries not to make.

## The task, in one line

gitingest fails on any repo URL that starts with `http://` instead of `https://`.
The issue points straight at the cause: `_parse_url` in `parse_query.py` checks for `"https://"` explicitly and never handles the plain-`http` case.
The task is to make the smallest source change that accepts `http://`, without editing the tests, which are hidden and do the grading.

## What tomo did

The [lab inspect](/guides/) walkthrough reads as a clean three-phase run: find the file, make the one edit, check it.

It investigated first.
It listed the tree, found `parse_query.py`, read it, then read `test_parse_query.py` and the `process_query.py` that calls into it, so it understood both the function and how it is used before touching anything.
Ten moves in, it had localized the bug to the URL-scheme branch the issue named.

Then it made one edit, to one file:

```python
# src/gitingest/parse_query.py, in tomo's passing change
if url.startswith("http://"):
    url = "https://" + url[7:]  # Replace http:// with https://
elif not url.startswith("https://"):
    url = "https://" + url
```

That is the smallest fix that answers the issue: a plain-`http` URL is rewritten to `https`, the existing scheme-less case is left exactly as it was, and nothing else changes.
No new dependency, no touched test, no rewrite of the surrounding function.

Then it verified.
It ran the project's own `pytest` on `test_parse_query.py`, wrote a throwaway `python -c` to poke `_parse_url` with an `http://` URL directly, and ran the wider test file to confirm it had not broken the scheme-less path.
It finished having checked its own work, which the summary records as "checked its own work before finishing".

## The waste worth naming

A pass is not a clean sheet, and the analyzer says so.
tomo hit six tool errors along the way, re-ran one shell command whose output it already had, and repeated two calls verbatim.
Most of the errors are self-inflicted friction rather than real trouble: a `read` aimed at a directory instead of a file, a `grep` that exited non-zero on no matches and got re-run with a `2>/dev/null || echo` guard, a first `python -c` that tripped on its own quoting before the second one worked.

None of it changed the verdict, and on 7,025 completion tokens the run was cheap where it counts.
But it is the kind of loop tomo is meant to tighten: the directory-read and the repeated grep are both avoidable with a first look before the call, and they are exactly what the analyzer now surfaces so a passing-but-loose run does not read as flawless.

## The lesson

For tomo, this is the good case stated plainly.
The fix lived in the code, tomo read the code, and tomo found the fix, then proved it with the repository's own tests.
No network, 35.8 MB of memory, one edit.
That is the shape of pass this lab is trying to make ordinary: the honest local one.

Set beside the [opencode cfn-lint run](/experiments/2026/07/13-cfn-lint-opencode-answer-lookup/), the contrast is the whole point.
That task's answer was not in the repo, so passing it meant leaving the repo to fetch the merged fix.
This task's answer was in the repo, so passing it meant reading carefully and reasoning, which is the skill worth growing.
tomo's job is to be excellent at the second kind, and the tightening left to do is the loop friction above, not the fix itself.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_MODEL=north-mini-code-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo cyclotruc__gitingest-94 --suite swebench-live --yolo
go run ./cmd/lab inspect tomo cyclotruc__gitingest-94 --suite swebench-live
```

The task, its grader, and the base commit are committed, so a rerun on the same commit and model lands on the same verdict, free-tier rate limits on the day permitting.
