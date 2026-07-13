---
title: "dynaconf on gpt-5.5: six times the cost, the same wall"
linkTitle: "dynaconf gpt-5.5 offline"
description: "The flagship codex model runs dynaconf-1225 with both answer doors closed. It writes nineteen edits across every loader, the validator, and the cli, twice what the cheap model touched, spends six times as much, reaches no answer, and fails on the exact same two settings-loader tests. With the doors shut, paying more buys a broader wrong fix, not a right one."
date: 2026-07-13T14:01:00+07:00
---

This is one run: the real `codex` CLI on the user's subscription, model `gpt-5.5` at high reasoning, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier, under the same both-doors-closed harness as the [gpt-5.4-mini run](/experiments/2026/07/13/13-49-dynaconf-gpt-5.4-mini-offline-honest-fail/).
It failed, cleanly.
It is the flagship measured honestly, and the honest flagship does not solve the task either.

The point of pairing it with the cheap model is the comparison.
The same task, the same closed doors, the same wall: what does five times the price actually buy on a task neither can look up?

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13, on the user's codex subscription |
| Tool | real `codex` CLI under `-s workspace-write` (shell has no network) |
| Model | `gpt-5.5`, high reasoning effort |
| Task | `dynaconf__dynaconf-1225`, dynaconf at base commit `39acdee`, history pruned, graded in a Python 3.12 venv |
| Verdict | FAIL, clean. No answer fetch over network or git history |
| Result | 3 failed, 6 passed. The `FAIL_TO_PASS` targets in `tests/test_settings_loader.py` stayed red, the two module-path variants failing outright |
| Cost | 6,079,821 tokens (input 6,056,806 at 97% cache hit, output 23,015 with 8,779 reasoning), 82 tool calls, 19 source edits, 615s, $4.4883 at gpt-5.5 API list price |

Priced from the [shared pricing table](/guides/): $0.8549 fresh input, $0.6905 output, the rest cache read.

## What it did

gpt-5.5 read the same checklist and took the same instinct as the cheap model, "`*_loader must take identifier param`", and went wider with it.
Where gpt-5.4-mini touched nine files and stopped at the loaders it needed, gpt-5.5 wrote nineteen edits: an `identifier` through every format loader (`toml`, `yaml`, `json`, `ini`, `redis`, `env`), the loader dispatch, `base.py`, then on into `validator.py`, `cli.py`, and `parse_conf.py`.
It is the fuller, more thorough port, closer in surface area to the seventeen-file gold patch than anything the cheaper models attempted.

And it lands in the same place.
On grading, the settings-loader targets stayed red, and the two module-path tests, `test_load_using_settings_loader_with_one_env_named_file_module_path` and its `_multi_env` sibling, failed exactly as they did for gpt-5.4-mini.
Nineteen edits and six times the spend moved the failure not at all.

## What the pair says

| Model | Edits | Tokens | Cost | Verdict | Failing tests |
|---|---|---|---|---|---|
| gpt-5.4-mini | 9 | 4.83M | $0.78 | FAIL, clean | same two module-path |
| gpt-5.5 | 19 | 6.08M | $4.49 | FAIL, clean | same two module-path |

The lever over a rival on this task is not correctness and it is not model size.
Both models fail, on the identical tests, for the identical reason: the broad identifier refactor regresses the module-path settings load.
The flagship just fails more expensively.

This is the finding that survives the doors being shut.
On [the earlier open-door runs](/experiments/2026/07/13/12-20-dynaconf-closed-sorts-the-models/) a strong model could always reach GitHub and "pass", so the leaderboard measured willingness to look up the answer.
Closed, dynaconf-1225 is a task no model in this comparison solves, cheap or flagship.
That is the honest bar tomo is measured against, and on the merits of the fix tomo sits with them.
tomo's real, fixable gap was never the fix, it was [running away digging for it](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/); the [convergence guard](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/) is what closes that gap, and it lets tomo fail cheaply and honestly the way gpt-5.4-mini does, instead of burning four million tokens the way gpt-5.5 nearly does to reach the same wall.

## Reproduce it

```bash
bash ~/data/evals/codex-real/run_offline.sh dynaconf__dynaconf-1225 gpt-5.5 high
# Expect: VERDICT: FAIL and fairness: CLEAN, the same two module-path tests red.
```
