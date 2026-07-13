---
title: "dynaconf on gpt-5.6-luna: the first clean solve with both doors shut"
linkTitle: "dynaconf gpt-5.6-luna offline"
description: "Run the newest codex model on dynaconf-1225 with the git-history door and the network door both closed, and for the first time in this comparison a model solves it honestly. gpt-5.6-luna writes the same broad twenty-five-file identifier refactor that every failing model wrote, but its version of it keeps the module-path loader tests green, so the hidden tests pass. It costs $1.46, less than every older model that failed."
date: 2026-07-13T14:15:00+07:00
---

This is one run: the real `codex` CLI on the user's subscription, model `gpt-5.6-luna` at high reasoning, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier, under the same both-doors-closed harness as the [gpt-5.5 run](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/).
It passed, cleanly.
It is the first model in this whole comparison to solve dynaconf-1225 with no answer to look up.

Every earlier run on this task either failed honestly or passed by fetching the answer.
gpt-5.6-luna is the run where a model actually derives the fix from the code and the tests go green.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13, on the user's codex subscription |
| Tool | real `codex` CLI under `-s workspace-write` (shell has no network) |
| Model | `gpt-5.6-luna`, high reasoning effort |
| Task | `dynaconf__dynaconf-1225`, dynaconf at base commit `39acdee`, history pruned, graded in a Python 3.12 venv |
| Verdict | PASS, clean. No answer fetch over network or git history |
| Result | `FAIL_TO_PASS` green, in-file `PASS_TO_PASS` stable. The two module-path settings-loader tests that broke every failing model here are green |
| Cost | 10,120,493 tokens (input 10,081,964 at 98% cache hit, output 38,529 with 15,933 reasoning), 80 tool calls, 25 files touched, 880s, $1.4605 at gpt-5.6-luna API list price |

Priced from the [shared pricing table](/guides/): $0.2457 uncached input, $0.9836 cache read, $0.2312 output.

## What it did

luna took the same reading of the task every model here took.
The bug is that a loader cannot be pointed at a named file, and the fix threads an identifier through the loader stack.
So luna wrote the broad refactor: an identifier through the format loaders (`toml`, `yaml`, `json`, `ini`, `redis`, `env`), the loader dispatch, `base.py`, and on into `cli.py`, `validator.py`, and `parse_conf.py`.
Twenty-five files touched, right in the range of the [nineteen-edit gpt-5.5 run](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/) and the [twenty-two and twenty-three-file claude runs](/experiments/2026/07/13/14-35-dynaconf-opus-offline-regresses-green-test/).

Breadth is not what separated it.
What separated it is that luna's version of the refactor handled the module-path loader variant.
The tests that stayed red for gpt-5.4-mini, gpt-5.5, sonnet, and opus were `test_load_using_settings_loader_with_one_env_named_file_module_path` and its `_multi_env` sibling.
Every failing model left those two red because their identifier plumbing did not carry through the module-path load.
luna's did, so on grading the fail-to-pass targets turned green and nothing that was green went red.

## Why it is the finding

The [earlier open-door runs](/experiments/2026/07/13/12-20-dynaconf-closed-sorts-the-models/) could not tell capability from network access, because a strong model would just fetch the merged pull request and "pass".
With both doors shut, the leaderboard finally measures the fix on its merits, and on the merits dynaconf-1225 turned out to be a task nothing in the comparison could solve until this run.

gpt-5.6-luna is the model that clears the bar.
It is also, and this is the part worth sitting with, the cheapest model in the group.
It solved for $1.46 while [gpt-5.5 spent $4.49 to fail](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/), [sonnet spent $10.32 to fail](/experiments/2026/07/13/14-25-dynaconf-sonnet-offline-honest-broad-fail/), and [opus spent $47.18 to fail and break a test that started green](/experiments/2026/07/13/14-35-dynaconf-opus-offline-regresses-green-test/).
On this task the money did not buy the fix.
The newer model did.

This is the honest bar tomo is measured against, and it is a real bar now.
A capable scaffold on a capable model can solve dynaconf-1225 from the code alone.
The tomo work that matters is making sure tomo, on whatever model, reaches for the fix cleanly and does not [run away digging for the answer](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/) or ship a broad edit that breaks working behavior.

## Reproduce it

```bash
bash ~/data/evals/codex-real/run_offline.sh dynaconf__dynaconf-1225 gpt-5.6-luna high
# Expect: VERDICT: PASS and fairness: CLEAN.
```
