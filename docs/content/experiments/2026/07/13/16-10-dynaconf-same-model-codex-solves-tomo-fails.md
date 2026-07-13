---
title: "Same model, codex solves dynaconf and tomo does not"
linkTitle: "dynaconf same model, codex solves, tomo fails"
description: "The cleanest control yet on dynaconf-1225: hold the model fixed and vary only the harness. The gpt-5.6 subscription models are driven through the same codex backend by both codex and tomo, over the closed-door task. codex solves it with luna, terra, and sol; tomo fails with all three, in two distinct shapes. luna under tomo makes a confident wrong fix, fifteen edits across six files that never touch the code the bug lives in, and verifies it green against its own wrong mental model. terra runs the git-history-and-network runaway until the clock kills it. This is read straight off the new forensic analyzer, and it points the tomo work away from the convergence guards and at grounding."
date: 2026-07-13T16:10:00+07:00
---

The earlier closed-door runs varied two things at once, the model and the harness, so they could not separate a harness gap from a model gap.
This run fixes that.
It holds the model fixed and varies only the harness.

The gpt-5.6 subscription models each talk to one backend, the codex responses API, and two harnesses drive that same backend over the same task: codex, through its own CLI, and tomo, through the lab's bridge that translates its chat calls into the same responses API.
Same weights, same both-doors-closed condition, different harness around them.
The question is whether tomo's harness is what stands between it and the solve.

## The result

| Model | codex | tomo | tomo failure shape |
|---|---|---|---|
| gpt-5.6-luna | PASS | FAIL | confident wrong fix, 15 edits over 6 files |
| gpt-5.6-terra | PASS | FAIL | investigation runaway, killed by the clock |
| gpt-5.6-sol | PASS | (running) | |

The codex column is the [14-55 synthesis](/experiments/2026/07/13/14-55-dynaconf-doors-closed-lessons-for-tomo/): luna, terra, and sol all solve `dynaconf__dynaconf-1225` under codex, on the fix's merits, from the code alone.
The tomo column is this run.
luna is the clean data point, both harnesses ran it to completion.
terra is weaker evidence and is called out as such below, because a wall-clock timeout, not the model, ended it.

One wire caveat, stated up front.
codex speaks the responses API natively; tomo speaks chat and the bridge converts it.
The model weights are identical, but the request shape is not, so a sliver of the gap could be the conversion rather than the harness.
It is a sliver: the failures below are about what the model spent its rounds on, not about a malformed request.

## tomo with luna: a confident wrong fix

The analyzer reads the whole run without a hand pass over the trace:

```
tomo did not solve dynaconf__dynaconf-1225 in 58 requests and 3,638,030 tokens.
It read 2 files, searched 8 times, made 15 edits to dynaconf/utils/parse_conf.py,
  dynaconf/base.py, dynaconf/loaders/__init__.py, dynaconf/loaders/env_loader.py,
  dynaconf/loaders/redis_loader.py, dynaconf/cli.py, ran 26 shell commands, wrote 4 plans.
It went to the network 1 time (api.github.com).
Its longest stretch without changing a file was 22 calls, a sign it dug well past the point of deciding.
It read git history 3 times (2 of them grepping every ref for the issue), the reflexive first look.
The convergence guard fired: no-edit and churn.
It checked its own work before finishing.
```

Read that against what the fix actually is.
The bug lives in `settings_loader` in `dynaconf/loaders/__init__.py`, which loads a dotted module path wrong: it produces `development_myproject.settings` where the fix produces `myproject.development_settings`.
The correct change is the #1204 port: give `settings_loader` an identifier, refactor the env-named-file loading into a helper that splits the dotted path with `rsplit(".", 1)`, and loop over the environments.
That is the change codex+luna wrote, and it turns the `_module_path` test green.

tomo+luna never went there.
Across all 57 of its model responses the strings `module_path`, `settings_loader`, and `rsplit` appear zero times.
It added one line to `loaders/__init__.py` and spent its fifteen edits elsewhere, in `env_loader.py` and `parse_conf.py`, building support for multiple environment prefixes.
Its own closing note names the direction it took: "supporting multiple prefixes."
That is a real feature and a wrong answer.
It read the issue, formed the wrong mental model of the bug, and then executed that model well enough to verify it green against its own reading.

