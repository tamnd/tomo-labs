---
title: "The regression guard is a finish gate, and a free model on a 13-item port never reaches the finish"
linkTitle: "Regression guard, inert on a checklist port"
description: "Experiment 0081. The reproduction gate holds a coding turn to a red-to-green, but it says nothing about what else the fix broke, so the regression guard runs the project's own tests before the loop and refuses to converge if a fix regresses one that was green. Unit-tested and shipped in v0.2.11, then A/B'd live on dynaconf-1225 with deepseek-v4-flash-free. The guard fired zero times in both arms. Not because the fixes were clean, but because the run never converged: the test-authoring sub-flow faithfully wrote one reproduction per item of a thirteen-item port from PR #1204, the free model tried to satisfy all of them, edited a dozen files, broke test collection, and churned to the round cap without ever reaching the finish line the guard sits on. Zero of five, inconclusive for the guard, and a clear read on where the wall actually is."
date: 2026-07-24T09:30:00+07:00
---

Experiment 0080 left a specific next step.
With the test-authoring sub-flow armed, a free model handed one broad red test made that test its whole world, rewrote the identifier-passing signature across seven loader entry points in a single pass, turned its own test green, and shipped a patch that regressed four tests that had been passing.
The reproduction gate proved the model fixed the reported bug and said nothing about what else the fix touched.
The proposed lever was a finish-side counterpart: before accepting a turn as done, run the pre-existing tests the change could touch and refuse to converge if any that passed now fail.

This note builds that guard, ships it, and runs it live.
The guard is correct and unit-tested.
The live A/B is inconclusive for it, and the reason it is inconclusive is the actual finding.

## The guard

The reproduction gate is a red-to-green: it holds the turn until a test the run authored goes from failing to passing, so the model proves it fixed the bug.
The regression guard is the other half.
Before the model edits anything, it runs the project's existing suite once and records the set of tests that currently pass, the baseline green set, the behavior the repository shipped working.
At every finish attempt on a turn that edited the tree, it runs the same suite again and refuses the ending if any baseline-green test now fails, naming the broken tests so the model repairs the regression instead of committing it.

It stays on the right side of the no-tailoring line the same way the other gates do.
The baseline is the repository's own existing tests, the suite any developer runs before committing, never the task's hidden grading suite, which the harness never sees.
The scratch reproduction the run authored is excluded from the baseline, so the guard protects only behavior that predates the turn.
It watches a pass-to-fail transition in tests that were already present, reads no issue text, and names no file the harness supplied.
Armed opt-in with `TOMO_OI_REGRESS=1` so it can be A/B'd, and bounded by two firings so a model that cannot un-break what it broke does not loop forever.

Four unit tests pin the behavior: it nudges then allows a repair, it stays silent when off, it is bounded by the firing cap, and it is silent on a clean fix that pays only the re-check.
It shipped in v0.2.11.

## The run

Same faithful harness as the luna notes: per-instance container, no-egress internal network, offline grade in a fresh instance container.
Two arms, `deepseek-v4-flash-free` fixed, twenty-round cap, pyright as the LSP, the test-authoring sub-flow armed in both.
The only difference is the guard.

| arm | flags | resolved | fail-to-pass | patch lines | guard firings |
|---|---|---|---|---|---|
| control | focus + testgen | False | 0 / 5 | 237 | n/a |
| treatment | focus + testgen + regress | False | 0 / 5 | 1086 | 0 |

Both zero of five.
Every graded test came back not as a failure but as `MISSING`: pytest could not collect `tests/test_settings_loader.py` at all under either patch, because the model's edits broke the import.
The guard fired zero times.

## Why the guard stayed inert

The guard is a finish gate.
It only runs at a finish attempt on an edited tree, and it can only fire when the model has converged: reached the point where it believes it is done and tries to end the turn.
This run never got there.

The test-authoring sub-flow, handed a checklist issue, did exactly what it was built to do and authored a comprehensive reproduction.
The issue is titled "Ports from #1204 to master" and is a thirteen-item checklist: insert token, `load_file` source metadata, `populate_obj`, `--json`, Django settings discovery, `json.dumps` repr on the CLI, settings_loader loading multiple environments, env_loader multiple prefixes, a `*_loader` identifier parameter, redis_loader `None` prefix, a Validator identifier, all of #1204's tests, and the docs.
The sub-flow wrote a test per item.
The trace carries `test_insert_token`, `test_env_loader_must_allow_multiple_prefixes`, `test_load_file_source_metadata`, `test_loader_must_take_identifier_param`, `test_new_way_find_django_settings`, `test_populate_obj_internal_attr`, `test_json_dumps_defaults_to_repr_on_cli`, and more.
That is not a hallucination.
It is a faithful, broad reproduction of a broad issue.

And that is the trap.
A comprehensive reproduction on a thirteen-item port turns the free model into a thirteen-feature porter.
It tried to satisfy all of it at once, edited a dozen files, ballooned to a thousand-line patch in the treatment arm, and broke test collection so hard that even the one item the hidden suite actually grades, settings_loader loading multiple environments, went from a target to an uncollectable module.
Its own broad reproduction never all went green, so it never converged, so it churned to the round and governor caps and never reached the finish line where the guard waits.

The guard cannot help a run that fails by never finishing.
It is built for 0080's shape, a model that converges cleanly on a narrow fix and over-edits at the last step; it bites a pass-to-fail transition at the moment of finish.
This run has no moment of finish to bite.

The 237-versus-1086 patch gap between the arms is not the guard doing anything either.
The guard never fired, so it changed no edit.
The gap is the test-authoring sub-flow plus free-model non-determinism: the control arm batched hard and gave up after a handful of mega-rounds with a nine-line session trace, the treatment arm ground on for forty-eight message lines.
Same wall, different amount of thrashing before hitting it.

## Where the wall actually is

Three arcs now point at the same thing on this task.

The scope gate (0078) capped the model's edit breadth and the model stalled at the ceiling.
The focus directive (0079) killed the empty patch but the model still would not converge on a green reproduction.
The test-authoring sub-flow (0080, and this note) got the reproduction written and faithful, and the model drowned trying to make all of it green.

The wall on dynaconf-1225 for a cheap model is not minimality and not the finish line.
It is decomposition.
A thirteen-item checklist wants to be landed one item at a time, each with the suite staying green between items, and every lever so far has instead handed the model the whole checklist at once, either as a scope to cover or as a reproduction to satisfy.
The regression guard is a correct and cheap gate that will earn its keep the moment a model converges and over-edits, which is a real failure shape, just not this task's for this model.

So the guard stays in, armed opt-in, unit-tested, and honestly labeled as inconclusive here.
The next lever should not sit at the finish line.
It should sit upstream, at target selection: land the smallest coherent slice of a checklist issue first, keep the baseline green, and only then take the next item, instead of authoring the whole port as one red wall a free model cannot climb.

Zero of five, and the clearest read yet on why.
