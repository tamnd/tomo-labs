---
title: "P-X3: the working-set header earns its rounds or does not land"
linkTitle: "P-X3"
description: "If the working-set header A/B runs, the header cuts rounds on multi-file tasks, or it does not land."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-X3
filed:     2026-07-17, from spec 2105 doc 05 (context model) section 8; conditional on its A/B running
suite:     a slice partitioned in advance into multi-file tasks (the target population)
           and single-file tasks (the expected-null arm)
tier:      set by the A/B note when it runs
engine:    tomo agent engine, header arm against no-header arm
band:      floor: rounds improve on the multi-file partition.
           ceiling: no effect on the single-file partition (an effect there would itself
           be suspicious).
mechanism: the diffuse-thrashing failure shape is the model losing track of its own
           working set across many files; a bounded truthful header attacks exactly
           that, and nothing else.
settled by: the pre-registered working-set header A/B, whenever the loop schedules it.
result:    (open, unscheduled)
verdict:   (open)
```

The prediction is deliberately one-sided as a ship gate.
Flat rounds means the header is a fresh-token tax and is rejected regardless of any softer benefit story.

A conditional prediction is still a registered one.
If the A/B never runs in a given milestone window, this entry is carried forward unscored, never quietly dropped.
