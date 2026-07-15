---
title: "What a run costs: deepseek and gpt-5.6 in the same tomo harness"
linkTitle: "dynaconf cost and caching, deepseek vs gpt-5.6"
description: "The lab now prices every probe run at list rate and breaks out the prefix-cached share, so a free run still shows what it would cost and stays comparable to a paid one. Run the identical tomo cx-offline harness on dynaconf-1225 with a cheap model and an expensive one and two things fall out. The cheap deepseek model never commits to an edit inside its budget while gpt-5.6 grounds and then edits the right gold file, so the harness is sound and the convergence gap is the model. And the same turn costs 180x more on gpt-5.6, partly the model rate and partly a caching hole: the deepseek proxy serves 81 percent of the resent history from its prefix cache while the codex bridge serves zero, so gpt-5.6 pays full price for a history it resends every round."
date: 2026-07-15T12:35:00+07:00
---

This run is two questions at once, asked with one new tool.

The tool is cost reporting in `lab probe`.
Every probe now prices its run at the model's published list rate, the free deepseek proxy included, and breaks out the share of the prompt the provider served from its prefix cache.
A free run still shows what it would cost, so a cheap run and a paid one land on the same axis.

The two questions are the ones that axis lets you ask.
First, is tomo's harness what stands between a model and the fix, or is the gap the model?
Second, what does a turn actually cost, and where does the cost go?

## Setup

The task is `dynaconf__dynaconf-1225` from SWE-bench-Live, run offline so no model can retrieve the answer.
Note [0027](/experiments/) covers why every model fails this instance on the merits: the graded slice is five `settings_loader` tests, but passing them means reproducing an unseen seventeen file pull request, so the pass rate is not the interesting number here.
The interesting numbers are behaviour and cost, and those the harness measures the same for everyone.

Every run below is the same harness: tomo's `cx-offline` engine, the container's environment pre-built so no run wastes its budget on `pip install`, and a hard round cap so an A/B ends in seconds rather than minutes.

    # cheap model, free deepseek proxy, base prompt
    lab probe dynaconf__dynaconf-1225 \
      --engine cx-offline --prep-env --max-rounds 16 --grade \
      --out /tmp/runA

    # same, with a prompt variant that pushes an earlier reproduction
    lab probe dynaconf__dynaconf-1225 \
      --engine cx-offline --prep-env --max-rounds 16 --grade \
      --system-file variant-converge.md --out /tmp/runB

The expensive model runs through the codex bridge, which lets a chat-completions tool drive the ChatGPT subscription's responses backend on the user's own authorised access:

    lab bridge --model gpt-5.6-sol --effort high --port 8790 &

    lab probe dynaconf__dynaconf-1225 \
      --engine cx-offline --prep-env --max-rounds 40 --grade \
      --model gpt-5.6-sol --base-url http://localhost:8790/v1 \
      --out /tmp/gpt-sol

`--prep-env` is the fidelity fix that makes this comparison honest.
The offline tree ships without a virtual environment, so a bare probe measures the model fighting `pip` before it can run a single test, not the model fixing the bug.
`--prep-env` builds the venv first, the way the container image does, mirroring the task's `check.sh` install, and prepends it to the turn's `PATH`.
Build artifacts from that install, the egg-info and pyc and cache files, are filtered out of the edited-file list so they never look like a fix.

## The result

| Run | Model | Rounds | First edit | Edited file | Gold hit | Graded | List cost |
|---|---|---|---|---|---|---|---|
| A | deepseek-v4-flash | 16 (cap) | never | none | no | FAIL | $0.032 |
| B | deepseek-v4-flash | 16 (cap) | never | none | no | FAIL | $0.032 |
| C | gpt-5.6-sol | 25 (cap) | ~round 22 | `dynaconf/validator.py` | yes | FAIL, 4 pass 5 fail | $3.99 |
| D | gpt-5.6-sol | 33 (stopped) | ~round 16 | `dynaconf/utils/__init__.py` | yes | FAIL, 4 pass 5 fail | $5.86 |

Runs A and B are the cheap model with two prompts.
Runs C and D are the expensive model, capped and then given room.

## The harness is sound, the convergence gap is the model

The cleanest read is the tool mix, because it says what each run spent its turn doing.

    deepseek (A):  read 13, grep 11, bash 9,  plan 1        edits 0
    deepseek (B):  bash 15, read 11, grep 9,  plan 1        edits 0
    gpt-5.6 (C):   read 12, grep 10, edit 1,  bash 1, plan 1
    gpt-5.6 (D):   read 17, grep 12, edit 1,  plan 2

