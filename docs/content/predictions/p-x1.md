---
title: "P-X1: the head+tail fix buys rounds, not grade"
linkTitle: "P-X1"
description: "The agent-engine head+tail truncation fix changes no pass rate, and improves rounds on at least one debugging-shaped task."
date: 2026-07-17T10:00:00+07:00
---

```
id:        P-X1
filed:     2026-07-17, from spec 2105 doc 05 (context model) section 8, before the M0 landing run
suite:     core 14
tier:      free
engine:    tomo agent engine, before and after the head+tail truncation fix
band:      floor: no pass-rate change in either direction.
           ceiling: rounds improve on at least one debugging-shaped task.
mechanism: head-only truncation drops the FAILED verdict from test output, forcing the
           model to spend extra rounds re-running or narrowing to see what it already
           produced; restoring the tail restores the verdict in-band.
settled by: the M0 landing lab run, agent engine, before/after arms, aggregated.
           go run ./cmd/lab run tomo
result:    (open)
verdict:   (open)
```

The pass-rate half of the claim is the guard against overselling.
The fix is a rounds-and-cost improvement, and if it moves pass rate the mechanism story is wrong and needs re-examination.

The trajectory-level signature to look for: consecutive near-identical test commands in the before arm collapsing to single runs in the after arm.
The signature check is what distinguishes the mechanism confirming from the number moving for another reason.
Scored once, at M0.
