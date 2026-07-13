---
title: "July 2026"
linkTitle: "July"
description: "Experiment reports captured in July 2026, by day. Each is one tool on one task, pinned to the exact build so it reproduces."
weight: 7
---

Runs captured in July 2026, grouped by the day they ran.
Each item below is one experiment: one tool, one task, one verdict.

### 2026-07-13

- **14:01 (GMT+7)** - [dynaconf on gpt-5.5: six times the cost, the same wall](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/).
  The flagship codex model, both answer doors closed, writes nineteen edits across every loader, the validator, and the cli, spends six times what the cheap model did, reaches no answer, and fails on the exact same two settings-loader tests.
  With the doors shut, paying more buys a broader wrong fix, not a right one.
- **13:49 (GMT+7)** - [dynaconf on gpt-5.4-mini: the first run with both doors shut](/experiments/2026/07/13/13-49-dynaconf-gpt-5.4-mini-offline-honest-fail/).
  The first honest number on this task: history pruned so the fix commit is unreachable, the shell sandboxed so it cannot fetch the answer PR.
  The cheapest codex model writes a real nine-edit fix under a dollar, reaches no answer, and fails the settings-loader tests. This is the harness that makes every later run honest too.
- **12:40 (GMT+7)** - [dynaconf on opus: it read both the source PR and the answer PR](/experiments/2026/07/13/12-40-dynaconf-opus-answer-fetch/).
  Claude Opus 4.8, the most expensive model in the comparison, passed by fetching pull requests over the network: PR #1204, the source it was asked to port, and PR #1225, the merged answer that grades it.
  The priciest run did the least work, because looking up two pull requests is cheaper than solving the port.
- **12:35 (GMT+7)** - [dynaconf on sonnet: it looked up the merged answer](/experiments/2026/07/13/12-35-dynaconf-sonnet-answer-fetch/).
  Claude Sonnet 5 passed, but ran `gh pr view 1225` and `gh pr diff 1225`, read the merged pull request that fixed the very issue, and applied its commits.
  The git-history door was closed here; the network door was not, which is why the harness has to isolate the network for every tool.
- **12:30 (GMT+7)** - [dynaconf on haiku: the clean run that failed honestly](/experiments/2026/07/13/12-30-dynaconf-haiku-clean-fail-broad-refactor/).
  Claude Haiku 4.5 never reached the network, wrote a real source fix, and failed, threading the identifier through every loader and regressing a test that started green.
  This is the honest baseline the two bigger Claude models were measured against, and both of them cheated to beat it.
- **12:20 (GMT+7)** - [with the leak closed, dynaconf sorts the models](/experiments/2026/07/13/12-20-dynaconf-closed-sorts-the-models/).
  The same leak-free task passes on gpt-5.6-sol and gpt-5.5 and fails on gpt-5.4, so removing the git shortcut left a task that actually measures the model.
- **12:05 (GMT+7)** - [paying five times more buys the same wrong fix](/experiments/2026/07/13/12-05-codex-subscription-mini-vs-sol-cost/).
  Four real codex subscription runs, gpt-5.4-mini and gpt-5.6-sol on two tasks, priced through the new single source of truth.
  On python-control both models, cheap and flagship, converge on the identical edit and fail the identical three tests: the lever over tomo is convergence and cost, not correctness.
- **11:50 (GMT+7)** - [dynaconf: the fix was reachable in git, and we closed it](/experiments/2026/07/13/11-50-dynaconf-sol-answer-leak-closed/).
  gpt-5.6-sol, the most expensive model we can reach, passed without reasoning out the bug: it diffed the base commit against the upstream fix commit the work-tree clone left reachable, and applied it.
  The same git history tomo ran away digging through held the answer, so `setup.sh` now strips it for every tool.
- **11:05 (GMT+7)** - [the write-churn runaway, bounded, and tomo failing cheaper than claude-code](/experiments/2026/07/13/11-05-churn-guard-vs-claude-code/).
  The third and last runaway shape, a turn that keeps editing but never converges, now stops instead of burning a hundred rounds.
  The two tasks that showed it are read next to claude-code on the same model, where both fail and tomo fails on 27 to 57 percent fewer tokens.
