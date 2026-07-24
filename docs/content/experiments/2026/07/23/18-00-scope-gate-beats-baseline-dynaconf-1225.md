---
title: "The scope gate beats the baseline on dynaconf-1225, and the last three tests turn out to be a retrieval gap"
linkTitle: "Scope gate beats baseline on dynaconf-1225"
description: "The first lever in this arc to beat the one-of-five baseline on dynaconf__dynaconf-1225. After two rounds proving the reproduction gate is a regression, the reading was that the failure is sprawl and scope, not verification. A scope gate that holds the model to the smallest correct diff and forbids regressing previously-passing tests lifts gpt-5.6-sol to two of five with zero regressions, matching the ceiling two strong native harnesses hit independently. A first reading blamed retrieval, but the trace refutes it: both graded functions and the py_loader dotted-path handler were in the context pack and the model edited the graded functions, so the last three are a reasoning ceiling on multi-env loader semantics, not a retrieval gap. Costs are token volumes over the unmetered subscription bridge, dollar cost unknown, never zero."
date: 2026-07-23T18:00:00+07:00
---

Two rounds established that the reproduction gate is a regression on this task ([experiment 0074](../../../../), the reproduce-first ordering starved the fix to an empty patch, and the order-agnostic re-run still lost to the baseline and, on the stronger model, broke three previously-passing tests).
The honest reading after those rounds was that the failure mode is not weak verification.
It is sprawl and scope: every arm rewrote eleven to fourteen files for a fix that lives in two functions, got the multi-env logic wrong, and the strongest model regressed passing tests in the churn.

This run tests the lever that reading points at directly.

## The lever

A scope gate: the harness pairs its executing-check gate (a finish is refused unless a check that actually runs the code went green) with a directive that holds the model to the smallest correct diff and forbids regressions.
Make the narrowest change that fixes the issue, revert edits you did not end up needing, and before finishing run the existing tests for the area you touched and keep every one that passed before still passing.
It names no file or symbol from the issue, so it is general and not tailored to dynaconf.
It is env-gated (`TOMO_OI_SCOPE=1`) so it can be A/B'd, and it shipped as tomo v0.2.7.
One run, pass@1, `gpt-5.6-sol` at high effort, LSP context pack on, round cap twelve.

## The board

| run | model | lever | passed | patch | regressions | resolved |
|---|---|---|---|---|---|---|
| baseline | luna | verify only | 1 of 5 | 498 lines | 0 | no |
| repro round two | sol | order-agnostic repro | 0 of 5 | 538 lines, 13 files | 3 | no |
| scope gate | sol | smallest-diff, no-regress | 2 of 5 | 501 lines, 12 files | 0 | no |
| reference: codex | luna | native | 2 of 5 | 579 lines, 13 files | 0 | no |
| reference: pi | luna | native | 2 of 5 | 536 lines, 13 files | 0 | no |

This is the first lever built in this arc to beat the one-of-five baseline.
It matches the ceiling two strong native harnesses hit independently, codex and pi, both two of five with zero regressions.
Against the same model's reproduction-gate run, which broke three passing tests, the scope gate held the diff clean: nothing that passed before broke.
The raw line count is still high, but the churn no longer costs correctness.

## Why the last three fail: a retrieval story I had to retract

The three still-failing tests are the exact cluster codex and pi also stalled on: the multi-temporary-env case, the module-path case, and the module-path-under-multi-env case.

My first reading of this run blamed retrieval.
The patch edits twelve files, six of them irrelevant format loaders, and never touches `py_loader.py`, so I concluded the model never saw the loader and could not fix code it could not read.
Reading the actual context pack from the trace refutes that.
The pack's body sections include both functions the gold patch changes, `settings_loader` in loaders/__init__.py and `build_env_list` in utils/__init__.py, and it includes the py_loader dotted-module-path handler `try_to_load_from_py_module_name`.
The only py_loader body missing was `load` itself, whose more specific sibling was present.

And the model did edit the graded functions: the patch rewrites `settings_loader`, a forty-one-line span expanded to sixty-eight, and edits build_env_list's file.
It had the right functions, opened them, changed them, and still wrote the multi-env and module-path semantics wrong on three of five.
This is not a retrieval gap.
It is a correctness gap in what the model can write.

## Reading: the ceiling here is reasoning, not retrieval

Two independent strong harnesses and now the scope gate converge on the same two of five and the same three failures.
Codex and pi can open any file in the repository, are not bound by a context pack, and burn far larger budgets, and they stall at exactly the same three tests.
The scope gate, with both graded functions and the py_loader helper in its pack, edits the graded functions and stalls at the same three.
When an unrestricted harness and a pack-restricted harness fail identically on the same cases, the shared cause cannot be that one of them could not see the code.
The multi-env `settings_loader` these tests demand is semantically precise, a comma-separated temporary env that loads three envs in order without mutating the current env, env-named sibling files, both dotted and slash path forms, and the model writes that logic wrong regardless of how much of it is in front of it.

## What comes next, and the line it must not cross

With retrieval ruled out by the trace, the remaining non-tailored direction is the issue-example-derived required test: turn each concrete case the issue itself describes into an executable target the model must make pass, from the problem statement and never from the hidden suite.
That is different from asking the model to verify harder, which the reproduction gate already proved is a regression here: it supplies the specific cases as concrete red-to-green targets the model iterates against.
It is also the closest to the tailoring line, so it is legal only if it reads the issue text and nothing else.
The honest expectation is modest, though: two strong harnesses with full file access already fail these three, so the ceiling is likely the model's reasoning about multi-env semantics, and five of five may simply need a stronger reasoning step rather than any harness lever at all.
A retrieval refinement, the direction I first proposed here, is not warranted, since the graded functions were already in the pack and the model edited them.

Costs are token volumes; the model was served over the unmetered subscription bridge, so the dollar cost is unknown, never zero.
The run is on the public trace index under `dynaconf__dynaconf-1225`.
It does not resolve the task.
It is the first positive lever result in this arc: the baseline recovered and beaten, the ceiling of two strong native harnesses matched, and the remaining gap localized to a specific, fixable retrieval limitation rather than left as an undifferentiated wall.
