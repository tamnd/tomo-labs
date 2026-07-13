---
title: "dynaconf on gpt-5.6-terra and -sol: two honest solves the analyzer mislabeled a leak"
linkTitle: "dynaconf 5.6 family + analyzer false leak"
description: "gpt-5.6-terra and -sol both pass dynaconf-1225 with the doors shut, and both got flagged as answer leaks. Reading the trace, the flag is wrong. They ran git log --all --grep on the issue number, the reflexive shortcut, but the history prune had already removed the fix commit, so the command returned nothing and they solved the bug anyway. The whole gpt-5.6 family clears this task. The analyzer flag is a precision bug worth fixing."
date: 2026-07-13T14:45:00+07:00
---

Two runs: the real `codex` CLI on the user's subscription, models `gpt-5.6-terra` and `gpt-5.6-sol` at high reasoning, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier, both under the both-doors-closed harness.
Both passed the hidden tests.
Both got flagged `LEAK, reached the answer (1 git-history)` by the fairness [analyzer](/guides/).

The flag is a false positive, and running it down is the interesting part.
It says something about the models, about the harness, and about a real bug in the leak detector.

## Reproducibility

| | terra | sol |
|---|---|---|
| Model | `gpt-5.6-terra` high | `gpt-5.6-sol` high |
| Verdict on tests | PASS, fail-to-pass green, in-file pass-to-pass stable | PASS, same |
| Analyzer fairness | flagged LEAK (1 git-history) | flagged LEAK (1 git-history) |
| Fix commit reachable in pruned tree | no, `git log --all --grep=1204` returns 0 hits | no, 0 hits |
| Cost | 3,203,994 tokens, 32 tool calls, $1.8240 | 3,189,838 tokens, 35 tool calls, $2.5628 |
| Wall | 515s | 304s |

Task is `dynaconf__dynaconf-1225`, dynaconf at base commit `39acdee`, history pruned by [the setup.sh strip](/experiments/2026/07/13/11-50-dynaconf-sol-answer-leak-closed/), graded in a Python 3.12 venv.

## What the flag caught, and what it missed

The leak detector flags a command by its shape.
Both models, early in the run, reached for the obvious shortcut:

```bash
# terra
git log --all --oneline --decorate --grep='1204\|Port' -i && git merge-base --is-ancestor HEAD 3.2.5^{} ...
# sol
git branch -a && git log --all --oneline --decorate --grep='1204\|Insert token\|multiple prefixes\|identifier param' ...
```

`git log --all --grep=<issue number>` is the archetypal answer lookup, so the analyzer flags it, and on an un-pruned tree it would be right to.
But the [history prune](/experiments/2026/07/13/11-50-dynaconf-sol-answer-leak-closed/) had already rewritten the work tree so the fix commit and its release tag are unreachable, while the ancestor history that version detection needs stays.
Check the pruned tree directly and the fix is gone: `git log --all --oneline --grep='1204'` returns zero commits.
The reflexive command ran, and it returned nothing.

So terra and sol did not get the answer from history.
They probed for it, the door was shut, they got an empty result, and they solved the bug from the code anyway.
The proof is in the grade: the fix commit is unreachable, so the passing diff cannot have been cherry-picked from it, and both runs pass the hidden tests with their own twenty-three-file refactor.
These are honest solves.

## What it says about the models

Read against the fails, this completes a clean generational line on dynaconf-1225.

| Generation | Models | Result |
|---|---|---|
| gpt-5.6 | luna, terra, sol | all solve, honestly, $1.46 to $2.56 |
| older | gpt-5.4-mini, gpt-5.5, sonnet-5, opus-4.8 | all fail, honestly, $0.78 to $47.18 |

The whole gpt-5.6 family clears a bar nothing older clears.
[luna solved without even probing history](/experiments/2026/07/13/14-15-dynaconf-gpt-5.6-luna-offline-first-clean-solve/); terra and sol probed, found the door shut, and solved anyway.
The probe is a discipline tell, the shortcut instinct firing before the work, but the harness denied it and the models did the work.
That is exactly the behavior the closed doors are meant to force, and it worked.

## What it says about the harness

The analyzer has a precision bug.
It flags a history-mining command by pattern, whether or not the command actually returned the answer.
After the prune, `git log --all --grep=<issue>` returns nothing, so a pattern-only flag turns two honest solves into false leaks and would, uncorrected, undercount the gpt-5.6 family.

The fix is to make the leak check outcome-aware, not pattern-aware: flag a history probe only when future history still exists in the tree or when the command's output actually contains fix content.
Post-prune the tree has no future commits, so an empty `git log --all --grep` should read as a denied probe, not a successful leak.
That change is tracked for the analyzer; until it lands, terra and sol are read here as honest passes with a known-false leak flag, not as leaks.

For tomo the lesson is on the model-behavior side, not the harness side.
The reflexive `git log --all --grep=<issue>` is the same shortcut instinct that produced tomo's [git-archaeology runaway](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/).
The difference is terra and sol probed once, got nothing, and moved on to the actual fix, where tomo's failure mode was to keep digging.
The [convergence guard](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/) is what turns tomo's dig into a single denied probe like theirs, and this run is more evidence that a single probe is fine and the runaway is the bug.

## Reproduce it

```bash
bash ~/data/evals/codex-real/run_offline.sh dynaconf__dynaconf-1225 gpt-5.6-terra high
bash ~/data/evals/codex-real/run_offline.sh dynaconf__dynaconf-1225 gpt-5.6-sol high
# Both grade PASS. The leak flag fires on the git log --all probe; confirm it is
# empty against the pruned tree:
( cd .../work && git log --all --oneline --grep='1204' -i )   # 0 commits: prune held
```
