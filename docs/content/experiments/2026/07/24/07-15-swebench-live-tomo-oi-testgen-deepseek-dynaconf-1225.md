---
title: "The harness authored the test: targeting fixed, but the free model over-edited and regressed the suite"
linkTitle: "tomo-oi + testgen, deepseek-flash-free, dynaconf-1225"
description: "The previous run on this task ended with a prediction: a cheap model told to write the test first skips it and drifts to the cheapest checklist item, so move test authoring into the harness. This run does that. Before the loop, the harness makes one issue-only call, writes the returned test to the workspace, and holds the model to turning it green. On deepseek-v4-flash-free the lever fired and fixed targeting: for the first time on this task the model localized to the correct settings-loader subsystem instead of the cheap cli one-liners. It still scored zero of five, because a broad self-authored test with no regression check licensed a 196-line rewrite across all seven loaders that regressed all four pre-existing tests. The wall moved from will-not-write-the-test to over-edits-chasing-its-own-test. This is also the first run captured through tomo's canonical single-file session store."
date: 2026-07-24T07:15:00+07:00
---

Reproducibility header: tool=tomo, engine=oi (code-as-action, LSP context pack via pyright), model=deepseek-v4-flash-free (free tier via the opencode.ai/zen upstream), suite=swebench-live, task=dynaconf__dynaconf-1225, pass@1, 20 rounds, tomo build 21bd4bf.
The lever under test is the test-authoring sub-flow, TOMO_OI_TESTGEN=1.
Reproduce command:

    source ~/data/.local.env   # OPENCODE_API_KEY for the zen upstream
    OVERLAY_IMAGE=tomolab-inst-agents:dynaconf__dynaconf-1225-focus21bd4bf \
    INGEST_TOOL=tomo-oi TOMO_ENGINE=oi TOMO_OI_FOCUS=1 TOMO_OI_TESTGEN=1 \
    TOMO_LSP="pyright-langserver --stdio" LAB_MODEL=deepseek-v4-flash-free \
      bash run_task.sh dynaconf__dynaconf-1225 <image> tomo "python -m pytest tests/test_settings_loader.py -rA"

## What the last run left open

dynaconf-1225 is a thirteen-item port issue.
The graded slice is two of those items, settings_loader multi-environment and build_env_list.
The prior run on this task armed a convergence directive that killed the empty-patch failure but not the miss: the free model committed real edits, then localized to cli.py json.dumps and a redis None-prefix, the cheap one-line items, and never engaged the settings-loader behavior the hidden tests exercise.
The reading was that telling a model to write the test first is not the same as it writing the test.
A cheap model swamped by a long checklist skips the test and goes straight to the easy edits, and then it has no failing target to tell it the fix is wrong.
So the proposal was to make the test exist: author it in the harness, before the loop, from the issue text alone.

## The lever

The test-authoring sub-flow makes one call that reads only the issue and returns test code.
The harness writes it to test_tomo_repro.py in the workspace, smoke-checks that it collects, regenerates once if it does not, and then arms the reproduction gate so the model must turn the already-failing test green.
It names no file or symbol from the issue, so the test is the model's own reproduction and not a copy of the hidden suite.
It fails soft: an empty issue, a failed call, or a file that will not collect after one retry leaves the loop unchanged.

## What fired

The sub-flow ran.
The trace opens with `[testgen] wrote reproduction tests to test_tomo_repro.py`, and the authored test is test_settings_loader_multiple_environments, on the graded slice rather than off it.
Targeting changed.
Where the prior run touched cli.py and redis, this run edited dynaconf/base.py and every loader: loaders/__init__.py, env_loader.py, ini_loader.py, json_loader.py, toml_loader.py, yaml_loader.py, and validator.py.
That is the correct subsystem.
Making the test exist did what the last run predicted: it forced breadth of fix onto the settings-loader machinery instead of letting the model skim the checklist for the cheapest edit.

## Why it still scored zero

The model over-edited.
The graded suite reports PASS_TO_PASS four total, four not passing: the 196-line patch regressed all four pre-existing tests.
Unconstrained convergence on a self-authored test turned out worse than the drift it replaced.
The model rewrote the identifier-passing signature across all seven loader entry points in one pass, chasing its own broad test, and broke behavior those loaders already had.
It never re-ran the existing suite to notice, because the only terminal condition it was given was its own authored test, and that test was still red.

The wall moved.
The prior wall was the model will not write the test, so it drifts to the cheap item.
This wall is the model writes the test and targets the right subsystem, then over-edits chasing it and regresses working behavior.
The missing capability is no longer retrieval or test-writing.
It is minimality: a cheap model handed a broad target edits broadly, and nothing in the loop checks that the existing suite still passes before it commits.

## A configuration note worth recording

The sub-flow is gated to run only when the issue-example gate is off.
A first attempt at this experiment armed both the example gate and testgen together, and the sub-flow silently did not fire; that run was the example configuration, edited cli.py, kept the suite green, and scored zero of five, indistinguishable from the earlier example-gate run except for its label.
Two mutually exclusive levers armed together buy the older one, with no error to say so.

## What this run says to build next

Add a regression guard to the reproduction gate.
Before a round counts as finished, run the pre-existing tests the change could touch and refuse to converge if any that passed now fail.
The authored test says you are not done; the regression guard says you just broke something.
A cheap model needs both signals, because on its own it optimizes the one it is given to the exclusion of the other.
Keep the authored test narrow too, one behavior per file, so turning it green does not license rewriting an entire module.

## On the trace

This is the first dynaconf-1225 run captured through tomo's canonical session store.
The published trace is the run's own session.jsonl, one append-only Session Trace Simple Format file the agent wrote as it ran, the same self-describing shape codex and claude emit, not a reassembly of wire captures.
The proxy usage log recorded two calls for a run the session shows as twenty-one assistant turns; the session file is the complete one, which is why it is now the source of truth.