This is the failure the convergence guards cannot see.
Both the no-edit guard and the churn guard fired on this run, and neither was wrong to: the run dug 22 rounds before its first edit and then wrote to six files.
But the guards bound cost and runaways, and this was neither.
It was a finished, self-verified, wrong fix.
No stall bound, no no-edit bound, no churn bound, and no do-no-harm gate catches a run that confidently edits the wrong code and confirms it against the wrong test.

## tomo with terra: the runaway, then the clock

terra is the other shape, and the shape the free models keep landing in.
The analyzer on its run:

```
It changed no file at all across 22 calls; the run was pure investigation and can only fail.
It read git history 2 times (1 of them grepping every ref for the issue), the reflexive first look.
The convergence guard fired: no-edit.
 6. fetched https://github.com/dynaconf/dynaconf/pull/1204 [network]
     -> ERROR: ... no such host [error]
 7. ran git fsck --unreachable ...; git log --all --grep=1204 [history-probe]
 ⚑ harness: no-edit guard nudged the model
```

It opened by reaching for the answer: a fetch of the pull request, denied by the closed network door, then `git fsck --unreachable` and `git log --all --grep=1204`, denied by the pruned history door.
Both doors held.
Then it read source, the no-edit guard nudged it once at the twelfth round, and it kept reading.
It never edited, and at 900 seconds the lab's wall-clock killed it with 28 requests done.

terra's failure is confounded and honest about it.
The bridge adds about 6.8 seconds of first-token latency to every call, so terra was slow before it was wrong, and the timeout is partly a bridge-latency artifact rather than a pure tomo verdict.
What is not confounded is the opening: the same reflexive git-history and network probe the free deepseek run made, now from a strong model, because the harness invites it.

## What this points the tomo work at

Three things follow, and they move the target off the convergence guards.

The guards are working and are not the lever here.
They caught the cost of terra's runaway and they fired honestly on luna.
tomo's cheap, honest fails are a real product property, and it comes from these bounds.
But the luna run proves a fully guarded run can still fail, because the guards watch effort and novelty, not correctness.
Tightening or re-firing a nudge would not have turned luna's wrong fix into the right one.

The guard nudges latch, and the traces show them ignored.
Each nudge fires exactly once per turn and then goes silent, so terra got one no-edit nudge at round twelve and nothing more through the timeout, and deepseek got one at round forty and mined on to round fifty-six.
Two independent traces where the single nudge changed nothing is not proof the nudges never help, but it is a reason to stop treating them as the lever and to lean the harness leaner: keep the hard limits that bound cost, and stop adding soft steering that the model reads past.

The gap that actually decides luna is grounding, not steering.
The same model engages the module-path bug under codex and misses it under tomo.
That is upstream of any guard: it is what the harness puts in front of the model as the task, and how much of the codebase the model has genuinely in view before it commits to a reading.
codex+luna arrived at `settings_loader` and the dotted-path split; tomo+luna arrived at multiple prefixes and never looked at the loader the issue points to.
The next tomo experiment is to compare the two task presentations directly and see what codex gives the model that tomo does not.

And the opening waste is real on both shapes.
luna and terra both spent early rounds on a github fetch and a git-history dig that the closed doors deny.
On a task where egress is gone and the history is pruned, offering the model a network tool and letting it rake the reflog is rounds spent learning the doors are shut.
That is a harness choice tomo can make differently.

## How this was read

Every number and tag above came from `lab inspect`, not a hand read of the trace.
The [forensic analyzer](https://github.com/tamnd/tomo-labs/pull/71) now counts shell edits, git-history reads and the answer-shortcut probe, the longest no-edit streak and the zero-edit flag, and the convergence-guard nudges, and the per-step walkthrough tags each move for what it was and surfaces the guard nudge inline where it fired.
The two failure shapes in this note, the confident wrong fix and the investigation runaway, are now the analyzer's own summary lines, so the next run reads itself.

## Reproduce

```bash
# tomo over the task, driven through the subscription bridge on one model
lab bridge --model gpt-5.6-luna --effort high --port 8790 &
LAB_UPSTREAM=http://host.containers.internal:8790 LAB_MODEL=gpt-5.6-luna \
  go run ./cmd/lab run tomo --suite swebench-live dynaconf__dynaconf-1225

# read the run the way this note did
go run ./cmd/lab inspect --suite swebench-live tomo dynaconf__dynaconf-1225
```
