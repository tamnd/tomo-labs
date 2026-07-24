---
title: "The checklist decomposer fixes the failure mode: one item lands clean, the suite stays green, the sprawl is gone"
linkTitle: "Decomposer fixes the failure mode"
description: "Experiment 0082. 0081 found the wall on dynaconf-1225 was decomposition: a thirteen-item port handed to a free model as one red wall made it a many-feature porter that edited a dozen files, broke test collection, and never converged. The decomposer splits the issue into ordered items and works one at a time, holding the reproduction gate to a single item and folding each landed item into the regression baseline before the next. Shipped in v0.2.12 and A/B'd on the same task and model. The result is the clearest behavioral win of the arc: the free model split the issue into eight items, landed the first one as a tight thirty-one-line fix, and kept every pre-existing test green, where the prior run broke collection so hard that even the graded tests went uncollectable. Still zero of five, because the graded item sits deep in the smallest-first list and the model cleared only one item inside its round budget. The mechanism works; the remaining gap is throughput, not sprawl."
date: 2026-07-24T14:00:00+07:00
---

Experiment 0081 ended with a diagnosis and a next step.
On dynaconf-1225 the wall for a free model was not targeting and not minimality, it was decomposition: the issue is a thirteen-item port from PR #1204, the test-authoring sub-flow faithfully wrote one reproduction per item, and the model, handed the whole checklist as one red wall, became a thirteen-feature porter.
It edited a dozen files at once, broke test collection so hard that even the one graded item went uncollectable, and never turned its own broad reproduction green, so it never converged.
The proposed lever sat upstream of the finish line: land the smallest coherent slice first, keep the baseline green, then take the next item.

This note builds that lever, ships it, and runs it.
The result is the clearest behavioral win in this arc, and an honest zero of five.

## The decomposer

Before the loop, one focused call reads the issue alone and decides whether it is several independent items or one coherent change.
A single-item issue disarms the decomposer and the run falls back to the whole-issue test-authoring sub-flow, so nothing is lost on the bugs that flow already handled.
On a real checklist it returns the items ordered smallest and most foundational first, and the run authors a reproduction for the first item only, holds the reproduction gate to that one item, and lets the model work until its test goes red to green.

When an item lands, the decomposer does three things before moving on.
It folds the item's fix into the regression baseline, so the next item cannot break what this one fixed.
It authors the next item's reproduction over the same scratch path.
It re-arms the gate.
The model walks the checklist one coherent slice at a time, keeping the earlier items working, instead of climbing a wall it cannot climb.
It reads only the issue text, never a hidden grading suite, and it is armed opt-in with `TOMO_OI_DECOMPOSE=1` and bounded to a fixed item count so the walk terminates.

## The run

Same faithful harness, same model, both arms `deepseek-v4-flash-free` at a twenty-round cap with pyright as the LSP.
Control is the prior best from 0081, focus plus the test-authoring sub-flow plus the regression guard.
Treatment swaps the sub-flow for the decomposer.

| arm | flags | resolved | fail-to-pass | patch lines | pass-to-pass broken |
|---|---|---|---|---|---|
| control | focus + testgen + regress | False | 0 / 5 | 0 | collection broke |
| treatment | focus + regress + decompose | False | 0 / 5 | 31 | 0 |

Both zero of five.
Everything else changed.

## What the decomposer did

The split call read the issue and returned eight items.
The trace shows the walk: `[decompose] 8 items, working 1/8`, then `[decompose] item 1/8 done, working 2/8`.
The model landed the first item and advanced to the second before the round budget ran out.

The first item was the redis loader's `None` prefix, one of the thirteen #1204 items, and the fix is exactly what that item asks for:

```python
-    prefix = obj.get("ENVVAR_PREFIX_FOR_DYNACONF")
+    prefix = obj.get("ENVVAR_PREFIX_FOR_DYNACONF") or ""
```

Thirty-one lines across the redis loader, a `None`-guarded prefix in the three places it is read.
Its reproduction went red then green, the regression guard confirmed nothing that passed before now failed, the decomposer folded it into the baseline and moved to item two.

Compare that to what the same model did on the same task without the decomposer.
In 0081 the treatment patch was a thousand and eighty-six lines across a dozen files, and it broke test collection so badly that every graded test came back not as a failure but as `MISSING`: pytest could not even import the test module.
Here the patch is thirty-one lines, collection is intact, the five graded tests come back cleanly `FAILED` rather than uncollectable, and all four pre-existing tests still pass.
The decomposer did exactly what it was built to do: it turned a non-convergent sprawl that broke the suite into a convergent, minimal walk that lands one real item and keeps everything green.

## Why it is still zero of five

The graded slice is `tests/test_settings_loader.py`, the settings_loader-multiple-environments item, which is one of the larger and more foundational-sounding items but also one of the harder ones.
The decomposer ordered the items smallest-and-most-foundational first, which correctly put the trivial redis one-liner near the front, and the free model spent its whole twenty-round budget landing that first item and starting the second.
It never reached the graded item, so none of the five FAIL_TO_PASS tests flipped.

This is a throughput gap, not a sprawl gap.
The model is no longer failing by trying to do everything at once and breaking the build; it is now failing by doing one thing at a time correctly and running out of budget before it reaches the thing that is graded.
That is a much better place to be stuck, because it is a place two independent levers can move: give the walk more budget per item, or change the item ordering so a graded-adjacent item comes up sooner without tailoring to the hidden suite.

## Where this leaves the arc

Four experiments now trace one wall and its retreat.
The scope gate capped breadth and the model stalled at the ceiling.
The focus directive killed the empty patch but the model would not converge.
The test-authoring sub-flow got the reproduction written and faithful and the model drowned making all of it green.
The decomposer stops the drowning: it makes the model land one item, keep the suite green, and move on, which is the first time a free model on this task produced a small correct change instead of a large broken one.

The task is not solved, and this note does not pretend it is.
But the failure mode that defeated every earlier lever, sprawl into a broken build, is gone, replaced by a slower and far more tractable one.
The next lever is throughput: how many items a cheap model can land per turn, and in what order, so the walk reaches the graded one before the budget ends.

One of eight items landed clean, zero regressions, and the clearest read yet that decomposition was the right diagnosis.
