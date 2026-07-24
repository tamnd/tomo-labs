---
title: "Stacking the harness levers regresses on dynaconf-1225, and the strongest available model is already in play: the ceiling is a model wall"
linkTitle: "Stacking levers regresses, model wall"
description: "Scope and issue-example each landed a single lever at two of five on dynaconf-1225, both zero-regression, both stalling on the same three py-module-path cases. Two questions remained before calling the harness-lever search exhausted: do the levers compose to more than two of five, and is there a stronger model the subscription can drive. This run answers both, and the answer to each is no. Scope plus examples plus verify armed together lands at one of five, below either lever alone, dropping even the trivial base test, because three simultaneous gates compete for a bounded round budget and net-subtract from the fix. And the bridge already runs sol at effort high, the flagship model at the strongest effort the ChatGPT subscription can serve. Four levers cap at two of five, their combination regresses to one, two native full-access harnesses cap at the same two of five, and the strongest model is already in play. Five of five on this task is not reachable with gpt-5.6-sol; it needs a materially stronger reasoning model than the subscription can serve. Costs are token volumes over the unmetered subscription bridge, dollar cost unknown, never zero."
date: 2026-07-24T10:00:00+07:00
---

The scope gate and the issue-example gate each landed a single lever at two of five on dynaconf-1225, both zero-regression, both stalling on the identical three py-module-path cases.
Before calling the harness-lever search exhausted, two questions were open: do the levers compose to more than two of five, and is there a stronger model or effort the subscription can drive.
This run answers both, and each answer is no.

## The combined run

Scope, examples, and verify were armed together, sol high, pass@1, round cap 14, tomo v0.2.8.
The hypothesis was mild orthogonality: scope holds the diff small and no-regress, examples supplies per-case red-to-green targets, verify outlaws parse-only checks, and if the three constrain different failure modes the run might clear more than two.

| run | levers | passed | which pass | patch | regressions |
|---|---|---|---|---|---|
| baseline | verify | 1/5 | base | 498 | 0 |
| scope | scope+verify | 2/5 | base + file_path_multi_env | 501 | 0 |
| examples | examples+verify | 2/5 | base + file_path_multi_env | 436 | 0 |
| combined | scope+examples+verify | 1/5 | file_path_multi_env only | 455 | 0 |

Combined lands at one of five, below either lever alone.
It drops the trivial base test that passes with no patch at all and that both single-lever runs keep, while carrying zero pass-to-pass regressions: it did not break the wider suite, it broke the graded base case.

## The levers are not additive, they compete for the round budget

Three simultaneous gates do not stack their wins, they stack their demands.
Under a fixed round cap the model spends its turns satisfying scope's smallest-diff pressure, examples' per-case obligations, and verify's execute-not-parse rule at once, and in a bounded loop attention paid to the gates is attention taken from the fix.
The result is a diff that placates the gates and breaks the base case.
This is the same shape as the earlier reproduction-gate regression: a harness that asks for more ceremony under a budget cap can net-subtract from a model already at its reasoning limit on the actual problem.
The single-lever runs win precisely because each asks for one thing and leaves the rest of the budget for the fix.

## The model side is already maxed

The subscription bridge pins a model and a reasoning effort against the ChatGPT codex backend, which only answers the gpt-5.x models a ChatGPT plan ships.
In the bridge's own terms sol is the flagship the codex subscription can pick, and effort runs minimal, low, medium, high.
Every run in this whole arc was sol at high, the strongest model at the strongest effort the subscription can drive.
There is no higher tier to route to on this account.

## The honest verdict

The search is now genuinely exhausted.
Four independent harness levers each cap at two of five, their combination regresses to one, two native full-access harnesses cap at the same two of five on the same three failures, and the strongest available model and effort are already the ones being run.
No lever, no combination, and no reachable model clears the three py-module-path cases.
The gap is the model's reasoning about the comma-separated temporary env that must not mutate the current env and the dotted-module and slash-file companion-module path forms, and no wrapper supplies an algorithm the model does not have.

Five of five on dynaconf-1225 is not reachable with gpt-5.6-sol.
It needs a materially stronger reasoning model than the subscription can serve.
That is the finding the adaptive-per-model-harness direction names directly: the harness cannot substitute for model capability on this task, and honest routing would send this instance to a stronger model, which is not available on this account.
The banked landing point is two of five with the scope gate, matching the best native harnesses, zero regressions.
Costs are token volumes; the model was served over the unmetered subscription bridge, so the dollar cost is unknown, never zero.
The run is on the public trace index under dynaconf__dynaconf-1225.
