---
title: "What the closed-door dynaconf runs teach tomo"
linkTitle: "dynaconf closed-door lessons for tomo"
description: "Seven honest runs on one task, three passes and four fails, read together. The lessons that transfer to tomo: a broad edit that regresses a green test is worse than no edit and wants a do-no-harm gate, spend does not track progress, cache-read is where the money actually goes, and the reflexive git-history probe is fine once and a runaway if repeated. This is the cross-trace synthesis, with a full cost breakdown, the tests each run was graded on, and the commands to reproduce it."
date: 2026-07-13T14:55:00+07:00
---

Seven models ran `dynaconf__dynaconf-1225` with both answer doors shut: the git-history door pruned and the network denied.
Three solved it, four failed, and every one of the seven was honest, no answer fetched.
Read as a set they say more than any single run, and most of what they say points at concrete tomo work.

This note collects the synthesis in one place: the runs, a full cost breakdown, the four lessons, the exact tests each run was graded on, and the commands to reproduce the whole thing.

## The runs

| Model | Harness | Files | Tokens | Cost | Verdict |
|---|---|---|---|---|---|
| [gpt-5.6-luna](/experiments/2026/07/13/14-15-dynaconf-gpt-5.6-luna-offline-first-clean-solve/) | codex | 25 | 10.1M | $1.46 | PASS, clean |
| [gpt-5.6-terra](/experiments/2026/07/13/14-45-dynaconf-5.6-family-solves-analyzer-false-leak/) | codex | 23 | 3.2M | $1.82 | PASS (honest, false leak flag) |
| [gpt-5.6-sol](/experiments/2026/07/13/14-45-dynaconf-5.6-family-solves-analyzer-false-leak/) | codex | 23 | 3.2M | $2.56 | PASS (honest, false leak flag) |
| [gpt-5.4-mini](/experiments/2026/07/13/13-49-dynaconf-gpt-5.4-mini-offline-honest-fail/) | codex | 9 | 4.83M | $0.78 | FAIL, clean |
| [gpt-5.5](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/) | codex | 19 | 6.08M | $4.49 | FAIL, clean |
| [sonnet-5](/experiments/2026/07/13/14-25-dynaconf-sonnet-offline-honest-broad-fail/) | claude | 22 | 23.5M | $10.32 | FAIL, clean |
| [opus-4.8](/experiments/2026/07/13/14-35-dynaconf-opus-offline-regresses-green-test/) | claude | 23 | 20.7M | $47.18 | FAIL, regressed a green test |

Every dollar figure is what the same tokens would cost on the metered API, priced from the [shared pricing table](/guides/).
A ChatGPT or Claude subscription is not billed per token, so this is a like-for-like cost, not the user's actual bill.

## Cost breakdown

The single number in the table hides where the money goes, and where it goes is the lever.
Here is each run split into its four token classes.

| Model | Fresh input | Cache read | Cache write | Output | Total |
|---|---|---|---|---|---|
| gpt-5.6-luna | $0.2457 | $0.9836 | included in read | $0.2312 | $1.4605 |
| gpt-5.6-terra | | not split by codex | | | $1.8240 |
| gpt-5.6-sol | | not split by codex | | | $2.5628 |
| gpt-5.4-mini | $0.2956 | $0.3305 | included in read | $0.1579 | $0.7840 |
| gpt-5.5 | $0.8549 | $2.9429 | included in read | $0.6905 | $4.4883 |
| sonnet-5 | $0.1198 | $6.8849 | $1.4293 | $1.8825 | $10.3165 |
| opus-4.8 | $0.1869 | $30.3442 | $5.5643 | $11.0872 | $47.1827 |

Two things fall straight out of the split.

Fresh input is a rounding error everywhere.
The prompt and the files are read once, cheap, and then never re-billed at the input rate.

