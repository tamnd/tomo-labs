---
title: "Predictions"
linkTitle: "Predictions"
description: "The prediction ledger: every measurable claim about a future run, filed before the run, with a floor, a ceiling, and the mechanism that would explain the number. Verdicts are written against the filed band, hit or miss, and an entry is never edited after its run starts."
weight: 21
featured: true
---

Every gate run in this lab is bound to a prediction that was filed before the run launched.
This section is that ledger: one page per prediction, checked into git, so "was this filed before the run" is answered by commit history rather than by memory.

## Why file predictions at all

A benchmark sweep with no prediction is a pile of numbers that can be read to say anything.
The failure mode is familiar: run enough arms, then narrate whichever gap looks flattering as if it had been the hypothesis all along.
Filing the band first closes that door.
If the number lands inside the band, the mechanism story earned its keep.
If it lands outside, that miss is the finding, and it gets a written verdict instead of a quiet reframe.

The ledger's aggregate hit rate is itself a health number for the lab.
A ledger that always hits is predicting too wide.
A ledger that mostly misses means the mechanism stories are decoration.

## The entry format

Each entry carries the same fields, fixed here so every page reads the same way:

```
id:        the prediction's name, stable forever
filed:     the date the band was frozen, before the settling run
suite:     which task set settles it
tier:      which model tier the settling run uses
engine:    the arm or arms under test
band:      floor and ceiling, both sides, so the claim can miss in either direction
mechanism: the one-sentence story that explains the band, the load-bearing part
settled by: the run that grades it, with the reproduce command
result:    filled after the run, a link to the note or report that carries the numbers
verdict:   hit, over, or under, with one sentence
```

The mechanism sentence is what separates a prediction from a guess.
"The oi engine will land 12 to 14 of 14 because the finishguard family closed the three last-mile failure shapes" is a prediction.
"The oi engine will do well" is not, because nothing could falsify it.

The band has a floor and a ceiling because a claim that only bounds one side cannot miss in the direction that matters.
"At least X" can never miss high, and missing high is evidence too: a mechanism that overperforms its own theory is not understood yet.

## The rules

The ledger is append-only.
An entry is never edited after its settling run starts; the only writes allowed after that point are the result link and the verdict.
A mistaken prediction is answered by its verdict, not by revision.
A revised band is a new entry with its own filing date.

Over and under are distinct verdicts on purpose.
An under means the mechanism story overpromised.
An over means the model or a guard did work the story did not credit.
Both feed the queue of what to try next.

A conditional prediction is still a registered one.
If the run that would settle an entry never launches in a given window, the entry is carried forward unscored, never quietly dropped.

## Reading the ledger

Entries are named by their id.
The P-C family tests the cost model, the P-X family tests the context model, the P-M family (bare numbers) tests the model policy, and the P-M0 through P-M4 entries are the milestone gate predictions of the 2105 campaign.
The naming is inherited from the spec docs that filed each claim; the ledger keeps one flat list rather than one list per doc, so a reader can audit the whole register in one pass.
