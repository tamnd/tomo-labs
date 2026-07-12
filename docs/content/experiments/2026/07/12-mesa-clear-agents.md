---
title: "mesa: the right name, in the wrong place"
linkTitle: "mesa clear_agents"
description: "tomo fixes a real mesa issue correctly, then fails the grade on a method name. The twist: its own throwaway test used the name that would have passed. A close read of one run, and why the task is fair even though every tool failed it."
date: 2026-07-12T23:49:00+07:00
weight: 10
---

This is a single run: tomo, on one real GitHub issue from the [swebench-live](/evals/swebench-live/) tier.
It failed.
The reason it failed is worth the read, because tomo had the correct answer in hand and used it in the wrong place, and because it is tempting to call the task unfair when it is not.

## Reproducibility

Everything you need to run this exact experiment again.

| | |
|---|---|
| Run captured | 2026-07-12 23:49 (GMT+7) |
| Tool | tomo, commit `ca72cdb15de8` (pseudo-version `v0.2.5-0.20260712142915`), one commit behind main |
| Model | `hy3-free`, which resolves to `tencent/hy3-20260706:free` on the OpenCode Zen free tier |
| Harness | tomo-labs commit `a5b4899`, run at `LAB_CONCURRENCY=1` |
| Task | `projectmesa__mesa-2394`, the mesa repo at commit `2dedca4a`, graded in a Python 3.12 venv on the host |
| Verdict | FAIL, "hidden tests not satisfied". 93,059 tokens, 16 model calls, 79.9 MB peak memory, 125 s |

```bash
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo projectmesa__mesa-2394 --suite swebench-live
```

A note on the version, since it is easy to get wrong.
The tool image's metadata label reads `v0.2.4`, the latest tag, but the binary that actually ran is a later main commit.
`tomo version` inside the image reports the pseudo-version above.
This is why the header pins the commit, not the tag: the tag was stale and the commit is the truth.

## The task

Every task in this tier is a real GitHub issue, handed to the tool with the project checked out at the commit just before the fix.
The tool has to change the source so a hidden test suite, which it never sees, goes green.

The mesa issue asks for a convenience method to remove every agent from a model at once.
The person who filed it suggests a name, in passing:

> It seems we need a model level method like `model.clear_agents()`, which would call `agent.remove` on each agent.

Note the word "like".
It reads as a suggestion, not a specification.
That detail matters later.

The hidden test that grades the fix does not use that name.
It calls a different one:

```python
def test_agent_remove():
    model.remove_all_agents()
```

And the fix the mesa maintainers actually merged adds `remove_all_agents` to the model, not the `clear_agents` the issue proposed.
So the name in the issue and the name that passes the grade are different.
This is the whole task.

## What tomo did

tomo got the hard part completely right.
It added a method to `mesa/model.py` that walks a copy of the agent set and calls `agent.remove()` on each one, with a docstring explaining that going through `remove` also clears the agents from mesa's experimental cell spaces.
That is exactly the behaviour the issue describes, and it matches the maintainer's own fix in every way that counts.

It named the method `clear_agents`.
The name from the issue.

Then it did something revealing.
It wrote itself a quick test to check its work, and that test called `model.remove_all_agents()`.
The real name.
The name that would have passed.

So within one run tomo produced both names.
`clear_agents` in the code it shipped.
`remove_all_agents` in the test it wrote to check that code.
The two disagreed.
Grading calls `remove_all_agents`, the shipped code only defines `clear_agents`, so the call fails with `AttributeError` and the test is red.

tomo was one name away from passing, and it had already typed that name itself.

## Is this a fair task?

The first reaction, and the one this write-up started with, is that the task is unfair.
The graded name never appears in the issue.
An agent that reads only the issue is pointed at `clear_agents` and walks straight into the wall.
So drop it, the thinking goes, and pick a cleaner task.

On a closer look that is the wrong call, for three reasons.

**The answer was reachable, and tomo reached it.**
A task is only unfair if the correct answer cannot be gotten from what the tool is given.
Here it plainly could: tomo wrote `remove_all_agents` in its own test, so the name was available to it, from its knowledge of the real mesa library.
The failure is not a missing answer, it is an answer the tool held and then contradicted.

**This is a normal kind of task, not a broken one.**
In a benchmark built from real merged fixes, the correct answer is whatever the maintainer shipped, and it is common for that to differ from an issue's off-hand phrasing.
The issue even hedged with "like".
Resolving that gap, trusting the library's real API over a casual suggestion, is part of the work, not a trick.

**It tells the tools apart, which is the point of a test.**
A weak agent takes the issue literally and fails.
A stronger one runs the check it wrote, or notices that it named the same method two different ways, and passes.
tomo sat right on that line.
A task that a better tool would pass and a weaker one would fail is a good task.
Dropping it would throw away a real, reproducible signal about how tomo works.

So the task stays.
The problem it exposes is tomo's, and that is exactly the kind of problem this lab exists to find.

## The lesson for tomo

The fix is not to make tomo smarter about mesa.
tomo already knew the right name.
The fix is to make tomo trust and use its own work.

Two concrete gaps, from this one run:

- **Run the check you wrote.**
  tomo wrote a test and then declared the task done without running it.
  That test called `remove_all_agents` against code that did not define it, so running it once would have raised the error and pointed straight at the fix.
  A change is not done until the check written for it has been run.
- **Keep a name consistent.**
  The same method appeared as `clear_agents` in the source and `remove_all_agents` in the test.
  A tool that notices it has named one thing two ways, and reconciles them, closes this class of bug on its own.

Neither of these needs a bigger model or more tokens.
tomo did this run in 93,059 tokens, lean by the standard of this tier.
It did not lose for want of effort.
It lost a step of self-checking it had every piece to perform.

## Reproduce it

```bash
# build the tomo image at the pinned commit, then run the one task
go run ./cmd/lab build tomo
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo projectmesa__mesa-2394 --suite swebench-live

# read the trace of what the tool did, turn by turn
go run ./cmd/lab inspect tomo projectmesa__mesa-2394 --suite swebench-live
```

The task, its grader, and the hidden answer are all committed, so a rerun on the same tool commit and the same model lands on the same verdict.
The one thing that moves it is a change to tomo.