Cache read is the bill.
It is the whole context, the conversation and every file the agent has pulled into view, re-sent to the model on every single turn.
On the $47 opus run, cache read alone is $30 of the $47.
On sonnet it is $6.9 of the $10.
The longer the run and the fatter the working context, the more this dominates, and it dominates by a wide margin over the tokens the model actually writes.

Output is second, and it only becomes visible on the very long runs.
Opus wrote $11 of output across 194 turns; luna wrote 23 cents across 80.
Cache write, the cost of laying context into the cache the first time, is a minor line even on the big runs.

The tokens counts tell the same story from the other side.
The three cheapest passes, the gpt-5.6 family, moved 3 to 10 million tokens.
The two most expensive fails, sonnet and opus, moved 20 to 23 million, most of it context re-sent turn after turn while the run went nowhere.

## Lesson 1: a broad edit that breaks a green test wants a do-no-harm gate

The naive read of the table is that breadth or spend decides the outcome.
Both are wrong.
luna went the widest, twenty-five files, and passed.
opus spent the most, $47, and produced the worst result of the seven, a repo where a test that was green at the base commit is now red.

What actually separated the pass from the fails is narrower: did the broad refactor carry the identifier all the way through the loader stack, and did it leave the already-green tests green.
luna's did both.
opus's did neither, and the second failure is the instructive one.
It did not merely fail to fix the target, it damaged working behavior on the way to failing.

tomo already has a [convergence guard](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/) that stops it running away searching for a fix.
It has nothing that stops it shipping an edit that regresses a test that was passing.
The gap this run exposes is a do-no-harm gate: after an edit batch, run the in-repo tests the change plausibly touched, and treat a green-to-red flip as a stop-and-reconsider signal rather than continuing to pile edits on.
It is model-independent, it would have caught opus before its twenty-third file, and it would not have fired once on luna.
That is the property to want: it punishes net-negative edits, not breadth.

## Lesson 2: spend does not track progress

Cost on this task ranged from $0.78 to $47.18 and told you nothing about the verdict.
The three cheapest passing runs, the gpt-5.6 family, cost $1.46, $1.82, and $2.56.
The most expensive run failed and regressed.
Turn count says the same: opus took 194 turns and sonnet 235 to reach a wall the gpt-5.6 models cleared in a few dozen tool calls.

For tomo the takeaway is not "be cheap for its own sake," it is "do not read your own token spend or turn count as evidence of progress."
A loop that has spent a lot has not thereby earned anything, and the guards should measure movement toward a passing fix, not effort.
tomo's leanness pitch holds on the merits here: its honest fails are cheap, and cheap-and-honest beats the $47 fail every time.

## Lesson 3: cache-read is where the money goes

The cost breakdown above is the whole lesson.
Output-length trimming barely moves the bill.
What moves it is the size of the context tomo re-sends each turn, because that is what gets billed at the cache-read rate on every turn of a long run.

That is a direct pointer for tomo.
It is why [trimming the redundant read-after-write](/experiments/2026/07/13/11-05-churn-guard-vs-claude-code/) was a real lever, and why keeping the working context tight, dropping stale file dumps and superseded tool output, is the cost work that pays off.
Shrinking what tomo re-sends per turn attacks the $30 line, not the 23-cent one.

## Lesson 4: the reflexive history probe is fine once, a runaway if repeated

[terra and sol](/experiments/2026/07/13/14-45-dynaconf-5.6-family-solves-analyzer-false-leak/) both opened with `git log --all --grep=<issue number>`, the shortcut instinct, before doing any real work.
The prune denied it, they got nothing, and they moved on and solved the bug.
One probe, denied, then the work.

That is the same instinct behind tomo's [git-archaeology runaway](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/).
The difference is entirely in the repeat.
terra and sol probed once and stopped; tomo's failure mode was to keep digging when the first dig came up empty.
The lesson is that the convergence guard should not try to forbid the probe, which is cheap and human, it should bound the repeat, which is the actual runaway.
This is more evidence the guard is aimed right: cap the digging, not the first look.

## What tests each run was graded on

