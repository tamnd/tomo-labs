---
title: "P-C3: the ceiling is a tail rail, not a governor"
linkTitle: "P-C3"
description: "The spend ceiling never fires on the core 14 at defaults, and fires first on swebench-live invention-shaped tasks."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-C3
filed:     2026-07-17, from spec 2105 doc 03 (cost model) section 8.3, before the M3 and M4 gate runs
suite:     core 14 and swebench-live
tier:      all tiers the gate runs
engine:    any; the claim is about the spend-ceiling rail, not an engine
band:      floor: zero ceiling fires across all core-14 gate runs at the chosen defaults.
           ceiling: the first ceiling fire in the whole program occurs on a swebench-live
           task tagged invention, and lands in the capped column, not the graded one.
mechanism: the core suite finishes in single-digit rounds for passing runs, far under any
           sane ceiling, while invention-shaped tasks are exactly the runs that burn
           tokens without converging, which is the population the ceiling exists for.
settled by: the M4 full-gate run across suites, with cap types reported in the CAPPED column.
           go run ./cmd/lab report && go run ./cmd/lab report --suite swebench-live
result:    (open)
verdict:   (open)
```

This prediction is also the ceiling's design review in disguise.
A rail that fires only on the runs the reachability tags say should be routed to a stronger model anyway is doing exactly its job: converting unbounded spend on unreachable tasks into a bounded, truthfully labeled stop.
A ceiling fire on a core task at defaults falsifies the chosen defaults, not the mechanism, and reopens the knob.
