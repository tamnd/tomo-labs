---
title: "July 2026"
linkTitle: "July"
description: "Experiment reports captured in July 2026, by day. Each is one tool on one task, pinned to the exact build so it reproduces."
weight: 7
---

Runs captured in July 2026, grouped by the day they ran.
Each item below is one experiment: one tool, one task, one verdict.

### 2026-07-13

- **08:44 (GMT+7)** - [python-control: tomo debugs itself in circles writing scratch scripts](/experiments/2026/07/13-python-control-tomo-scratch-file-runaway/).
  tomo makes 34 edits and still fails, because 33 of them are throwaway debug scripts it wrote to watch the bug rather than fix it.
  One edit to real source, never tested, then the wall: a second runaway with a different face and the same missing convergence discipline.
- **08:19 (GMT+7)** - [fonttools: tomo writes more than the fix and fails on one normalization](/experiments/2026/07/13-fonttools-tomo-overnormalized-cff/).
  The one failure where tomo did everything right: it found the exact file, wrote a fuller CFF fix than the maintainers, and verified it.
  It failed a single hidden test because it reused a variable that forces `.notdef` to the front, breaking the exact-order contract the test pins down: a correctness gap, not a discipline one.
- **08:04 (GMT+7)** - [dynaconf: tomo runs away digging through git history](/experiments/2026/07/13-dynaconf-tomo-git-archaeology-runaway/).
  tomo spends 132 requests and four million tokens running `git log`, `git diff`, and `git show` to mine a fix out of history, hits the fifteen-minute wall, and never edits a file.
  The fix was local and readable; the run is a git-archaeology trap and a missing stop-and-commit discipline, both levers in tomo.
- **01:26 (GMT+7)** - [gitingest: tomo fixes it the honest local way](/experiments/2026/07/13-gitingest-tomo-lean-local-fix/).
  tomo reads the source, finds the URL branch that only handles https, adds the http case, and verifies with the project's own tests.
  One source edit, no network, 242k tokens: the honest-local pass, the opposite of an answer lookup.
- **01:21 (GMT+7)** - [cfn-lint: pi fails it the honest way too](/experiments/2026/07/13-cfn-lint-pi-honest-local-fail/).
  A second rival, cut short by the free-tier rate limit, never fetches the pull request and never rewrites the source wording, failing the same structural way tomo did.
- **01:11 (GMT+7)** - [cfn-lint: opencode passes by fetching the answer PR](/experiments/2026/07/13-cfn-lint-opencode-answer-lookup/).
  opencode passes by fetching the fixed source on the project's main branch and the merged pull request's diff, then copying their exact new wording into the checked-out source.
  The pass proves the task is reachable, but only by looking up the answer online, so it stays a marker, not a target for tomo to chase.
- **01:00 (GMT+7)** - [cfn-lint: tomo fixes the issue, the grade wants something else](/experiments/2026/07/13-cfn-lint-tomo-issue-literal/).
  tomo implements exactly the message the issue asks for and fails the grade.
  The graded wording is a generic validator message the maintainers changed instead, and it appears nowhere in the checked-out repo.
- **00:50 (GMT+7)** - [faker: the fix that let tomo apply its own answer](/experiments/2026/07/13-faker-yolo-autonomous-fix/).
  The follow-up to the lockout below.
  tomo gains a `--yolo` mode that runs it fully autonomous, the way every rival already runs, and the task it had solved but could not write now passes.
  It passes leaner too: 40 percent fewer tokens and half the model calls.
- **00:14 (GMT+7)** - [faker: solved, then locked out by a web fetch](/experiments/2026/07/13-faker-iban-untrusted-lock/).
  tomo writes the exactly correct Belgian IBAN fix, then cannot apply it.
  A reference URL it fetched tripped its own prompt-injection guard, which escalated every later edit to an approval that never comes headless.
  A run tomo had already won, lost to its own safety switch.

### 2026-07-12

- **23:49 (GMT+7)** - [mesa: the right name, in the wrong place](/experiments/2026/07/12-mesa-clear-agents/).
  tomo, on the mesa `remove all agents` issue.
  It fixes the behaviour correctly but fails the grade on a method name, having already used the winning name in its own throwaway test.
  A close read of why the task is fair and the bug is tomo's.
