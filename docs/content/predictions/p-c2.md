---
title: "P-C2: the oi leanness survives repricing"
linkTitle: "P-C2"
description: "On DeepSeek-class caching, tomo-oi's effective cost per pass on the core suite lands in 0.3x to 0.6x of tomo-cx."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-C2
filed:     2026-07-17, from spec 2105 doc 03 (cost model) section 8.2, before the M4 gate run
suite:     core 14
tier:      mid (DeepSeek-class caching)
engine:    tomo-oi against tomo-cx
band:      floor: 0.3x (oi would need to be even leaner than the best observed case
           after the cache discount).
           ceiling: 0.6x (above this, the leanness thesis is weaker than the
           total-token evidence suggested).
mechanism: the observed total-token spread of 2.2x to 4.8x between oi and cx compresses
           toward parity under the deep cache discount, because the discount helps the
           fatter transcript more in absolute terms, but not enough to close the
           structural gap.
settled by: the first M4 gate run on the core 14, mid tier, n >= 10.
           go run ./cmd/lab run tomo-oi && go run ./cmd/lab run tomo-cx && go run ./cmd/lab report
result:    (open)
verdict:   (open)
```

The denominator is per pass: total effective cost of the arm's passing runs divided by its pass count.
A cheap arm that fails everything cannot win this column, and both arms' pass rates publish beside the ratio.

A miss above 0.6x would be the single most important finding the gate could produce, because it would say the cost clause of the lab's headline bar rests on total-token evidence that does not survive repricing.
