---
title: "python-control: tomo debugs itself in circles writing scratch scripts"
linkTitle: "python-control tomo scratch runaway"
description: "A second runaway with a different shape. On a python-control conversion bug, tomo made 34 edits and still failed, because 33 of them were throwaway debug scripts it wrote to instrument the problem rather than fix it. It made exactly one edit to real source, never ran the project's tests on it, and hit the wall. The analyzer separates the one real edit from the 33 dead ends and names the habit."
date: 2026-07-13T08:44:00+07:00
weight: 992
---

This is a single run: tomo, on `python-control__python-control-1064`, a real GitHub issue from the [swebench-live](/evals/swebench-live/) tier.
It failed at the fifteen-minute wall, and it failed the same expensive way the [dynaconf runaway](/experiments/2026/07/13-dynaconf-tomo-git-archaeology-runaway/) did, but the surface behaviour is different enough to be worth its own report.
dynaconf burned its budget mining git history and never wrote anything.
This run wrote a great deal, thirty-four edits in all, and still never wrote the fix.
The difference is where the writing went, and that is the whole lesson.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13 08:44 (GMT+7) |
| Tool | tomo, `--yolo`, pinned image on the swebench-live suite |
| Model | `north-mini-code-free` on the OpenCode Zen free tier |
| Harness | tomo-labs, run at `LAB_CONCURRENCY=1`, per-attempt wall ceiling 900s |
| Task | `python-control__python-control-1064`, the python-control repo at its base commit, graded in a Python 3.12 venv on the host |
| Verdict | FAIL, killed at the 900s wall. 2,374,845 tokens, 121 model calls |

```bash
export LAB_MODEL=north-mini-code-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo python-control__python-control-1064 --suite swebench-live --yolo
```

## The task, in one line

A python-control conversion bug: a system built with `zpk` and the same system built with `tf` should behave identically, and a transfer-function to state-space step somewhere in `control/xferfcn.py` and `control/statesp.py` diverges.
The hidden test suite grades the fix.
As with every task in this tier the repo is checked out at the buggy commit, so the job is to read the conversion code, find where the two paths part, and make the smallest source change that lines them up.

## What tomo did

It edited thirty-four files and failed.
The [lab inspect](/guides/) summary is the report in miniature:

```
tomo did not solve python-control__python-control-1064 in 121 requests and 2,374,845 tokens.
It read 26 files, searched 16 times, made 34 edits to /work/reproduce_issue.py,
/work/debug_issue.py, /work/simple_debug.py, /work/reproduce_original.py,
/work/examine_difference.py, ... /work/control/xferfcn.py, ... and ran 43 shell commands.
It edited 6 test files, which the grader resets before grading, so that change does not count.
8 calls repeated an earlier call verbatim, a sign of spinning.
It finished without running a test or a syntax check on the edit.
```

Read the edit list and the run's whole character falls out.
Of the thirty-four edits, thirty-three went to files tomo created for itself: `reproduce_issue.py`, `debug_issue.py`, `simple_debug.py`, `reproduce_original.py`, `examine_difference.py`, `reproduce_and_explore.py`, `debug_zpk.py`, `compare_responses.py`, `trace_zpk.py`, `trace_tf_creation.py`, `inspect_state_space.py`, `deep_analysis.py`, `trace_execution.py`, `final_verification.py`, on and on, twenty-five throwaway instrumentation scripts.
Exactly one edit landed in real source, `control/xferfcn.py`.
Six more edits went to test files, which the grader resets before it grades, so they could never have counted even if they had been right.

## The loop it got stuck in

The walkthrough shows the shape plainly.
tomo would read a slice of the conversion code, write a script to poke at it, run the script, get a warning or a traceback, and then write another script to poke a little differently:

```
 9. edited /work/reproduce_issue.py       -> ran it, got a scipy overflow warning
11. edited /work/debug_issue.py           -> ran it, same warning
14. edited /work/simple_debug.py          -> ran it, printed a debug banner
19. edited /work/reproduce_original.py
20. edited /work/examine_difference.py    -> ran it, got a traceback
27. edited /work/reproduce_and_explore.py -> ran it, got a banner
31. edited /work/debug_zpk.py
34. edited /work/compare_responses.py     -> ran it, same overflow warning
36. edited /work/trace_zpk.py
```

Each script was a fresh attempt to see the bug rather than a step toward fixing it.
The investigation never converged into a change to the conversion code, it just kept spawning new observation posts.
tomo spent two and a third million tokens watching the bug from thirty-three angles and patched it from none.

## Why it is the same failure as dynaconf, underneath

On the surface this is the opposite of the dynaconf runaway.
There, tomo wrote nothing and read history for fifteen minutes.
Here, tomo wrote constantly.
But the root is identical: no discipline that says stop, decide, edit the real file, and run the project's own tests.

The tell is the analyzer's last line, that the run finished without ever running a test or a syntax check on the edit.
tomo made its one real change to `control/xferfcn.py` and then went back to writing trace scripts, and it never once ran the suite that would have told it whether the change helped.
It also never noticed that six of its edits were to test files the grader throws away.
A single real edit, unverified, buried under thirty-three scratch files, is what a run looks like when nothing pulls it back to the actual work.

## The levers, both in tomo

This is a genuine capability gap, and the fixes belong in [tomo](https://github.com/tamnd/tomo), not the harness.

1. **Fix the source, do not instrument it to death.** A couple of reproduction scripts is reasonable.
   Twenty-five is a tell that the agent is substituting observation for action.
   After a small budget of scratch scripts without a source edit, tomo should be pushed to read the real code and change it, the same way the dynaconf lesson says stop mining git and go read the loader.
2. **Verify against the project's own tests before finishing.** The one real edit was never run against the suite.
   A run should not be allowed to end on an untested source change while budget remains, and edits to files the grader resets, the tests, should be flagged as not counting toward a fix.

Both are the same underlying policy the dynaconf run pointed at: a budget-aware convergence bias that forces the agent from investigation into a decisive, verified edit.
Across the sweep tomo has now hit this in three distinct disguises, git-archaeology on dynaconf, scratch-file proliferation here, and thrash-while-editing on instructlab, and once at the other extreme, quitting after six requests on a kubernetes task.
One policy, budget-aware in both directions, addresses all four.

## The lesson

Writing a lot is not the same as fixing anything.
tomo produced thirty-four edits and zero progress, because the edits were instrumentation, not repair, and nothing in the agent noticed the difference or made it commit.
The bug was local and readable, the same as dynaconf, so this points at tomo rather than the benchmark.
It is the clearest single argument in the sweep for a convergence policy: the failure is not too little effort, it is effort with no discipline pointing it at the source.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_MODEL=north-mini-code-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo python-control__python-control-1064 --suite swebench-live --yolo
go run ./cmd/lab inspect tomo python-control__python-control-1064 --suite swebench-live
```

The task, its grader, and the base commit are committed, so a rerun on the same commit and model lands on the same verdict, free-tier rate limits on the day permitting.
