---
title: "P-C1: repricing reorders a published comparison"
linkTitle: "P-C1"
description: "Landing the effective-cost columns changes no engine behavior but reorders at least one published rival comparison."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-C1
filed:     2026-07-17, from spec 2105 doc 03 (cost model) section 8.1, before the M1 gate run
suite:     core 14 plus swebench-live, the existing archived tables
tier:      the tiers the archived runs used (free-tier zen models)
engine:    no engine change; this is a pure accounting claim
band:      floor: at least one published pairwise rival ordering changes sign or loses
           significance when repriced under the fresh/cached contract.
           ceiling: no pass or fail grade changes anywhere, because pricing touches no
           engine code.
mechanism: the old verdicts were rendered on headline-token columns across mixed
           cache-fidelity paths, so repricing the same raw traces under the effective-cost
           contract should flip at least one pairwise ordering.
settled by: the M1 gate report over the archived traces, repriced at ingestion.
           go run ./cmd/lab reparse && go run ./cmd/lab report
result:    (open)
verdict:   (open)
```

The interesting outcome either way.
A hit says the old cost columns were actively misleading and the repricing was overdue.
A miss, meaning nothing flips, says the old verdicts were robust despite the banned columns, which lowers the urgency of relitigating old claims and lets the campaign spend its time forward.
That verdict is worth having in writing, which is why this entry exists.
