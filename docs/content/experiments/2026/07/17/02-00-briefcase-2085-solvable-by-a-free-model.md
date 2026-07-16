---
title: "The third task, and the second a free model solves: briefcase-2085 is about converging, not diagnosing"
linkTitle: "briefcase-2085, a free model solves it"
description: "The campaign's third slice runs the free roster on briefcase-2085, a well-framed git-config bug where the issue names the failing call and even proposes the fix. This one is neither the harness's fault, as the first task was, nor a diagnosis trap, as the second was. A free model, hy3, solves it cleanly by applying the fix the issue literally suggests, editing the right file at the right line. That takes the campaign to two of fifteen solved, both free, both zero paid dollars. The instructive failure is a second free model that finds the exact same line, names the exact same problem, and then oscillates between the two candidate fixes for thirty rounds without committing, because the one test that would tell it which fix the grader wants is hidden. So the recurring thread from the previous slice returns in a milder form: a hidden failing test removes the signal a model needs to break a tie, and the model that passes is the one that trusts the issue's own wording."
date: 2026-07-17T02:00:00+07:00
---

The first task in this campaign was the harness's fault, and fixing the engine gave a free model a pass.
The second was a diagnosis trap that every model, free and flagship, walked into identically.
This third one is a different shape again, and it is the friendliest so far: a well-framed bug that a free model solves on its own.

The task is beeware briefcase-2085.
The roster and protocol are the same as before: the free zen models, one graded pass@1 each with tomo-oi, a thirty-round cap, the network off, graded by the task's own check with future git history stripped.

The short version: hy3 solves it, cleanly, by doing exactly what the issue suggests.
That is two of fifteen now, both free, both at no cost.

## The task

Briefcase caches a project template as a git clone and, before fetching, points the clone's `origin` remote at the template url.
It does that with `remote.set_url(new_url=template, old_url=remote.url)`.
If the user's git config rewrites urls with an `insteadOf` rule, the old url git computes no longer matches, and the call fails with exit 128: "No such URL found."
The bug report is a good one.
It quotes the failing command, quotes git's exact error, and proposes two fixes: catch the error and carry on, or drop the optional old url and just set the new one.

The gold fix is the second option, one line in `src/briefcase/commands/base.py`: drop `old_url` so the call is `remote.set_url(new_url=template)`.

## The sweep

| Model | Rounds | Actions | Input tokens | Result | Edited | Cause |
|---|---|---|---|---|---|---|
| hy3-free | 7 | 185 | 305.6k | pass | base.py + changelog | dropped old_url, as the issue suggested |
| nemotron-3-ultra-free | 30 | 30 | 389.2k | fail | base.py | right line, oscillated between the two fixes, hit the cap |
| mimo-v2.5-free | 4 | 0 | 0 | abort | | free-tier quota, 429 |
| north-mini-code-free | 7 | 2 | 9.1k | abort | | transient 400 from the provider, retried and aborted again |
| deepseek-v4-flash-free | | | | untested | | quota already spent this window |

One pass, one clean fail, two aborts, one untested.
The cheapest, and only, pass is hy3 at no cost and 305.6k input tokens.
The two aborts are honest infra, not task failures: mimo hit the free tier's usage limit, and north hit a transient provider error twice in a row.
deepseek was untested because an earlier health probe this window spent its quota.
A healthy-window re-run of all three is queued.

## Why hy3 passed

hy3 read the issue, went to `update_cookiecutter_cache`, and applied the issue's own second suggestion: drop the old url so the call becomes `remote.set_url(new_url=template)`.
The graded tests assert exactly that new call signature, so they went green.
There is a nice wrinkle: partway through, hy3 ran the visible suite and saw tests that appeared to assert the old behavior, and it nearly talked itself out of the fix.
But the tests that grade the task are added at grade time and reward the new signature, and hy3's edit matched them, so it passed anyway.

## Why nemotron did not

nemotron is the instructive failure, because it did not misdiagnose anything.
It found the exact line and stated the exact problem in its own words: the call at line 1017 passes an old url that git cannot resolve.
Then it could not commit.
Across thirty rounds it wrote the bare `new_url=template` form and the original two-argument form more than once each, and it ran out at the round cap with a graded state identical to doing nothing: four failing tests, eleven passing.
It had the answer in hand and thrashed between it and the status quo.

## The hidden test is why the tie is hard

The issue offers two fixes, and only one satisfies the grader, because the hidden tests assert the new `set_url` signature rather than a caught-and-warned error.
But those tests are added at grade time and are not in the checkout.
So a model weighing the two options has nothing red in front of it to say which one the grader wants.
hy3 broke the tie by trusting the issue's literal wording and dropping the old url.
nemotron had no such anchor and oscillated.

This is the same limitation that neutered verify-to-green on the previous task, in a milder form.
There, the hidden test hid the whole bug and every model missed it.
Here, the right answer is reachable straight from the issue, so a capable model gets there, but the hidden test still denies the loop a signal to correct a wrong guess.

## Why there is no harness fix to ship

hy3 already solves the task, so nothing in the harness is broken.
nemotron's failure is convergence thrash, and the only lever that would break its tie is to tell it which of the two fixes the grader accepts, which is the answer.
So the engine stays as it is.
The general, non-leaking discipline that would most likely have helped is the reproduce-first idea from the previous slice: build the issue's own reproduction, the `insteadOf` git config plus the set_url call, confirm the exit-128 failure, then prefer the edit that makes the reproduction succeed.
On this task that points at dropping the old url, because that is what makes the reproduced command run.
It is worth a measured A/B eventually, but it stays a candidate, not a shipped change.

## Three shapes so far

- gitingest-94 was harness-bound. Free models could solve it, and the failures were engine bugs; fixing them gave a free pass.
- sqllineage-661 was a diagnosis trap. Every model edited the wrong file because the issue title mis-framed the bug, and there was no fair fix.
- briefcase-2085 is convergence-bound. The fix is obvious from a well-framed issue, one free model converges and passes, another finds the line but cannot commit because the discriminating test is hidden.

The thread joining the last two is the hidden failing test.
It removes the one signal that lets the verify-to-green loop correct a wrong guess.
Worth tagging each remaining task by whether its failing behavior is visible or hidden, because that predicts how much the loop can help.

## Lessons

- A well-framed issue does a lot of the work. When the report names the failing call and proposes the fix, the task turns on whether the model trusts that wording and commits to it.
- Convergence is its own failure mode, distinct from diagnosis. nemotron found the exact line and still failed, by oscillating between two plausible edits it could not choose between.
- A hidden failing test hurts even when the answer is reachable. It cannot hide a bug the issue spells out, but it can still deny the loop the signal that would break a tie, so the model that trusts the issue wins and the model that hedges loses.

## Reproduce

1. Build the lab against the current tomo and run the committed sweep: `scripts/campaign_sweep.sh beeware__briefcase-2085`.
2. A passing run edits `src/briefcase/commands/base.py` and drops the optional old url from the remote update, matching the issue's second suggestion. Read the edited files in each run's summary to see which models converged there.
3. A run whose summary shows a non-empty error and no pass is an abort on the free tier's quota or a transient provider error, not a task failure; re-run it in a healthy window.
