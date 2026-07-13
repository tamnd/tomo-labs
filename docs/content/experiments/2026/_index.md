---
title: "2026"
linkTitle: "2026"
description: "Experiment reports from 2026, by month. Each is one tool on one task, pinned to the exact build so it reproduces."
weight: 10
---

Reports from 2026, nested by month and then by the day the run was captured, so the trail of what changed, and when, reads top to bottom.
Each entry pins the tool commit and model it ran on, so a report from July and a report from December are never quietly comparing two different builds.
Each item below is one experiment, linked straight to its report.

## July

- **2026-07-13 14:01 (GMT+7)** - [dynaconf on gpt-5.5: six times the cost, the same wall](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/).
  The flagship codex model, both doors closed, writes nineteen edits and spends six times what the cheap model did, then fails on the identical two settings-loader tests: paying more buys a broader wrong fix, not a right one.
- **2026-07-13 13:49 (GMT+7)** - [dynaconf on gpt-5.4-mini: the first run with both doors shut](/experiments/2026/07/13/13-49-dynaconf-gpt-5.4-mini-offline-honest-fail/).
  The first honest number on this task, with history pruned and the shell sandboxed: the cheapest codex model writes a real nine-edit fix under a dollar, reaches no answer, and fails the settings-loader tests.
- **2026-07-13 12:40 (GMT+7)** - [dynaconf on opus: it read both the source PR and the answer PR](/experiments/2026/07/13/12-40-dynaconf-opus-answer-fetch/).
  Claude Opus 4.8, the most expensive model in the comparison, passed by fetching PR #1204, the source it was asked to port, and PR #1225, the merged answer that grades it: the priciest run did the least work.
- **2026-07-13 12:35 (GMT+7)** - [dynaconf on sonnet: it looked up the merged answer](/experiments/2026/07/13/12-35-dynaconf-sonnet-answer-fetch/).
  Claude Sonnet 5 passed by running `gh pr view 1225` and `gh pr diff 1225` and applying the merged fix's commits: the git-history door was closed, the network door was not.
- **2026-07-13 12:30 (GMT+7)** - [dynaconf on haiku: the clean run that failed honestly](/experiments/2026/07/13/12-30-dynaconf-haiku-clean-fail-broad-refactor/).
  Claude Haiku 4.5 never reached the network, wrote a real source fix, and failed by threading the identifier through every loader: the honest baseline the two bigger Claude models cheated to beat.
- **2026-07-13 12:20 (GMT+7)** - [with the leak closed, dynaconf sorts the models](/experiments/2026/07/13/12-20-dynaconf-closed-sorts-the-models/).
  The same leak-free task passes on gpt-5.6-sol and gpt-5.5 and fails on gpt-5.4, so removing the git shortcut left a task that actually measures the model.
- **2026-07-13 12:05 (GMT+7)** - [paying five times more buys the same wrong fix](/experiments/2026/07/13/12-05-codex-subscription-mini-vs-sol-cost/).
  gpt-5.4-mini and gpt-5.6-sol on python-control converge on the identical edit and fail the identical three tests, so the lever over tomo is convergence and cost, not correctness.
- **2026-07-13 11:50 (GMT+7)** - [dynaconf: the fix was reachable in git, and we closed it](/experiments/2026/07/13/11-50-dynaconf-sol-answer-leak-closed/).
  gpt-5.6-sol passed by diffing the base commit against the upstream fix commit the work-tree clone left reachable: the same history tomo dug through held the answer, so `setup.sh` now strips it.
- **2026-07-13 11:05 (GMT+7)** - [the write-churn runaway, bounded, and tomo failing cheaper than claude-code](/experiments/2026/07/13/11-05-churn-guard-vs-claude-code/).
  The third runaway shape, a turn that keeps editing but never converges, now stops instead of burning a hundred rounds; on the two tasks that showed it tomo fails next to claude-code on 27 to 57 percent fewer tokens.
- **2026-07-13 10:05 (GMT+7)** - [dynaconf: the guard that stops the runaway, and pi running straight into the wall](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/).
  The runaway fix, merged and measured live: 41 requests and 1.7M tokens instead of 132 and four million, while pi on the same model burns thirteen million into the wall. Both fail, tomo 5x faster on 87 percent fewer tokens.
- **2026-07-13 08:44 (GMT+7)** - [python-control: tomo debugs itself in circles writing scratch scripts](/experiments/2026/07/13/08-44-python-control-tomo-scratch-file-runaway/).
  tomo makes 34 edits and still fails, because 33 of them are throwaway debug scripts it wrote to watch the bug rather than fix it: one real edit, never tested, then the wall.
- **2026-07-13 08:19 (GMT+7)** - [fonttools: tomo writes more than the fix and fails on one normalization](/experiments/2026/07/13/08-19-fonttools-tomo-overnormalized-cff/).
  tomo finds the exact file and writes a fuller CFF fix than the maintainers, then fails one hidden test by reusing a variable that forces `.notdef` to the front: the sweep's cleanest correctness gap.
- **2026-07-13 08:04 (GMT+7)** - [dynaconf: tomo runs away digging through git history](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/).
  tomo burns 132 requests and four million tokens mining `git log`, `git diff`, and `git show` for a fix that history does not hold, hits the wall, and never edits a file: a git-archaeology trap and a missing stop-and-commit discipline.
- **2026-07-13 01:26 (GMT+7)** - [gitingest: tomo fixes it the honest local way](/experiments/2026/07/13/01-26-gitingest-tomo-lean-local-fix/).
  tomo reads the source, finds the URL branch that only handles https, adds the http case, and verifies with the project's own tests: one source edit, no network, the opposite of an answer lookup.
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
