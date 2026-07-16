---
title: "The issue title was the trap: on sqllineage-661 even the flagship edits the wrong file"
linkTitle: "sqllineage-661, the flagship also misses"
description: "The free models could not solve sqllineage-661, and all of them patched the public entry point instead of the parser where the bug lives. The obvious next question is whether a stronger model closes it, so tomo-oi ran it on the three gpt-5.6 variants through the codex subscription. All three fail, and all three edit the same wrong file. Six models now, three free and three flagship, and every one lands on runner.py; zero touch the parser. The reason is in the issue itself: it is titled inconsistent order of lineage tuples, so every model reads it as a sorting bug and sorts the output in the entry point, when the real defect is a stray parser recursion that invents a spurious lineage tuple. The failing test is hidden, so there is no red assertion to correct the framing. This is not a capability gap a better model climbs, it is a mis-framed-issue trap, and there is no fair harness fix because the fix is to disbelieve the issue title."
date: 2026-07-17T00:30:00+07:00
---

The previous slice left sqllineage-661 with a clean but unsatisfying result: none of the five free models solved it, and the three that ran all patched the public entry point, `runner.py`, when the real bug is a thirteen-line fix deep in the sqlfluff parser.
That looked like a capability gap, the kind a stronger model closes.
So this slice asks the direct question: does a flagship model solve it.

tomo-oi ran sqllineage-661 on the three gpt-5.6 variants, sol, terra, and luna, one graded pass@1 each, driving the codex subscription through the lab bridge so the harness is the same and only the model changes.
All three fail.
All three edit the same wrong file.

| Model | Rounds | Input tokens | Cost | Result | Edited |
|---|---|---|---|---|---|
| gpt-5.6-sol | 11 | 115.4k | $0.6138 | fail | runner.py |
| gpt-5.6-terra | 4 | 14.5k | $0.0532 | fail | runner.py |
| gpt-5.6-luna | 7 | 64.6k | $0.0810 | fail | runner.py |

Six models now, three free and three flagship, and every one lands on `runner.py`.
Zero touch the parser.
When the cheap tier and the expensive tier fail in the exact same place, the cause is not the model.

## The issue title is the trap

The task's issue is titled "Inconsistent order of lineage tuples", and the reporter says the example "inconsistently fails or passes because the column lineages are ordered differently".
The prompt adds: make the smallest change that fixes the issue.
Read that at face value and it is an ordering bug, and the smallest change is to sort the lineage output deterministically in `runner.py`, where the public call returns its tuples.

That is exactly what every model did.
gpt-5.6-sol rewrote a sort key in `runner.py`, announced that the lineage tuples now have consistent full-path ordering, ran the visible suite, saw twenty-three green, and stopped.
The free models made the same class of edit in the same file.

The real bug is not ordering.
The gold fix is in `sqllineage/core/parser/sqlfluff/utils.py`: when a select reads from a subquery that has a join inside it but no join at the top level, the parser must not recurse into the subquery and pull in the inner join's lineage.
The non-determinism the reporter noticed is a symptom of a spurious lineage tuple that stray recursion invents, not of an unstable sort.
Sorting the output makes the order stable and leaves the spurious tuple in place, so the hidden test still fails.

## Why nothing corrected the framing

The failing test is invisible to the agent.
It is added by the hidden test at grade time and is not in the checkout, and the reproduction in the issue is prose, not a test in the tree.
So the models ran the visible suite, saw it pass, and never had a red assertion to push back on their reading of the issue.
The "twenty-three passed" was true of the visible suite and irrelevant to the grade.
There was no failing signal to widen from.

## Why there is no fair fix here

The one lever that would flip these runs is to tell the model the issue title is wrong, that the defect is in the parser and not in the ordering.
That is the answer to the task, and putting it in the harness, even softly and aimed at this task, is a leak.
The engine stays as it is.

There is one general lever worth naming, not as a fix but as a future experiment: a reproduce-first discipline, where before editing the agent turns the issue's reproduction into a runnable script and confirms it fails, then fixes until it passes.
On this task that might help, because running the issue's own SQL surfaces the spurious lineage tuple rather than just an unstable order.
But it is universal debugging advice, not a steer toward this answer, it is speculative here, and it belongs in its own measured experiment.

## Lessons

- When the cheap tier and the flagship fail in the same file for the same reason, stop blaming the model. sqllineage-661 is a diagnosis trap, and six models walked into it identically.
- An issue title is a hypothesis, not a spec. "Inconsistent order" pointed every model at a sort when the fix was a parser recursion, and no model doubted the title.
- A hidden failing test removes the one signal that could break a wrong framing. With nothing red in the checkout, verify-to-green has nothing to pull against, and confident mis-reading goes unchallenged.
- There is no fair harness fix for a task whose only remaining lever is the location of the answer. Saying so plainly, and leaving one honest general idea for a separate experiment, is the result.

## Reproduce

1. Start the codex bridge on one gpt-5.6 variant and point the probe at it: `lab bridge --model gpt-5.6-sol --effort medium --port 8790`, then `lab probe reata__sqllineage-661 --engine oi --model gpt-5.6-sol --base-url http://localhost:8790/v1 --grade`.
2. Read the edited files in the run summary. A run that edits `runner.py` and leaves `core/parser/sqlfluff/utils.py` untouched has taken the issue title at face value.
3. Confirm the trap: the failing test is added by the hidden test at grade time and is not in the checkout, so a model that runs the visible suite sees green and has no red assertion to correct its framing.
