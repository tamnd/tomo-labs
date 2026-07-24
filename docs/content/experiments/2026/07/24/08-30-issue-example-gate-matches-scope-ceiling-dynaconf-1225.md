---
title: "The issue-example gate matches the scope ceiling on dynaconf-1225, and confirms it: a correct checklist did not supply the algorithm"
linkTitle: "Issue-example gate matches scope ceiling"
description: "The last non-tailored harness lever for dynaconf__dynaconf-1225, tested after trace inspection localized the remaining three failures to a model-reasoning ceiling rather than retrieval. The issue-example gate is a pre-loop sub-flow that distills the issue into a checklist of its own concrete cases and injects them as required red-to-green targets, from the issue text alone. It lands at two of five with zero regressions on a smaller patch, the same solve rate and the same two passing tests as the scope gate, and it fails the same three py-module-path cases. The extraction fired and, tellingly, its checklist was correct on the graded cases, naming settings_loader and build_env_list and the multi-env behavior the hidden tests check. The model had the code and a correct per-case contract and still wrote the semantics wrong. Four levers and two native harnesses now converge on the same two of five: the ceiling is reasoning, and five of five needs a stronger reasoning step, not another lever. Costs are token volumes over the unmetered subscription bridge, dollar cost unknown, never zero."
date: 2026-07-24T08:30:00+07:00
---

The scope-gate run established, by inspecting its own context pack, that the three still-failing tests on dynaconf-1225 are a reasoning ceiling and not a retrieval gap: both graded functions and the py_loader dotted-path handler were in the pack, and the model edited the graded functions and still wrote the multi-env semantics wrong.
That reading left one non-tailored lever untried, and this run tries it.

## The lever

The issue-example gate is a pre-loop sub-flow, not a prompt line.
Before the coding loop, one focused call reads the issue text alone and distills it into a checklist of the concrete cases the issue itself states, one testable line each.
That checklist is injected as required targets: the model must write a red-then-green test for every case, not one of its choosing, and the reproduction gate holds the finish to a real red-to-green.
The novelty over the plain reproduction gate, which was a regression here, is the enumeration: it turns the issue's prose into a per-case contract a single-case fix cannot satisfy.
It reads only the issue the task shipped with, never the workspace tests and never the hidden grading suite, so it stays general and untailored.
It shipped as tomo v0.2.8 and was armed alone, verify plus examples with no scope, for clean attribution, run once, pass@1, gpt-5.6-sol at high effort.

## The board

| run | lever | passed | which two pass | patch | regressions |
|---|---|---|---|---|---|
| baseline | verify only | 1/5 | base only | 498 | 0 |
| scope gate | smallest-diff, no-regress | 2/5 | base + file_path_multi_env | 501 | 0 |
| issue-example gate | enumerated red-to-green | 2/5 | base + file_path_multi_env | 436 | 0 |
| codex (native) | native | 2/5 | base + one multi-env | 579 | 0 |
| pi (native) | native | 2/5 | base + one multi-env | 536 | 0 |

The issue-example gate lands at two of five with zero regressions, on a smaller patch than the scope gate.
It passes exactly the two tests the scope gate passes and fails exactly the same three, the py-module-path cluster.
Four levers now converge on the identical two of five and the identical three failures.

## The lever fired, and it handed the model the right checklist

This is not a plumbing null result.
The extraction ran and its checklist reached the model, carried through ten of the run's requests.
And the checklist was correct on the cases that grade.
Distilled from the issue text alone, it included "Loading settings for multiple environments with settings_loader must load every specified environment" and "Building an environment list from multiple environments with build_env_list must include every specified environment", naming both graded functions and the exact multi-env behavior the hidden tests check, alongside the identifier-parameter case the loaders also needed.
The harness put an explicit, correct, per-case contract in front of the model, pointed at the precise functions the gold patch changes.

And the model still wrote the multi-env logic wrong on three of five.

## Reading: an enumerated contract is not an algorithm

This closes the question the scope-gate run opened.
The reasoning ceiling is real, and it is not a matter of the model not knowing what to build.
The context pack gave it the functions; the issue-example gate also gave it a checklist that names those functions and states, in plain terms, the behavior the failing tests demand.
With both, the model edits the right code and produces the wrong semantics for the comma-separated temporary env that must not mutate the current env and the dotted-module and slash-file path forms.
Knowing which cases to satisfy and having the code in front of you does not supply the ability to write the case correctly.
No harness lever tested here, retrieval, verification, scope, or an enumerated red-to-green contract, closes that gap, and the convergence is the evidence: four independent levers and two native harnesses, some pack-restricted and some with full file access, all stop at the same two of five and the same three failures.

## What this settles

The scope gate stays the lever worth keeping: it beat the baseline with zero regressions and is the right default for the sprawl-and-regress failure mode.
The issue-example gate is a clean, general, untailored sub-flow that matches it on a smaller patch and earns its place as an A/B knob, but on this task it does not break the ceiling.
Reaching five of five on dynaconf-1225 does not need another harness lever; the measured evidence says it needs a materially stronger reasoning step on the multi-env loader semantics.
That is the honest landing point: two of five, banked, with the remaining three established across four levers and two native harnesses as a model-capability ceiling rather than a harness gap.
Costs are token volumes; the model was served over the unmetered subscription bridge, so the dollar cost is unknown, never zero.
The run is on the public trace index under dynaconf__dynaconf-1225.
