---
title: "The new Open Interpreter is a Codex fork, and it costs about 2x tomo's code-as-action"
linkTitle: "new OI is a Codex fork, ~2x the cost"
description: "Open Interpreter's Python code-as-action loop ended at 0.4.2, and its main line is now a Rust program, a fork of OpenAI Codex tuned for low-cost models. The lab's openinterpreter column now tracks that rewrite, release rust-v0.0.25, and it ran head to head against tomo's two engines on the same 14 scenarios, same model, same proxy, pass@1. New OI passes all 14, and it is leaner than tomo's own Codex-style engine, but on the 13 tasks both clear it spends about 2.4x the tokens of tomo's code-as-action engine. The one task new OI wins is the free model quitting a long checklist early plus one real, fixable harness gap, not an architecture deficit. So the gate to rewrite tomo's oi engine from the Codex fork does not open, and code-as-action stays."
date: 2026-07-16T15:14:00+07:00
---

Open Interpreter was rewritten.
The Python code-as-action loop ended at 0.4.2, and main is now a Rust program, a fork of OpenAI Codex tuned for low-cost models that emulates the agent harness each model was post-trained on.
I pointed the lab's openinterpreter column at that rewrite, release rust-v0.0.25, and ran it against tomo's two engines on the same scenarios, same model, same proxy, one paid attempt each.
The question the plan set was simple: if the new version performs better, rewrite tomo's oi engine from it.

## Setup

Fourteen core scenarios, deepseek-v4-flash-free through the opencode zen proxy, pass@1, the container itself the only sandbox.
Three engines run the identical grid: the new OI Codex fork, tomo's Codex-style `cx` engine, and tomo's code-as-action `oi` engine.
Totals are the sum of each run's reported tokens.

The new OI column drives the self-contained release bundle through a codex-style config routed at the lab proxy:

    interpreter exec --config config.toml <scenario prompt>

tomo's two engines run through the lab against the same proxy:

    lab bench --engine cx --model deepseek-v4-flash-free --grade --out /tmp/cx
    lab bench --engine oi --model deepseek-v4-flash-free --grade --out /tmp/oi

## The result

| Engine | Shape | Pass | Total tokens |
|---|---|---|---|
| new OI (Codex fork, rust-v0.0.25) | structured tools | 14/14 | ~238,700 |
| tomo-cx | Codex-style, structured tools | 14/14 | ~280,600 |
| tomo-oi | code-as-action | 13/14 | ~99,800 |

## tomo-oi is the leanest by a wide margin

Read the two facts that matter off that table.

First, tomo-oi is far the leanest.
It runs at about 2.4x under new OI and 2.8x under tomo-cx, at nearly the same pass rate.
Code-as-action carries no tool schema and no structured-call scaffolding, so the cheap model writes one fenced block instead of function-call JSON, and the transcript stays small.

Second, new OI is actually leaner than tomo-cx at the same 14/14.
That is a genuine signal that the Codex harness is a solid structured-tools loop, better than tomo's own cx on this set.
But tomo already competes with cx on the structured side and with oi on the code side.
New OI beats the former and loses to the latter.

## The one task new OI wins, and why

The hardest scenario is 13-release-fix: read a spec, fetch a JSON file over HTTP, write a new Go source file that exposes a function the rest of the program calls, fix a bug in another file, build, run to produce a report, and pass the test suite.
New OI clears it.
tomo-oi missed it three times, three different ways, on the free model.

The first miss was structural.
The model "wrote" the new source file by pasting its contents in a ```go fence.
Only python and shell blocks execute in code-as-action, so that block ran nothing, the file never landed, and the build failed on the undefined name the file was meant to define.

The other two were the cheap model quitting a long checklist early.
Once it wrote the files correctly with heredocs and fixed the bug, but ended the turn without running the program, so the report was never produced.
Once it rewrote the caller to use a function it never defined, then ended after two requests without ever running the build, so it never saw the undefined-symbol error it had just created.

Only the first is a defect in the harness.
The other two are model variance on a long checklist, not a missing rail.

## The fix: a dropped-block guard

The block runner silently drops any fenced block whose language is not python or shell.
When the model pastes a whole source file in a non-runnable fence and that is its only block, the reply has nothing to run, the turn ends, and the file is never written.

The guard closes that gap without a threshold to tune.
When a reply has no runnable block but does carry a non-runnable block whose tag names a source or config language a model would only include to create a file, the engine nudges once that only python and shell run, and a file is written with a heredoc or a python write, then loops so the model actually does it.
It fires at most once a turn.
Prose and data-display tags are deliberately excluded, so a model that ends on a sample or an illustration is not nudged.

Live, this flipped 13-release-fix from a missing file and an undefined symbol to a clean build with the file on disk.
The task still misses on the free model for the early-quit reasons above, but the structural failure is gone.
Unit tests cover the nudge, the fire-once bound, and the source-only selection.

## What I did not do, on purpose

The other two misses share a shape: the model edited files and ended the turn having run no build or test at all.
A guard for "you edited but never verified" is tempting and would likely flip this task, but it has real false-positive surface.
Several non-build scenarios legitimately edit files and run no verification-marked block, only a python block that prints the result.
Such a guard would nudge them spuriously every turn, a round of waste each, which is exactly the governor over-tuning the standing rules warn against.
verify-to-green already catches the dangerous case, ending on a red check.
Ending on no check, when there may be nothing to check, is not worth a rail that fires across half the suite.
Noted as a candidate, not shipped.

## Lessons

- The gate does not open. New OI passes 14/14 and beats tomo-cx, but on total tokens to complete it costs about 2.4x tomo-oi. Rewriting engine/oi from a Codex fork would duplicate the engine tomo already has in cx and throw away the code-as-action cost win.
- Code-as-action is the cost lever. No tool schema and no structured-call scaffolding means the cheap model writes one fenced block, not function-call JSON, and the transcript stays small. That is where the 2.4x and 2.8x gaps come from.
- The rival is a good structured-tools loop. New OI leaner than tomo-cx at the same pass rate is a real signal, and the thing worth taking from it is its preventive prompt discipline, not its architecture.
- One benchmark loss was a fixable rail, not a wall. The dropped-block guard closes the harness gap the grid surfaced, and it is general, not tuned to this task.
- Do not build the tempting guard. An "edited but never verified" rail would flip one task and waste a round across half the suite. verify-to-green already handles the dangerous case.

## Reproduce

1. Build the lab against the local tomo checkout: `go build -o /tmp/lab ./cmd/lab`.
2. Install the new OI release bundle, rust-v0.0.25, and point its config.toml at the lab proxy so it routes the same deepseek-v4-flash-free model as tomo.
3. Source the proxy key before any run: the free model is served through the opencode zen proxy, so the key must be in the environment first.
4. Run the 14-scenario grid three times, once per engine: new OI via `interpreter exec`, then tomo with `--engine cx` and `--engine oi`, each with `--grade`.
5. Read each run's summary for the per-task pass flag and reported tokens, and sum the tokens per engine to reproduce the table.