Both deepseek runs ground the task the way the prompt asks, a wide grep and then reading each definition, and never leave that phase.
Sixteen rounds in, they are still reading source and re-running the passing base tests, and they have not written a line.
The transcript tail of run A is the whole story: at the round cap it reads `validator.py` one more time and says "let me look at the specific areas more carefully to understand what needs changing."

gpt-5.6 grounds too, with a comparable read and grep budget, but then it commits.
It edits `validator.py` in the capped run and `utils/__init__.py` when given room, and both are files the gold patch touches, so the strong model lands its edit inside the seventeen file target area.
It turns four in-file tests green.

So the harness is not what holds a model back.
Given a model that will commit to an edit, `cx-offline` grounds it, points it at the right file, and lets it change code and test.
The cheap model's failure is that it never reaches the commit, and that is a property of the model on this task, not of the loop around it.

The B run is the evidence that it is the model and not the prompt.
Run B adds one general sentence to the prompt, that when the existing tests already pass, that is the signal to write the missing reproduction rather than keep reading.
It moved the tool mix, more `bash` as the model ran tests and git, but it did not produce an edit.
A prompt nudge does not manufacture a commit the model will not make on its own.
The honest caveat is that the sixteen round cap sits below the round where either model first edits, gpt-5.6 first edits around round 16 to 22, so this A/B cannot see the edit transition and the prompt lever needs a higher cap to test fairly.

## The cost, and where it goes

This is where the new instrumentation earns its place.

    deepseek-v4-flash   in   392,173   cached 317,440 (81%)   cost $0.032
    gpt-5.6-sol         in 1,142,319   cached       0 ( 0%)   cost $5.865

The same task in the same harness costs 180x more on gpt-5.6.
Part of that is the model's list rate, which is real and expected.
But part of it is a caching hole that is the lab's to close, and the cost breakout is what makes it visible.

Every round of a tomo turn resends the whole conversation so far, so the input token count grows quadratically over a run.
Prefix caching is what keeps that affordable: a provider that recognises the resent prefix bills it at a fraction of the fresh rate.
The free deepseek proxy does exactly this, serving 81 percent of the input from its cache, so the quadratic resend is mostly absorbed.
The codex bridge serves zero.
It translates each chat request into a fresh responses call with no cache marker, so gpt-5.6 pays full price for 1.14M input tokens, most of which is history it already sent.

That single number, cached 0 percent, is the most actionable thing in this run.
The model rate is fixed, but the caching is not: a bridge that carried the responses API's own prompt-cache markers would cut the resent-history cost several fold, on every gpt-5.6 run, on every task.

## Lessons

- The harness clears the bar. A strong model in `cx-offline` grounds, edits the right gold file, and verifies, so the loop is not the blocker. The cheap model's "never commits to an edit" is a model-capability limit on this task, and a prompt sentence does not fix it.
- Price the free runs. Reporting list cost for the free proxy too is what put the deepseek and gpt-5.6 runs on one axis and made the 180x visible. A free run that reports `$0.00` would have hidden the comparison that matters.
- The cost breakout found a bug. Cached-token reporting turned an invisible cost into a named one: the bridge's 0 percent cache rate against the proxy's 81 percent. That is a concrete engine lever, translate the responses cache markers through the bridge, worth far more on paid models than any prompt tweak.
- Pre-build the environment or measure the wrong thing. Without `--prep-env` a probe measures `pip` fighting, not the fix. It is the difference between a fidelity test and noise.
- This task grades PR retrieval, not reasoning. Consistent with 0027: no model passes `dynaconf-1225` pass@1 from the code alone, so read it for behaviour and cost, and keep the pass-rate headline for tasks the prompt fully describes.

## Reproduce

The probe is in-process and needs no container, so a full A/B is a seconds-long loop.

1. Build the lab against the local tomo checkout: `go build -o /tmp/lab ./cmd/lab`.
2. Source the proxy key for the deepseek runs: the free model is served through the opencode proxy, so the key must be in the environment before the probe.
3. Run A and B as above, then read them back with `lab probe analyze /tmp/runA`, which prints the round-by-round token curve and the tool mix without spending a token.
4. For the gpt-5.6 runs, start `lab bridge` first and point the probe at it with `--base-url`. The bridge drives the user's own subscription and must be run one consumer at a time, since two callers on the same subscription rate-limit each other.
5. Every run writes `summary.json` with the priced cost and the cached-token count, `trace.jsonl` with the full request and response of every call, and `transcript.md` with the readable turn.
