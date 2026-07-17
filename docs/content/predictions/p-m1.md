---
title: "P-M1: cheap-first loses no grade and halves cost per pass"
linkTitle: "P-M1"
description: "Cheap-first loses no grade on substitution-tagged tasks and cuts cost per pass by at least 2x."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-M1
filed:     2026-07-17, from spec 2105 doc 07 (model policy) section 9, before the M3 gate runs
suite:     the substitution-tagged slice of the core 14
tier:      cheap-first ladder (cheap attempt, escalate on fail) against mid-tier-first
engine:    tomo-oi under the model-routing policy
band:      floor: cost per pass cut by at least 2x with no aggregated pass-rate loss.
           ceiling: the grade half holds exactly (any grade loss on tagged tasks is a
           miss regardless of the cost win).
mechanism: with cheap output at $0.28/MTok against mid-tier rates, even a modest cheap
           pass rate on tasks the reachability tag says are reachable leaves the blended
           cost far below mid-first, and 2x is the conservative bound.
settled by: a two-arm comparison over the substitution-tagged core slice, both arms at
           the repeat floor, verdict read from per-climb cost sums and tier-credited
           pass columns at the M3 gate.
result:    (open)
verdict:   (open)
```

The escalation accounting is part of the claim.
The cheap-first arm is charged with every failed cheap attempt it spends, because hiding the failed rungs would be the triangular-mirage trick applied to escalation.

A miss on the grade half would say the reachability tags are wrong.
A miss on the cost half would say the escalation rate on tagged tasks is higher than the tag evidence implies.
