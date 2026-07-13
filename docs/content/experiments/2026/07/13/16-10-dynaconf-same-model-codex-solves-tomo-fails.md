---
title: "Same model, codex solves dynaconf and tomo does not"
linkTitle: "dynaconf same model, codex solves, tomo fails"
description: "Hold the model fixed on the codex backend and vary the harness. On dynaconf-1225, codex solves the bug with luna, terra, and sol; tomo fails with all four subscription models it was run on. Three of the four that ran to completion make the same wrong turn: they never touch settings_loader, the loader the bug lives in, and spend their edits in the parsing code next door. The analyzer reads each run without a hand pass. Two things come out of it: one honest confound to fix in the benchmark, and one deeper grounding gap that is the real tomo lever."
date: 2026-07-13T16:10:00+07:00
---

The earlier closed-door runs varied two things at once, the model and the harness, so they could not separate a harness gap from a model gap.
This run fixes the model and varies the harness.

The gpt-5.6 subscription models each talk to one backend, the codex responses API, and two harnesses drive that same backend over the same task: codex, through its own CLI, and tomo, through the lab's bridge that translates its chat calls into the same responses API.
Same weights, same both-doors-closed condition, different harness around them.
The question is whether tomo's harness is what stands between it and the solve.

## The result

| Model | codex | tomo | tomo failure shape |
|---|---|---|---|
| gpt-5.6-luna | PASS | FAIL | wrong fix, 6 files, never touched settings_loader |
| gpt-5.6-terra | PASS | FAIL | investigation runaway, killed by the clock |
| gpt-5.6-sol | PASS | FAIL | wrong fix, 6 files, never touched settings_loader |
| gpt-5.5 | PASS | FAIL | wrong fix, 4 files, never touched settings_loader |

The codex column is the [14-55 synthesis](/experiments/2026/07/13/14-55-dynaconf-doors-closed-lessons-for-tomo/): these models solve `dynaconf__dynaconf-1225` under codex, on the fix's merits, from the code alone.
The tomo column is this run: four models, all through the bridge, all failing.
Four for four is the headline, and the three that ran to completion fail the same way.

## The same wrong turn, three times

The fix lives in `settings_loader` in `dynaconf/loaders/__init__.py`, which loads a dotted module path wrong: it produces `development_myproject.settings` where the fix produces `myproject.development_settings`.
The correct change is the #1204 port: give `settings_loader` an identifier, refactor the env-named-file loading into a helper that splits the dotted path with `rsplit(".", 1)`, and loop over the environments.
That is what codex+luna wrote, and it turns the `_module_path` test green.

None of the three finishing tomo runs went there.
Across every model response in each run, the strings `settings_loader`, `module_path`, and `rsplit` appear zero times:

| Model | settings_loader | module_path | rsplit | files edited |
|---|---|---|---|---|
| luna | 0 | 0 | 0 | parse_conf, base, loaders/__init__, env_loader, redis_loader, cli |
| sol | 0 | 0 | 0 | base, loaders/__init__, ini_loader, json_loader, utils/__init__, parse_conf |
| gpt-5.5 | 0 | 0 | 0 | base, loaders/__init__, utils/__init__, parse_conf |

Three independent strong models, under tomo, all read the same eleven-item issue checklist and all picked the wrong items off it, converging on the token-parsing and prefix code around `parse_conf.py` and `base.py` rather than the loader the bug names.
One model missing the crux is a fluke.
Three missing the same crux the same way is the harness.

This is the failure the convergence guards cannot see.
The guards fired honestly on these runs, bounding cost, but the runs were finished, self-verified, wrong fixes, not runaways.
No stall bound, no no-edit bound, no churn bound, and no do-no-harm gate catches a run that confidently edits the wrong code and confirms it against the wrong test.

## tomo with terra: the runaway, then the clock

terra is the other shape, the one the free models keep landing in.
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
It never edited, and at 900 seconds the wall-clock killed it.

terra's failure is confounded and honest about it.
The bridge adds about 6.8 seconds of first-token latency to every call, so terra was slow before it was wrong, and the timeout is partly a bridge-latency artifact rather than a pure tomo verdict.
What is not confounded is the opening: the same reflexive git-history and network probe the free deepseek run made, now from a strong model.

