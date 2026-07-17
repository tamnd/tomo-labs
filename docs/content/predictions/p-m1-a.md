---
title: "P-M1-A: at least one published gap dissolves under honest n"
linkTitle: "P-M1-A"
description: "The aggregated table shows at least one previously-published single-run gap shrinking inside noise under n >= 10."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-M1-A
filed:     2026-07-17, from spec 2105 doc 10 section 5.4, before the M1 gate run
suite:     core 14 plus the swebench-live cut, wherever a single-run gap was published
tier:      free
engine:    the pairs the old single-run tables compared
band:      floor: at least one previously-published single-run gap lands inside the
           spread band at n >= 10.
           ceiling: the gaps that were mechanism-backed (with a written story and a
           trajectory signature) survive aggregation.
mechanism: the repeat record shows even n=5 misled by 2x on this substrate; single-run
           tables published before the aggregation machinery must contain at least one
           gap that was sampling noise wearing a verdict.
settled by: the M1 exit gate report, n >= 10 on tomo-oi and tomo-cx.
           go run ./cmd/lab report && go run ./cmd/lab report --suite swebench-live
result:    (open)
verdict:   (open)
```

This prediction is the measurement law auditing its own past.
A miss, meaning every published gap survives aggregation, would say the old single-run tables were luckier or better-mechanism-grounded than the repeat record suggests, and that too is worth a written verdict.
