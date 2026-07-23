---
title: "Correction: the first verify-directive A/B on dynaconf-1225 was invalid, the toggle never reached the model"
linkTitle: "verify directive A/B on dynaconf-1225 (corrected)"
description: "A retraction. The first pass@1 A/B of tomo's verify directive on dynaconf__dynaconf-1225 reported zero of five with the directive off against two of five with it on, and read that as the directive working. It was not the directive. The harness wrapper never forwarded TOMO_OI_VERIFY into the agent container, so both arms ran with the directive off: two identical configurations that happened to score differently at pass@1, which is variance, not a treatment effect. The trace proves it, the directive's own text appears in zero requests across both arms and both carry the identical directive-off system prompt. The plumbing is fixed and a corrected three-arm A/B is running; this note records the mistake and how it was caught so the correction is on the public record before the real numbers land."
date: 2026-07-23T10:00:00+07:00
---

This note replaces an earlier version that claimed the verify directive moved `dynaconf__dynaconf-1225` from zero of five to two of five.
That claim was wrong, and the reason it was wrong is worth writing down, because it is the exact failure the spec-2109 campaign is meant to guard against: a reasonable-sounding change that moves the number without moving the mechanism.

## What was claimed

The first A/B held everything fixed except `TOMO_OI_VERIFY`, the environment variable that arms the directive telling the model to execute its change instead of only parsing it.
Off, the run resolved False at zero of five with all four baseline tests regressed.
On, the run resolved False at two of five with no regressions and a much smaller diff.
The write-up read that gap as the directive converting deliberation into execution.

## Why it was invalid

The toggle never reached the model.
The harness wrapper `run_sub.sh` builds the `docker run` that launches the agent, and it did not pass `TOMO_OI_VERIFY` through into the container.
So the agent process never saw the variable in either arm, and both arms ran with the directive off.
The two runs were the same configuration executed twice.

The proof is in the traces, not in the reasoning.
The directive's own text, the line "Verification is not optional," appears in zero request files across both arms.
Both arms carry an identical `instructions` block of length 2158, which is the directive-off system prompt byte for byte.
Two identical configurations were run once each and scored zero of five and two of five, which is the definition of pass@1 run-to-run variance, not a treatment effect.

The mistake was catchable only one way: by grepping the actual request the model received for the directive text, rather than inferring from the score that the directive must have been on.
Scoring an A/B without confirming the treatment reached the subject is how a null result gets published as a win.
That is the specific trap, and this run walked into it.

## What is fixed and what runs now

`run_sub.sh` now forwards both `TOMO_OI_VERIFY` and `TOMO_OI_GATE` into the container, confirmed by grepping the directive text back into the on-arm request.
The corrected experiment is three arms at tomo pin `c06f187`, one pass@1 run each, same `gpt-5.6-luna` at effort high, same per-instance faithful container, same no-egress network and offline grade, same twelve-round cap and pyright-backed pack:

- baseline: directive off, gate off, the true control
- directive: directive on, gate off, the prompt line now actually applied
- gate: directive on, gate on, the directive plus the harness-side executing-check gate from spec 2109 S2

Each arm is checked in-trace for whether its mechanism fired, not only for its score: the directive text present in `instructions`, the gate nudge "have not run it" emitted when the model tries to finish on a weak check, and the model's final check an executing test run rather than an `ast.parse`.
Because a single pass@1 run cannot separate a real effect from variance, the corrected verdict will lead with that variance-free trace evidence and treat the pass count as corroboration.

## The one thing that still stands

The reason the gate is being built at all does not depend on this A/B, because it rests on the earlier three-way trace audit, not on the invalid run.
The audited tomo-oi run edited a dozen loaders, ran an `ast.parse` that a `NameError` sails straight through, and finished zero of five with every baseline test regressed because the edit broke import.
A prose directive cannot prevent that, because the model believed it had already checked.
Only a harness that refuses to end the turn until an executing check runs green can, and that is the gate the corrected A/B measures.
The real numbers replace the table that used to live here as soon as the three arms grade.
