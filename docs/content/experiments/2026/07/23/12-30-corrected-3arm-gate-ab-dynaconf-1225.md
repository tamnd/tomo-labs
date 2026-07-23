---
title: "The corrected three-arm A/B on dynaconf-1225: the directive reaches the model now, and it still is not enough"
linkTitle: "3-arm gate A/B on dynaconf-1225"
description: "The re-run of the verify A/B after the plumbing was fixed, three arms at pass@1 on dynaconf__dynaconf-1225 under gpt-5.6-luna: baseline, verify directive on, and verify plus the harness-side executing-check gate. The directive text now reaches the model in the on arms and is absent in the baseline, which is the fix the invalid first run lacked. The baseline burned the most tokens and produced an empty patch, both on-arms landed a real patch at one of five, and the gate cut syntactic checks and total tokens without lifting solve rate. No arm reached five of five, because none ran the dotted-module-path branch that grades, which is the terminal condition the next lever has to force. Costs are token volumes with cache detail, the model was served over an unmetered subscription bridge so the dollar cost is unknown, never zero."
date: 2026-07-23T12:30:00+07:00
---

This is the corrected version of the A/B whose first run was [retracted earlier today](../10-00-verify-directive-ab-dynaconf-1225/).
That run was invalid because the harness wrapper never forwarded `TOMO_OI_VERIFY` into the agent container, so both arms ran with the directive off and the different scores were variance, not treatment.
The plumbing is fixed, and this is the re-run: three arms, one lever changed at a time, each graded, each checked in the trace for whether its mechanism actually fired.

## The three arms

Everything is held fixed except the two toggles.
Same task, `dynaconf__dynaconf-1225`, same model, `gpt-5.6-luna`, same overlay image pinned at tomo commit `c06f187`, same round cap of twelve, same pyright LSP context pack.

- Baseline: `TOMO_OI_VERIFY=0`, `TOMO_OI_GATE=0`. The stock oi loop.
- Verify: `TOMO_OI_VERIFY=1`, `TOMO_OI_GATE=0`. The prompt directive that outlaws parse-only checks and demands an executing one.
- Gate: `TOMO_OI_VERIFY=1`, `TOMO_OI_GATE=1`. The directive plus the harness-side gate that refuses to accept a round as terminal when its only check was syntactic.

## What the trace proves, before the scores

The point of the retraction was that a score means nothing until the trace confirms the mechanism was present, so that is the first table.

| Arm | directive text in `instructions` | patch produced | grades |
|-----|----------------------------------|----------------|--------|
| Baseline | absent | empty, zero lines | 0 of 5 |
| Verify | present | 441 lines | 1 of 5 |
| Gate | present | 472 lines | 1 of 5 |

The directive text is absent in the baseline and present in both on-arms.
That is the single fact the first run got wrong, and it is now right: the toggle reaches the model.
No arm regressed a baseline-green test this time (`p2p_regressions=0` for all three), unlike the audited run this campaign started from.

## The scores, read honestly

The baseline scored zero of five, but it scored zero by producing no patch at all: it wandered the repository for ten model calls and never committed an edit.
That is a degenerate baseline, a no-op run, not a wrong fix, so "zero versus one" is not a clean solve-rate comparison the way it would be if the baseline had shipped a wrong patch.
Both on-arms landed a real, sizable patch and each greened exactly one of the five graded tests, the same one an earlier pass@1 reached.
Neither reached five of five.

The gap is the same one the audit named.
None of the three arms ran the dotted-module-path branch that the three failing tests exercise, because those tests are added to the checkout only at grade time, so no arm could run the test that grades even if it wanted to.
The directive makes the model check its work; it does not tell the model which branch is unchecked.
That is the terminal-condition lever, and it is why the next build is a test-runner that constructs and runs the failing case rather than another prompt line.

## Cost, with cache detail

The model was served over the subscription bridge, which does not meter per-token dollars, so the dollar cost of every arm is unknown.
It is not zero.
What is measured is token volume and the cache-read share, per arm, summed across every model call in the run.

| Arm | calls | prompt | completion | reasoning | cache-read | total | cache-read share | wall |
|-----|------:|-------:|-----------:|----------:|-----------:|------:|-----------------:|-----:|
| Baseline | 10 (+2 errored) | 622,457 | 133,426 | 72,462 | 51,968 | 755,883 | 8.3% | 46:55 |
| Verify | 7 | 327,149 | 115,539 | 68,107 | 35,584 | 442,688 | 10.9% | 1:02:44 |
| Gate | 8 | 303,264 | 102,955 | 54,598 | 19,200 | 406,219 | 6.3% | 42:15 |

The reading that survives scrutiny: the baseline is the most expensive arm, not the cheapest.
It spent 756k tokens to produce nothing, because with no directive the loop kept re-reading and re-testing without converging on an edit.
The verify arm cost 42% fewer total tokens than the baseline and shipped a patch; the gate arm cost 46% fewer and shipped a patch.
So on this single run the correctness levers are also the cheaper levers, which is the opposite of the usual worry that a gate adds overhead.
Cache-read share is modest across the board, six to eleven percent, because the oi loop runs few rounds with a large fresh context each, so most prompt tokens are new rather than re-sent.

## The gate's own signal

The gate is supposed to push the model off syntactic checks, so the trace was grepped for occurrences of executing-check commands against syntactic-check commands.
The raw counts are baseline 90 syntactic, verify 69, gate 35.
These counts are confounded, the grep runs over the full replayed conversation in every request file, so a run with longer histories inflates every count, and they should be read as directional not precise.
Directionally they point the right way: the gate arm carried the fewest syntactic-check occurrences while spending the fewest tokens, which is consistent with the gate nudging the model away from `ast.parse` and toward running something.
It is not proof, and pass@1 cannot make it proof.

## The verdict

The mechanism is fixed and confirmed: the directive reaches the model, the gate fires, and neither regresses a baseline test.
The directive and gate arms are cheaper than the baseline and produce real patches where the baseline produces none.
But solve rate is one of five for both on-arms, and the missing four are the dotted-module-path cases that no arm ran, because the grading tests are not in the checkout to run.
The lever that could move this is not a bigger directive.
It is a separate test-runner flow that constructs the failing dotted-module-path load, watches it fail, and feeds that back into the main loop as the thing to make pass, which forces entry into the file the fix lives in without a human naming it.
That flow is the next build, and it is deliberately harness, not prompt: the system message does not grow, the harness runs the test and hands back the verdict.

## Caveats

Every number here is a single pass@1 sample, and the retraction is the standing proof that one sample separates nothing.
The baseline's empty-patch outcome this run against the earlier invalid run's two-thousand-line sprawl is the same configuration scoring differently twice, which is exactly the variance the campaign refuses to read as signal.
The verdict above leans on the variance-free facts, directive present or absent, patch produced or not, regression or not, and treats the pass counts and token totals as corroboration, not as the finding.
