---
title: "The convergence directive kills the empty patch on a free model, but a cheap model still will not write the test it needs to verify its own fix"
linkTitle: "Focus lands the patch, test-writing is the next wall"
description: "Two runs bracketed the model range on dynaconf-1225 and failed identically: a flagship model at maximum reasoning effort and a free deepseek model both read the thirteen-item port issue as one wide surface, explored every file, and finished with an empty patch, zero of five. The gap was convergence, not retrieval. A convergence directive (TOMO_OI_FOCUS) tells the model that a multi-item issue is graded item by item and to land one at a time, writing each item's test first, before moving on. On the free deepseek-v4-flash it works: the empty patch is gone, sixty-six real lines shipped. It still scores zero of five, and the trace says why. The model wrote no test at all, spraying a page of intent-to-read prose and then saying it would just make the edits directly, so the reproduction gate had nothing to bind and localization drifted to the cheap one-line items, cli json and a redis prefix, never the graded settings-loader multi-env slice. Telling a model to write the test first is not the same as it writing one. The next lever moves test authoring into the harness. Cost is free-gateway tokens, dollar cost zero on the free tier, never assumed zero elsewhere."
date: 2026-07-24T10:50:00+07:00
---

Two runs bracketed the model range on dynaconf-1225 and failed the same way.
The issue is a thirteen-item port, and the graded slice is two of those items, settings_loader multi-env and build_env_list.
A flagship model at maximum reasoning effort read all thirteen as one surface, explored every file, and finished with an empty patch, zero of five.
The free deepseek-v4-flash model did the same, zero of five, no source edit.
A reproduction gate cannot rescue that run, because a model that never tries to finish just hits the round cap with nothing committed.
So the gap was not retrieval and not test-writing ability.
It was convergence, landing a committed fix before the budget runs out.

## The convergence directive

TOMO_OI_FOCUS adds one addendum to the oi system prompt.
When an issue lists several things to change, they are graded independently, so land them one at a time.
Pick the single most concrete item, write its small focused test, make that test pass in the source, confirm green, and only then move on.
It names the anti-pattern directly: a turn that only read code and ran the existing suite produced nothing gradeable, and a budget spent with no source edit scores zero no matter how much you understood.
It names no file or symbol from the issue, so it is general and not tailored to the hidden tests.
It ships in tomo v0.2.9, off by default, meant to pair with the issue-example gate.

## The run

deepseek-v4-flash-free through the zen gateway, examples gate and focus directive both on, pass@1, twenty rounds, tomo v0.2.9.

Result: zero of five, but the patch is sixty-six lines, not empty.

The convergence lever did its job at the free tier.
The empty-patch failure is gone.
The model committed real edits instead of exploring until the cap.

## Why it still failed

Two things went wrong, and both point at one missing capability.

The model wrote no test.
The examples gate hands it a per-item checklist and the focus directive tells it to write each item's test first, and the trace shows it emitting roughly two hundred fifty lines of "we'll read the relevant files" prose, then saying out loud "Wait, alternative approach: I'll just make the targeted edits directly," and skipping the test step.
With no test file on disk the reproduction gate had nothing to bind to, so red-to-green was never enforced and the round cap shipped whatever existed.

It localized to the wrong items.
The sixty-six lines touch cli.py json.dumps, redis_loader None-prefix, and base.py populate_obj underscore-skipping.
None of those are the graded settings_loader multi-env slice.
build_env_list was touched only by a variable rename at one call site.
It took the cheap one-line items off the checklist and never engaged the hard behavior the graded tests exercise.

## The finding

Telling a model to write the test first is not the same as it writing the test.
A cheap model swamped by a long checklist skips the test and goes straight to the easy edits, and then it has no way to know its fix is wrong.
Without a failing test it cannot verify, and without verification it drifts to whichever item is cheapest to touch.
The instruction is not enough.
The harness has to make the test exist.

## Next

Move test authoring out of the model's hands and into the harness as a sub-flow.
Before the loop, make one issue-only call that returns actual test code for the enumerated items, write those tests to a fresh file in the workspace, and let the finish gate require them green.
Breadth of authored tests forces breadth of fixes: with the settings_loader test red on disk from round zero, the model cannot skip it for the cli one-liner.
The call reads the issue text alone, never the graded suite, so the tests are the model's own reproduction and not a copy of the hidden ones.
Cache the stable issue-plus-system prefix so the extra call is close to free.
