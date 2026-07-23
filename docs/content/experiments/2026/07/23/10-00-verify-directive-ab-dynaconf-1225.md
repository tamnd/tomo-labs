---
title: "The verify directive on dynaconf-1225: off breaks the file at zero of five, on holds it at two of five with no regressions"
linkTitle: "verify directive A/B on dynaconf-1225"
description: "A pass@1 A/B of the executing-check directive in tomo's oi engine, holding gpt-5.6-luna and the pin-2c575dd overlay fixed and toggling only TOMO_OI_VERIFY. Off, the model ships a 2134-line sprawl that fails to import, so all five named tests come back MISSING and all four baseline-green tests regress: zero of five. On, the same model runs the suite instead of eyeballing a compile pass, ships a 315-line diff, greens two of the five, and regresses none: two of five with the four pass-to-pass tests intact, at 42 percent fewer tokens. The directive helps and does not suffice, which is exactly what the spec predicted and what tells us the gated version is the next build."
date: 2026-07-23T10:00:00+07:00
---

This is the first A/B in the spec 2109 campaign, the harness-lever plan that came out of the three-way trace audit on `dynaconf__dynaconf-1225`.
The audit's reading was that the gap to a full pass on this task is not a model-capability gap but a retrieval-and-verification gap: the pack already puts the right file in front of the model, and the model still fails, either by never running the test that would show it what is wrong or by running the wrong check and stopping.
The [tomo-oi luna note]({{< relref "23-00-swebench-live-tomo-oi-gpt56luna-dynaconf-1225" >}}) was the sharpest case: the pack found `loaders/__init__.py`, the model rewrote it too broadly, left a dangling name, and the oi loop's own `ast.parse` check waved the `NameError` through because a `NameError` is a runtime error, not a syntax error.
The verify directive is the opt-in first cut at closing that gap: a line in the system prompt that tells the model to execute its change, not just parse it, before it finishes.
This run measures whether the directive alone is worth its place.

## The A/B

Everything is held fixed except one environment variable.
Same task, same faithful per-instance container, same no-egress internal network, same offline grade.
Same model, `gpt-5.6-luna` at effort high through the subscription bridge.
Same overlay, tomo pin `2c575dd`, which carries the call-edge context-pack expansion and the multi-language verify directive, so the directive is present in both arms and only its arming differs.
Same round cap, `TOMO_MAX_ROUNDS=12`, and the same LSP-backed pack through pyright.
The only difference is `TOMO_OI_VERIFY`: off for one arm, on for the other, one run each, pass@1.

The task has five `FAIL_TO_PASS` tests and four `PASS_TO_PASS` tests, all in `tests/test_settings_loader.py`.
A pass is all five red tests green with the four green tests still green.
The bar the whole spec is measured against is five of five under a free model with no human naming the file.

## The result

| metric | verify off | verify on |
| --- | --- | --- |
| resolved | False | False |
| FAIL_TO_PASS green | 0 of 5 | 2 of 5 |
| PASS_TO_PASS regressions | 4 of 4 broke | 0 |
| patch lines | 2134 | 315 |
| model calls | 10 | 7 |
| prompt tokens | 622,457 | 327,149 |
| completion tokens | 133,426 | 115,539 |
| reasoning tokens | 72,462 | 68,107 |
| total tokens | 755,883 | 442,688 |
| wall clock | 46:55 | 1:02:44 |

The dollar cost is subscription-flat on the gpt-5.6-luna bridge, which meters no per-token price and returns no cache-hit count, so neither arm has a metered dollar figure.
That is not zero, it is unmetered: the token columns are the real cost comparison, and on tokens the winning arm is 42 percent cheaper.

## What each arm did

Verify off produced the failure mode the audit named, in its worst form.
The model rewrote a dozen loaders in one 2134-line sprawl, and the result failed to import, so the five named tests did not come back red, they came back MISSING: they never collected.
A test that cannot be collected cannot be graded, and a change that breaks import takes the four baseline-green tests down with it, so this arm did not just fail to fix the bug, it regressed everything that worked.
This is the zero-of-five-with-regressions outcome from the original luna note, reproduced.

Verify on produced a different shape of run entirely.
The model ran the suite instead of trusting a compile pass, and the trace shows it: counting the code the assistant emitted across the final request, the on arm ran 123 pytest or unittest invocations against the off arm's 70, while its weak syntax-only checks, the `ast.parse` and `py_compile` lines, barely moved, 67 against 55.
The directive converted deliberation into execution.
The patch that came out was a tight 315 lines, a seventh of the off arm's, and it greened `test_load_using_settings_loader` and `test_load_using_settings_loader_with_one_env_named_file_file_path_multi_env` without breaking any of the four pass-to-pass tests.

## What it did not do

Three tests stayed red, and they are the same three the audit flagged as the hard requirement: the module-path family, `test_load_using_settings_loader_with_multi_temporary_env`, `..._with_one_env_named_file_module_path`, and `..._with_one_env_named_file_module_path_multi_env`.
These bottom out in the Python-module-path loader, the branch the model never entered in any of the three audited runs.
The directive got the model to run the tests, and running the tests showed it two of them were already satisfiable with a modest edit, but it did not force the model into the file where the other three live.
Telling a model to check is not the same as refusing to let it finish until the named tests are green.

## The reading

The directive helped and did not suffice, which is exactly the prior the spec carried into this A/B.
It helped enough to ship opt-in: on this task it strictly dominated the off arm on every axis that matters, more solved tests, zero regressions instead of four, a seventh the diff, and 42 percent fewer tokens.
There is no version of this result where you would rather run the off arm.
But it left the hard requirement untouched, and it left it untouched for the reason the audit named: a model told to check can still check the wrong thing and stop, and a prompt line cannot make the harness refuse the ending.

So the next build is not a second, louder directive.
It is the gated version: the harness refuses to end a round without an executing check, and refuses to finish while any named `FAIL_TO_PASS` test is unrun or red.
That is S2 and S3 in the workplan, and this A/B is the measurement that was waiting for them.
The directive moved the number from zero to two; the gate is what has to move it from two to five, because the gate is the thing that forces entry into the module-path branch the directive alone never reached.