Every verdict in the table is the output of one grader, the task's `check.sh`, run identically on all seven work trees.
It builds a fresh Python 3.12 venv with `uv`, installs dynaconf from the work tree, applies the hidden test patch, and runs the bug's test file with `pytest -rA`.
The grade is read from the per-test outcome lines, not from the exit code.

Two sets of tests decide it, both in `tests/test_settings_loader.py`.

The fail-to-pass set is the bug.
These are red at the base commit and a correct fix turns them green.

```
test_load_using_settings_loader
test_load_using_settings_loader_with_multi_temporary_env
test_load_using_settings_loader_with_one_env_named_file_module_path
```

The `_module_path` variant is the one the whole sub-gpt-5.6 field missed: every honest fail carried the identifier through the file-path loader but not the module-path loader, so this test stayed red.

The pass-to-pass set is the do-no-harm check.
These are green at the base commit and must stay green.
This is where opus alone broke:

```
test_load_using_settings_loader_with_one_env_named_file_file_path   # green at base, opus turned it red
```

A run passes only if every fail-to-pass id is green and no pass-to-pass id regressed.
luna, terra, and sol cleared both.
The four fails all missed the module-path fail-to-pass id; opus additionally flipped the file-path pass-to-pass id, which is why it is the only run that left the repo worse than it found it.

This is exactly the shape the do-no-harm gate from lesson 1 would run for tomo: after an edit batch, run `tests/test_settings_loader.py` and stop if anything that was green went red.

## Reproduce

Everything here runs from the tomo-labs checkout.
The offline, both-doors-closed condition is two independent controls: `setup.sh` prunes the git-history door, and the container the tool runs in denies all network except the local model bridge.

List the tier and run tomo over it:

```bash
go run ./cmd/lab scenarios --suite swebench-live        # list the tasks
go run ./cmd/lab run tomo --suite swebench-live          # run tomo over the tier
go run ./cmd/lab report --suite swebench-live            # the comparison table
```

Set up and grade the dynaconf instance by hand to see exactly what the runs saw:

```bash
S=evals/swebench-live
W=$(mktemp -d)
bash $S/tasks/dynaconf__dynaconf-1225/setup.sh "$W"   # clone at base commit, prune future history
# ... a tool edits the code in $W ...
bash $S/tasks/dynaconf__dynaconf-1225/check.sh "$W"   # PASS or FAIL, from the hidden tests
```

Confirm the git-history door is actually shut in the prepared tree:

```bash
git -C "$W" log --all --oneline | wc -l                    # only ancestors of the base commit
git -C "$W" log --all --oneline --grep=1204                # 0 hits, the fix commit is gone
git -C "$W" cat-file -e da0054e 2>&1 || echo "fix commit unreachable"
```

Run the graded tests directly, the way `check.sh` does:

```bash
cd "$W"
uv venv --python 3.12 .venv && . .venv/bin/activate
uv pip install -q -e '.[test]'
python -m pytest -rA -p no:cacheprovider tests/test_settings_loader.py
```

Check any subscription run for an answer fetch, the fairness pass every result goes through:

```bash
go run ./cmd/lab codex analyze --patch     # codex rollout: did the edit come from a git diff of the fix
go run ./cmd/lab claude analyze --patch    # claude session: same question
```

The [answer-doors writeup](/experiments/2026/07/13/11-50-dynaconf-sol-answer-leak-closed/) is the full story of the `setup.sh` prune and why both doors have to be shut for the grade to mean anything.

## The bar, stated plainly

With both doors shut, dynaconf-1225 is a task the current model generation solves and the previous one does not, on the fix's merits, from the code alone.
That is the honest bar tomo is measured against.
tomo's real gaps were never the quality of the fix.
They were running away digging for it, which the [convergence guard](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/) closes, and shipping a broad edit without checking it did no harm, which the do-no-harm gate from lesson 1 would close.
The deeper writeup of these lessons and the tomo changes they imply lives in the tomo experiment journal, note 0024.
