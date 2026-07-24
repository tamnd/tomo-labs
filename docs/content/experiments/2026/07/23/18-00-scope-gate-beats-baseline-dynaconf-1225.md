---
title: "The scope gate beats the baseline on dynaconf-1225, and the last three tests turn out to be a retrieval gap"
linkTitle: "Scope gate beats baseline on dynaconf-1225"
description: "The first lever in this arc to beat the one-of-five baseline on dynaconf__dynaconf-1225. After two rounds proving the reproduction gate is a regression, the reading was that the failure is sprawl and scope, not verification. A scope gate that holds the model to the smallest correct diff and forbids regressing previously-passing tests lifts gpt-5.6-sol to two of five with zero regressions, matching the ceiling two strong native harnesses hit independently. The three still-failing tests are the py-module-path loader cluster, and the run never edited py_loader.py at all: the context pack surfaces py_loader by name but never its body, so the model cannot fix code it does not see. The next lever is a retrieval refinement, not a stronger verifier. Costs are token volumes over the unmetered subscription bridge, dollar cost unknown, never zero."
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

## Why the last three fail: the model never opens the file that matters

The three still-failing tests are the exact cluster codex and pi also stalled on: the multi-temporary-env case, the module-path case, and the module-path-under-multi-env case.
All three are decided in `py_loader.py`, in `load` and `try_to_load_from_py_module_name`: comma-separated temporary envs that must not mutate the current env, env-named sibling files, dotted-module and slash-file path forms.

The scope run edited twelve files.
Six of them are format loaders, ini and json and redis and toml and vault and yaml, that have nothing to do with this bug.
It never edited `py_loader.py`.
The model cannot fix code it never opens.

This is not the flat capability wall the previous note framed.
It is a retrieval gap.
The call-edge expansion shipped earlier does surface py_loader in the first request's context pack, marked "reached from settings_loader", so the model sees the name.
But the probe budget fills with call sites inside the large `settings_loader` body before it reaches `py_loader.load`'s own body, so the pack shows where py_loader is called and not what it does.
The model sees a referenced symbol with no body, reads it as out of scope, and spends its edits on the loaders it can actually read.

## What comes next, and the line it must not cross

The direction is a retrieval refinement, not a prompt.
Order the context pack so a callee reached across a call edge gets its own body section, not only its call sites, when the budget allows, and prefer a not-yet-shown callee body over another call site inside a function already in the pack.
That is general: it names no file from the issue, it changes how the pack is built for any task, and it would surface the decisive callee in any bug whose fix sits one hop out from the named entry point.
It stays on the right side of the no-tailoring line because it is a budget-ordering rule over the call graph, not a rule about dynaconf.

The issue-example-derived required-test lever remains the other open direction, but this run says retrieval comes first.
A required test the model cannot make pass because it never sees the loader is no more use than the reproduction gate was.

Costs are token volumes; the model was served over the unmetered subscription bridge, so the dollar cost is unknown, never zero.
The run is on the public trace index under `dynaconf__dynaconf-1225`.
It does not resolve the task.
It is the first positive lever result in this arc: the baseline recovered and beaten, the ceiling of two strong native harnesses matched, and the remaining gap localized to a specific, fixable retrieval limitation rather than left as an undifferentiated wall.
