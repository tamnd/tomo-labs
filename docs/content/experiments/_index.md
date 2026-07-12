---
title: "Experiments"
linkTitle: "Experiments"
description: "Write-ups of single runs worth reading in full: one tool on one task, what it did turn by turn, why it passed or failed, and what the run says about the tool or the task. Each report pins the exact tool version, model, and commit so you can reproduce it."
weight: 19
featured: true
---

The [evals](/evals/) pages give you the aggregate: a table of every tool over a whole benchmark.
This section is the opposite zoom level.
Each report here is one run, read closely: one tool, on one task, with the whole story of what it did and why it landed where it did.

These are the runs worth stopping on.
A tool that fails a task in a surprising way, a task that turns out to be harder or more ambiguous than it looked, a result that overturns a first guess once you read the trace.
The point is not the score, it is the reason behind the score.

## Organised by date

Reports are grouped by year, and named by the date the run was captured, so the section reads as a timeline.
That ordering is the point: put July next to December and you can see how tomo changed across the runs, not just where it stands today.
A report is never edited to match a later build.
It is a dated record of one build on one day, and a newer report supersedes it rather than overwriting it.

The tree nests year, then month, then day, so a report lives at a path like `/experiments/2026/07/12-…`.
Each item below is one experiment, linked straight to its report, newest first.

### 2026

- **2026-07-13 00:50 (GMT+7)** - [faker: the fix that let tomo apply its own answer](/experiments/2026/07/13-faker-yolo-autonomous-fix/).
  A new `--yolo` mode runs tomo fully autonomous, the way every rival already runs, and the task tomo had solved but could not write now passes, on 40 percent fewer tokens.
- **2026-07-13 00:14 (GMT+7)** - [faker: solved, then locked out by a web fetch](/experiments/2026/07/13-faker-iban-untrusted-lock/).
  tomo writes the exactly correct Belgian IBAN fix but cannot apply it, because a page it fetched tripped its own prompt-injection guard and every later edit was declined headless.
- **2026-07-12 23:49 (GMT+7)** - [mesa: the right name, in the wrong place](/experiments/2026/07/12-mesa-clear-agents/).
  tomo fixes the mesa `remove all agents` issue correctly but fails the grade on a method name it had already used in its own throwaway test.

## How to read a report

Every report opens with a reproducibility header: the tool and its exact version or commit, the model, the harness commit, and the task, all pinned.
A benchmark number means nothing if you cannot say which build produced it, so each report says precisely which build produced it, and gives you the one command to run it again.

After that the report is plain prose.
What the task asked for, what the tool did step by step, where it went right or wrong, and the lesson.
You do not need to have read the source of the harness to follow one.
Where a report leans on a harness detail, like how grading works or how a task is validated, it explains that detail in place.

## Why keep failures

A tool that fails a task the harness can grade is one of the most useful things this lab produces.
It is a concrete, reproducible gap: here is a run, here is where it went wrong, here is the smallest change that would have fixed it.
Some of the reports below are failures kept on purpose, because the reason a run failed is often more instructive than a wall of green.

One caution the reports take seriously: a failing run is not automatically a bad task.
Before blaming the task, the report checks whether the answer was actually reachable from what the tool was given.
Sometimes it was, and the failure is real.
That distinction is the whole discipline of reading a run honestly, and the reports try to model it.
