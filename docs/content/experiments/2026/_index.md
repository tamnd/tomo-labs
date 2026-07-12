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

- **2026-07-13 00:14 (GMT+7)** - [faker: solved, then locked out by a web fetch](/experiments/2026/07/13-faker-iban-untrusted-lock/).
  tomo writes the exactly correct Belgian IBAN fix but cannot apply it, because a page it fetched tripped its own prompt-injection guard and every later edit was declined headless.
- **2026-07-12 23:49 (GMT+7)** - [mesa: the right name, in the wrong place](/experiments/2026/07/12-mesa-clear-agents/).
  tomo fixes the mesa `remove all agents` issue correctly but fails the grade on a method name it had already used in its own throwaway test.
