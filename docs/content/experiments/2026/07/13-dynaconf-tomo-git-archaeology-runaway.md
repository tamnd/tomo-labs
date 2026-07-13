---
title: "dynaconf: tomo runs away digging through git history"
linkTitle: "dynaconf tomo runaway"
description: "tomo's worst run of the sweep: on a dynaconf bug it spent 132 requests and four million tokens running git log, git diff, and git show to reverse-engineer a fix from history, hit the fifteen-minute wall, and never edited a single file. A clean look at the git-archaeology trap and the missing stop-and-commit discipline the analyzer makes visible."
date: 2026-07-13T08:04:08+07:00
weight: 993
---

This is a single run: tomo, on `dynaconf__dynaconf-1225`, a real GitHub issue from the [swebench-live](/evals/swebench-live/) tier.
It failed, and it failed in the most expensive way a run can: it hit the fifteen-minute wall having burned four million tokens without ever writing a line of the fix.
It is the run to read next to the [gitingest pass](/experiments/2026/07/13-gitingest-tomo-lean-local-fix/), because it is the same tool on the same model doing the opposite of what a good run does.
This lab exists to surface exactly this, so the report is written plainly and the fix it points to is a real one, in tomo itself.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13 08:04 (GMT+7) |
| Tool | tomo, `--yolo`, pinned image on the swebench-live suite |
| Model | `north-mini-code-free` on the OpenCode Zen free tier |
| Harness | tomo-labs, run at `LAB_CONCURRENCY=1`, per-attempt wall ceiling 900s |
| Task | `dynaconf__dynaconf-1225`, the dynaconf repo at base commit `39acdee`, graded in a Python 3.12 venv on the host |
| Verdict | FAIL, killed at the 900s wall (exit 124). 4,007,703 tokens (3,940,720 prompt, 66,983 completion), 132 model calls |

```bash
export LAB_MODEL=north-mini-code-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo dynaconf__dynaconf-1225 --suite swebench-live --yolo
```

## The task, in one line

A dynaconf settings-loader bug: five `tests/test_settings_loader.py` cases fail at the base commit and must go green, and the fix lives in the loader source.
The issue references the upstream report by its number, "#1204".
As with every task in this tier, the repo is checked out at the buggy commit and the hidden test suite does the grading, so the job is to read the loader, understand the loading logic, and make the smallest source change that satisfies the tests.

## What tomo did

Nothing that changed a file.
The [lab inspect](/guides/) summary is stark:

```
tomo did not solve dynaconf__dynaconf-1225 in 132 requests and 4,007,703 tokens.
It read 6 files, searched 20 times, and ran 104 shell commands.
6 calls repeated an earlier call verbatim, a sign of spinning.
It hit 15 tool errors along the way.
```

Six reads across 132 requests. One hundred and four shell commands. Zero edits.
Break the shell commands down and the run's whole character falls out: 88 of the 104 were git.

| git command | count |
|---|---|
| `git log` | 39 |
| `git diff` | 32 |
| `git show` | 10 |
| `git status` | 4 |
| other (`remote`, `merge-base`, `branch`) | 3 |

The walkthrough shows what it was chasing. From the fifth move on it was grepping history for the issue number and diffing commit ranges, hoping the fix was somewhere in the log:

```
5. ran git log --oneline | grep -i "1204" | head -20
6. ran git log --all --oneline | grep -i "1204" | head -20
7. ran git show --stat 188ca5e
14. ran git diff 39acdee..188ca5e --name-only
17. ran git diff 39acdee..188ca5e -- dynaconf/cli.py | head -200
20. ran git log --oneline --graph --all --grep="1204" | head -20
```

It kept this up for the full fifteen minutes.

## Why it never found anything

The search was doomed at move five, and this is the heart of the lesson.
tomo treated "#1204" as a commit to locate in history.
But in SWE-bench-Live the repository is checked out *at* the buggy commit: the fix is the future, it is the answer key, and it is not in the log.
There is no commit to `git show`, no range to `git diff`, no grep of `git log` that returns it, because it has not been written yet in this tree.
Every one of those 88 git commands was looking for something that could not be there.

The tell in the analyzer is that only six of the 132 calls were verbatim repeats.
tomo's loop governor bounds identical-call spinning, and it did its job: the run was not stuck re-issuing one command.
It was issuing 88 *different* git commands, each just distinct enough to look like progress, none of it progress at all.
That is a second, subtler failure mode the verbatim-repeat guard is blind to by design.

## The two levers, both in tomo

This is a genuine capability gap, unlike a task whose answer only lives online.
The fix for dynaconf was sitting in the loader source the whole time; tomo simply never went to read it.
Two changes in the agent would have turned this run around, and both belong in [tomo](https://github.com/tamnd/tomo), not the harness:

1. **Stop mining git for the answer.** A referenced issue or PR number is context, not a search target. When a bug task is checked out at the buggy commit, there is no fixing commit in history, so `git log`, `git diff`, and `git show` against it can only waste budget. tomo should read the code the issue points at, not excavate the log for a patch that is not there.
2. **Commit to an edit.** One hundred and thirty-two requests with zero edits should trip a convergence bias long before the wall does. After a stretch of investigation with nothing written, tomo should be pushed to make its best fix and verify it, rather than keep exploring. The governor bounds verbatim repeats; it needs a second bound on investigation that is not producing an edit.

There is a matching failure at the other extreme in the same sweep: on `kubernetes-client__python-2303` tomo gave up after six requests and thirteen thousand tokens.
So its investigation depth is uncalibrated in both directions, running away on one task and quitting early on another.
A budget-aware convergence policy is the single change that helps both ends.

## The lesson

No well-behaved agent burns four million tokens with nothing written to show for it.
This is not a hard task or an unfair one, and it is not the answer-key problem some tasks have: the fix was local and readable.
tomo lost it to a habit, going to git history for an answer that history does not hold, with no discipline to pull itself back to the actual work of editing and checking.
That is the honest read, it points at tomo rather than the benchmark, and it is exactly the kind of lever this lab is built to find.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_MODEL=north-mini-code-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo dynaconf__dynaconf-1225 --suite swebench-live --yolo
go run ./cmd/lab inspect tomo dynaconf__dynaconf-1225 --suite swebench-live
```

The task, its grader, and the base commit are committed, so a rerun on the same commit and model lands on the same verdict, free-tier rate limits on the day permitting.
