---
title: "P-X2: the overflow valve never fires on the core 14"
linkTitle: "P-X2"
description: "The context overflow valve fires on zero of the core 14, and fires only on long dynaconf-class runs, if anywhere."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-X2
filed:     2026-07-17, from spec 2105 doc 05 (context model) section 8, before the first full gate
suite:     core 14 (the zero-firings claim) and swebench-live (the only-here claim)
tier:      all, with free and cheap tiers mattering most (smallest windows)
engine:    tomo agent engine with the overflow valve landed
band:      floor: zero valve firings on any core-14 gate run.
           ceiling: firings, if any, occur only on long swebench-live runs of the
           dynaconf class.
mechanism: with source truncation capping every entry, progressive disclosure keeping the
           prefix lean, and the governor bounding rounds, the core suite's transcripts
           stay far under any realistic window; only pathologically long trajectories
           approach overflow.
settled by: passive instrumentation on every gate run from the first full gate onward;
           valve firings land in the capped column.
           go run ./cmd/lab report
result:    (open, accumulates per gate run)
verdict:   (open, scored per suite per tier as the gate schedule executes)
```

The measurement is passive: valve firings are always-on instrumentation, so every gate run accumulates evidence at zero marginal cost.

This entry is the falsifier for the no-summarizer bet.
If the valve fires on a core-14 task, something upstream (a cap, a trim, a leak of oversized content) has regressed, and that firing is a tripwire before it is a statistic.
A firing pattern that tracks window size would confirm the mechanism; a firing on a large-window frontier run would indicate an upstream regression instead.