## The honest confound, and the fix

There is a confound in the codex column, and it is worth stating plainly rather than burying.
codex did not just have a better harness on this task.
Its offline harness also handed the model a better-grounded task.
The codex prompt closes with environment notes tomo's never had:

```
Environment notes:
- You have NO network access. Do not attempt to install packages, fetch URLs,
  or use git remotes; those calls will fail.
- A virtualenv with the project and its test dependencies pre-installed is at
  ./.venv-agent. Run tests with ./.venv-agent/bin/python -m pytest.
```

The swebench-live prompt tomo and every rival run under said none of this.
It ended at the issue text.
So a tool driven through codex was told the two doors were shut, and a tool driven through the lab's suite had to find out by walking into them, which is exactly what terra's opening does.

That asymmetry both wastes tomo's rounds and muddies this comparison, so the benchmark half is being fixed: [the swebench-live prompt now states the closed doors](https://github.com/tamnd/tomo-labs/pull/73) up front, the network is blackholed and the history is truncated to the base commit, for every tool equally.
It names what is absent, never what to change, so it is not a hint at the fix.
This levels the presentation and should end the opening waste on both the free and the subscription runs.

But leveling the doors will not, by itself, fix the wrong turn.
The environment notes tell a run not to fetch or mine history; they do not tell it to read `settings_loader`.
The three wrong fixes did not fail because they wasted an opening round on a closed door.
They failed because, with the code fully in front of them, they read the eleven-item checklist and dug into the wrong item.

## What actually decides it: how the run reads the codebase

Put the two openings side by side.
codex+luna, once oriented, ran one wide sweep:

```
rg -n "source_metadata|build_env_list|settings_loader|env_loader|redis_loader|json.dumps|populate_obj" 
```

It enumerated every symbol the checklist named, in one command, then read `base.py` where `settings_loader` is called and worked outward from there.
Breadth first, the whole issue mapped to the code before committing to a reading.

tomo+luna searched eight times, each narrow, and settled early on the prefix-and-token items, so `settings_loader` never entered its view.
It formed a mental model from part of the checklist and executed that model well enough to verify it green against its own reading.
That is a real feature and a wrong answer.

The difference that decides this task is not a guard and not a door.
It is how much of the codebase the run genuinely has in view before it commits.
codex's harness produced a systematic enumeration; tomo's produced a narrow dive.
That is upstream of every convergence bound, and it is the tomo lever this run points at.

## Where that leaves the guards

The convergence guards are working and are not the lever here.
They bounded terra's runaway and they fired honestly on the wrong fixes.
tomo's cheap, honest fails are a real product property and they come from these bounds, so the review of them lands on keep, not remove.

The one real weakness in them is smaller than a redesign: each soft nudge fires exactly once per turn and then goes silent, and two traces show the single nudge changed nothing, terra's at round twelve and deepseek's at round forty.
That is a reason to lean the harness leaner over time, keep the hard limits that bound cost and stop leaning on soft steering the model reads past, but it is not what lost dynaconf.
What lost dynaconf was the reading, three times over.

## How this was read

Every number and tag above came from `lab inspect`, not a hand read of the trace.
The [forensic analyzer](https://github.com/tamnd/tomo-labs/pull/71) counts shell edits, git-history reads and the answer-shortcut probe, the longest no-edit streak and the zero-edit flag, and the convergence-guard nudges, and the per-step walkthrough tags each move and surfaces the guard nudge inline where it fired.
The crux check, whether a run ever named `settings_loader`, `module_path`, or `rsplit`, is a grep over the model's own responses, and it is the same three-zero result for every finishing run.

## Reproduce

```bash
# tomo over the task, driven through the subscription bridge on one model
lab bridge --model gpt-5.6-luna --effort high --port 8790 &
LAB_UPSTREAM=http://host.containers.internal:8790 LAB_MODEL=gpt-5.6-luna \
  go run ./cmd/lab run tomo --suite swebench-live dynaconf__dynaconf-1225

# read the run the way this note did
go run ./cmd/lab inspect --suite swebench-live tomo dynaconf__dynaconf-1225
```
