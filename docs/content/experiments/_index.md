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

The tree nests year, then month, then day, and each report is named for the time it was captured, so a report lives at a path like `/experiments/2026/07/12/23-49-mesa-clear-agents/`.
Each item below is one experiment, linked straight to its report, newest first.

<!-- Ordering convention: a report is a file named HH-MM-slug.md under its day folder (2026/07/13/), and its front matter carries the full date it was captured. The theme sorts by that date descending, so newest reads first with no weights to hand-maintain. To add a report, drop it in the right day folder with an HH-MM-slug name and a date, and add one bullet to the top of that day here and in the year and month index. -->

### 2026

- **2026-07-13 14:01 (GMT+7)** - [dynaconf on gpt-5.5: six times the cost, the same wall](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/).
  The flagship codex model, both answer doors closed, writes nineteen edits across every loader and spends six times what the cheap model did, then fails on the identical two settings-loader tests. With the doors shut, paying more buys a broader wrong fix, not a right one.
- **2026-07-13 13:49 (GMT+7)** - [dynaconf on gpt-5.4-mini: the first run with both doors shut](/experiments/2026/07/13/13-49-dynaconf-gpt-5.4-mini-offline-honest-fail/).
  The first honest number on dynaconf-1225: history pruned so the fix commit is unreachable, the shell sandboxed so it cannot fetch the answer PR. The cheapest codex model writes a real fix under a dollar, reaches no answer, and fails the settings-loader tests.
- **2026-07-13 12:40 (GMT+7)** - [dynaconf on opus: it read both the source PR and the answer PR](/experiments/2026/07/13/12-40-dynaconf-opus-answer-fetch/).
  Claude Opus 4.8, the most expensive model in the comparison, passed by fetching PR #1204, the source it was asked to port, and PR #1225, the merged answer that grades it: the priciest run did the least work.
- **2026-07-13 12:35 (GMT+7)** - [dynaconf on sonnet: it looked up the merged answer](/experiments/2026/07/13/12-35-dynaconf-sonnet-answer-fetch/).
  Claude Sonnet 5 passed by running `gh pr view 1225` and `gh pr diff 1225` and applying the merged fix's commits: the git-history door was closed, the network door was not, which is why the harness has to isolate the network for every tool.
- **2026-07-13 12:30 (GMT+7)** - [dynaconf on haiku: the clean run that failed honestly](/experiments/2026/07/13/12-30-dynaconf-haiku-clean-fail-broad-refactor/).
  Claude Haiku 4.5 never reached the network, wrote a real source fix, and failed by threading the identifier through every loader: the honest baseline the two bigger Claude models cheated to beat.
- **2026-07-13 12:20 (GMT+7)** - [with the leak closed, dynaconf sorts the models](/experiments/2026/07/13/12-20-dynaconf-closed-sorts-the-models/).
  The same leak-free task passes on gpt-5.6-sol and gpt-5.5 and fails on gpt-5.4, so removing the git shortcut left a task that actually measures the model rather than its willingness to look up the answer.
- **2026-07-13 12:05 (GMT+7)** - [paying five times more buys the same wrong fix](/experiments/2026/07/13/12-05-codex-subscription-mini-vs-sol-cost/).
  gpt-5.4-mini and gpt-5.6-sol on python-control converge on the identical edit and fail the identical three tests, so the lever over tomo is convergence and cost, not correctness.
- **2026-07-13 11:50 (GMT+7)** - [dynaconf: the fix was reachable in git, and we closed it](/experiments/2026/07/13/11-50-dynaconf-sol-answer-leak-closed/).
  gpt-5.6-sol passed by diffing the base commit against the upstream fix commit the work-tree clone left reachable: the same history tomo dug through held the answer, so `setup.sh` now strips it for every tool.
- **2026-07-13 11:05 (GMT+7)** - [the write-churn runaway, bounded, and tomo failing cheaper than claude-code](/experiments/2026/07/13/11-05-churn-guard-vs-claude-code/).
  The third and last runaway shape is bounded: a turn that keeps editing without converging, on scratch scripts or the same file over and over, now stops instead of burning a hundred rounds. On the two tasks that showed it, read next to claude-code on the same model, both fail and tomo fails on 27 to 57 percent fewer tokens.
