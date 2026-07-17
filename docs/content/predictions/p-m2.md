---
title: "P-M2: the targeted effort purchase converts under 2.5x"
linkTitle: "P-M2"
description: "The targeted effort purchase converts at least one standing near-miss per sweep, under 2.5x fresh tokens."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-M2
filed:     2026-07-17, from spec 2105 doc 07 (model policy) section 9, before the M3 gate runs
suite:     the standing near-miss set, identified by deterministic verify-depth signals
tier:      the tier each near-miss stood on, with effort=high purchased on top
engine:    tomo-oi with the manual effort purchase
band:      floor: at least one task in the standing near-miss set converts per gate sweep.
           ceiling: the fresh-token ratio of a converting purchase stays at or under 2.5x
           against the default-effort attempt (observed band 2.12x and 2.29x).
mechanism: verify-depth near-misses are runs that found the right neighborhood and
           stopped one verification level short; buying reasoning depth on exactly those
           runs converts them at a price that is a property of the effort mechanism,
           not of the task.
settled by: the running purchase ledger, one row per purchase, verdict read at each M3
           gate sweep's close from the conversion count and the fresh-token ratios.
result:    (open)
verdict:   (open)
```

A conversion above 2.5x, or a sweep of purchases with zero conversions, is evidence the near-miss identification is misfiring, and the purchase criteria tighten before any further spend.
