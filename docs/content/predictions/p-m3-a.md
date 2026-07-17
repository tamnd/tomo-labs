---
title: "P-M3-A: the spend-ceiling slice is invisible at defaults"
linkTitle: "P-M3-A"
description: "The spend-ceiling slice changes nothing on any suite at defaults; a pure rail is invisible until a runaway makes it visible."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-M3-A
filed:     2026-07-17, from spec 2105 doc 10 section 7.4, before the M3 gate runs
suite:     core 14 and swebench-live
tier:      all tiers the M3 gate runs
engine:    any engine under the spend ceiling at default values
band:      floor: no pass count, token median, or cost column moves on any suite when
           the ceiling slice lands at defaults.
           ceiling: the ceiling's first visible act, whenever it comes, is a capped
           entry on a runaway, not a changed graded row.
mechanism: a pure-rail slice adds a bound that no healthy run approaches, so at
           defaults it should be observationally absent until a runaway crosses it.
settled by: the M3 gate sweep, compared column by column against the pre-slice report.
           go run ./cmd/lab report && go run ./cmd/lab report --suite swebench-live
result:    (open)
verdict:   (open)
```

This entry complements P-C3.
P-C3 predicts where the ceiling fires first; this one predicts that landing the ceiling changes nothing before that first fire.
A miss here means the defaults were set inside the envelope healthy runs actually use, which reopens the knob before any spend claim can lean on it.
