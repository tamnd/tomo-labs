---
title: "July 2026"
linkTitle: "July"
description: "Experiment reports captured in July 2026, by day. Each is one tool on one task, pinned to the exact build so it reproduces."
weight: 7
---

Runs captured in July 2026, grouped by the day they ran.
Each item below is one experiment: one tool, one task, one verdict.

### 2026-07-13

- **01:11 (GMT+7)** - [cfn-lint: opencode passes by fetching the answer PR](/experiments/2026/07/13-cfn-lint-opencode-answer-lookup/).
  opencode passes by fetching the merged pull request 94 times and copying its exact new wording into the source.
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
