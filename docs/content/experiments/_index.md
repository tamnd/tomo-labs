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

<!-- Sidebar order convention: weight is scoped per month folder (2026/07/, 2026/08/, ...), since the theme sorts each folder's pages independently. Within a folder, count weights DOWN from 1000 (oldest 1000, each newer report one less) so the ascending weight sort lists newest first. A new report takes the next value below that folder's current smallest; a new month starts fresh at 1000. -->

### 2026

- **2026-07-13 08:44 (GMT+7)** - [python-control: tomo debugs itself in circles writing scratch scripts](/experiments/2026/07/13-python-control-tomo-scratch-file-runaway/).
  tomo makes 34 edits and still fails, because 33 of them are throwaway debug scripts it wrote to watch the bug rather than fix it: one real edit to source, never tested, then the fifteen-minute wall.
- **2026-07-13 08:19 (GMT+7)** - [fonttools: tomo writes more than the fix and fails on one normalization](/experiments/2026/07/13-fonttools-tomo-overnormalized-cff/).
  The one failure where tomo did everything right: found the exact file, wrote a fuller CFF fix than the maintainers, verified it, then failed a single hidden test by reusing a variable that forces `.notdef` to the front. A correctness gap, not a discipline one.
- **2026-07-13 08:04 (GMT+7)** - [dynaconf: tomo runs away digging through git history](/experiments/2026/07/13-dynaconf-tomo-git-archaeology-runaway/).
  tomo burns 132 requests and four million tokens mining `git log`, `git diff`, and `git show` for a fix that history does not hold, hits the wall, and never edits a file: a git-archaeology trap and a missing stop-and-commit discipline.
- **2026-07-13 01:26 (GMT+7)** - [gitingest: tomo fixes it the honest local way](/experiments/2026/07/13-gitingest-tomo-lean-local-fix/).
  tomo reads the source, finds the URL branch that only handles https, adds the http case, and verifies with the project's own tests: one source edit, no network, the honest-local pass.
- **2026-07-13 01:21 (GMT+7)** - [cfn-lint: pi fails it the honest way too](/experiments/2026/07/13-cfn-lint-pi-honest-local-fail/).
  A second rival, cut short by the free-tier rate limit, never fetches the pull request and never rewrites the source wording, failing the same structural way tomo did.
- **2026-07-13 01:11 (GMT+7)** - [cfn-lint: opencode passes by fetching the answer PR](/experiments/2026/07/13-cfn-lint-opencode-answer-lookup/).
  opencode passes a task whose graded wording is nowhere in the repo, by fetching the fixed source on the project's main branch and the merged pull request's diff and copying their exact new messages, which proves the task is reachable only by looking up the answer online.
- **2026-07-13 01:00 (GMT+7)** - [cfn-lint: tomo fixes the issue, the grade wants something else](/experiments/2026/07/13-cfn-lint-tomo-issue-literal/).
  tomo implements exactly the message the issue asks for and fails, because the grade wants a generic validator wording the maintainers changed instead, which appears nowhere tomo could see.
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
