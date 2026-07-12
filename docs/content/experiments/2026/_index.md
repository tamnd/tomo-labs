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

- **2026-07-13 01:26 (GMT+7)** - [gitingest: tomo fixes it the honest local way](/experiments/2026/07/13-gitingest-tomo-lean-local-fix/).
  tomo reads the source, finds the URL branch that only handles https, adds the http case, and verifies with the project's own tests: one source edit, no network, the opposite of an answer lookup.
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