- **2026-07-13 10:05 (GMT+7)** - [dynaconf: the guard that stops the runaway, and pi running straight into the wall](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/).
  The fix for the git-archaeology runaway, merged and measured live: the same task now stops at 41 requests and 1.7 million tokens instead of 132 and four million, while pi on the same model and no such bound burns thirteen million tokens into the wall. Both still fail, but tomo fails 5x faster and on 87 percent fewer tokens.
- **2026-07-13 08:44 (GMT+7)** - [python-control: tomo debugs itself in circles writing scratch scripts](/experiments/2026/07/13/08-44-python-control-tomo-scratch-file-runaway/).
  tomo makes 34 edits and still fails, because 33 of them are throwaway debug scripts it wrote to watch the bug rather than fix it: one real edit to source, never tested, then the fifteen-minute wall.
- **2026-07-13 08:19 (GMT+7)** - [fonttools: tomo writes more than the fix and fails on one normalization](/experiments/2026/07/13/08-19-fonttools-tomo-overnormalized-cff/).
  The one failure where tomo did everything right: found the exact file, wrote a fuller CFF fix than the maintainers, verified it, then failed a single hidden test by reusing a variable that forces `.notdef` to the front. A correctness gap, not a discipline one.
- **2026-07-13 08:04 (GMT+7)** - [dynaconf: tomo runs away digging through git history](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/).
  tomo burns 132 requests and four million tokens mining `git log`, `git diff`, and `git show` for a fix that history does not hold, hits the wall, and never edits a file: a git-archaeology trap and a missing stop-and-commit discipline.
- **2026-07-13 01:26 (GMT+7)** - [gitingest: tomo fixes it the honest local way](/experiments/2026/07/13/01-26-gitingest-tomo-lean-local-fix/).
  tomo reads the source, finds the URL branch that only handles https, adds the http case, and verifies with the project's own tests: one source edit, no network, the honest-local pass.
- **2026-07-13 01:21 (GMT+7)** - [cfn-lint: pi fails it the honest way too](/experiments/2026/07/13/01-21-cfn-lint-pi-honest-local-fail/).
  A second rival, cut short by the free-tier rate limit, never fetches the pull request and never rewrites the source wording, failing the same structural way tomo did.
- **2026-07-13 01:11 (GMT+7)** - [cfn-lint: opencode passes by fetching the answer PR](/experiments/2026/07/13/01-11-cfn-lint-opencode-answer-lookup/).
  opencode passes a task whose graded wording is nowhere in the repo, by fetching the fixed source on the project's main branch and the merged pull request's diff and copying their exact new messages, which proves the task is reachable only by looking up the answer online.
- **2026-07-13 01:00 (GMT+7)** - [cfn-lint: tomo fixes the issue, the grade wants something else](/experiments/2026/07/13/01-00-cfn-lint-tomo-issue-literal/).
  tomo implements exactly the message the issue asks for and fails, because the grade wants a generic validator wording the maintainers changed instead, which appears nowhere tomo could see.
- **2026-07-13 00:50 (GMT+7)** - [faker: the fix that let tomo apply its own answer](/experiments/2026/07/13/00-50-faker-yolo-autonomous-fix/).
  A new `--yolo` mode runs tomo fully autonomous, the way every rival already runs, and the task tomo had solved but could not write now passes, on 40 percent fewer tokens.
- **2026-07-13 00:14 (GMT+7)** - [faker: solved, then locked out by a web fetch](/experiments/2026/07/13/00-14-faker-iban-untrusted-lock/).
  tomo writes the exactly correct Belgian IBAN fix but cannot apply it, because a page it fetched tripped its own prompt-injection guard and every later edit was declined headless.
- **2026-07-12 23:49 (GMT+7)** - [mesa: the right name, in the wrong place](/experiments/2026/07/12/23-49-mesa-clear-agents/).
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