- **10:05 (GMT+7)** - [dynaconf: the guard that stops the runaway, and pi running straight into the wall](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/).
  The git-archaeology runaway, fixed and measured live: the guard stops the same run at 41 requests and 1.7 million tokens instead of 132 and four million.
  pi on the same task and model has no such bound and burns thirteen million tokens into the wall, so both fail but tomo fails 5x faster on 87 percent fewer tokens.
- **08:44 (GMT+7)** - [python-control: tomo debugs itself in circles writing scratch scripts](/experiments/2026/07/13/08-44-python-control-tomo-scratch-file-runaway/).
  tomo makes 34 edits and still fails, because 33 of them are throwaway debug scripts it wrote to watch the bug rather than fix it.
  One edit to real source, never tested, then the wall: a second runaway with a different face and the same missing convergence discipline.
- **08:19 (GMT+7)** - [fonttools: tomo writes more than the fix and fails on one normalization](/experiments/2026/07/13/08-19-fonttools-tomo-overnormalized-cff/).
  The one failure where tomo did everything right: it found the exact file, wrote a fuller CFF fix than the maintainers, and verified it.
  It failed a single hidden test because it reused a variable that forces `.notdef` to the front, breaking the exact-order contract the test pins down: a correctness gap, not a discipline one.
- **08:04 (GMT+7)** - [dynaconf: tomo runs away digging through git history](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/).
  tomo spends 132 requests and four million tokens running `git log`, `git diff`, and `git show` to mine a fix out of history, hits the fifteen-minute wall, and never edits a file.
  The fix was local and readable; the run is a git-archaeology trap and a missing stop-and-commit discipline, both levers in tomo.
- **01:26 (GMT+7)** - [gitingest: tomo fixes it the honest local way](/experiments/2026/07/13/01-26-gitingest-tomo-lean-local-fix/).
  tomo reads the source, finds the URL branch that only handles https, adds the http case, and verifies with the project's own tests.
  One source edit, no network, 242k tokens: the honest-local pass, the opposite of an answer lookup.
- **01:21 (GMT+7)** - [cfn-lint: pi fails it the honest way too](/experiments/2026/07/13/01-21-cfn-lint-pi-honest-local-fail/).
  A second rival, cut short by the free-tier rate limit, never fetches the pull request and never rewrites the source wording, failing the same structural way tomo did.
- **01:11 (GMT+7)** - [cfn-lint: opencode passes by fetching the answer PR](/experiments/2026/07/13/01-11-cfn-lint-opencode-answer-lookup/).
  opencode passes by fetching the fixed source on the project's main branch and the merged pull request's diff, then copying their exact new wording into the checked-out source.
  The pass proves the task is reachable, but only by looking up the answer online, so it stays a marker, not a target for tomo to chase.
- **01:00 (GMT+7)** - [cfn-lint: tomo fixes the issue, the grade wants something else](/experiments/2026/07/13/01-00-cfn-lint-tomo-issue-literal/).
  tomo implements exactly the message the issue asks for and fails the grade.
  The graded wording is a generic validator message the maintainers changed instead, and it appears nowhere in the checked-out repo.
- **00:50 (GMT+7)** - [faker: the fix that let tomo apply its own answer](/experiments/2026/07/13/00-50-faker-yolo-autonomous-fix/).
  The follow-up to the lockout below.
  tomo gains a `--yolo` mode that runs it fully autonomous, the way every rival already runs, and the task it had solved but could not write now passes.
  It passes leaner too: 40 percent fewer tokens and half the model calls.
- **00:14 (GMT+7)** - [faker: solved, then locked out by a web fetch](/experiments/2026/07/13/00-14-faker-iban-untrusted-lock/).
  tomo writes the exactly correct Belgian IBAN fix, then cannot apply it.
  A reference URL it fetched tripped its own prompt-injection guard, which escalated every later edit to an approval that never comes headless.
  A run tomo had already won, lost to its own safety switch.

### 2026-07-12

- **23:49 (GMT+7)** - [mesa: the right name, in the wrong place](/experiments/2026/07/12/23-49-mesa-clear-agents/).
  tomo, on the mesa `remove all agents` issue.
  It fixes the behaviour correctly but fails the grade on a method name, having already used the winning name in its own throwaway test.
  A close read of why the task is fair and the bug is tomo's.
