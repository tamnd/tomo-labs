---
title: "dynaconf on sonnet with the door shut: it stopped fetching and failed honestly"
linkTitle: "dynaconf sonnet offline"
description: "The earlier sonnet run passed dynaconf-1225 by fetching the merged pull request. Close the network door and run it again and the fetch is gone. Sonnet writes its own broad twenty-two-file identifier refactor, reaches no answer, and fails on the same two module-path loader tests that stop every model here. It spends $10.32 to fail. This is the honest sonnet number, and it is a fail."
date: 2026-07-13T14:25:00+07:00
---

This is one run: the real `claude` CLI on the user's subscription, model `claude-sonnet-5`, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier, this time under the both-doors-closed harness.
It failed, cleanly.
It is the re-run of the [sonnet run that passed by fetching the answer](/experiments/2026/07/13/12-35-dynaconf-sonnet-answer-fetch/), with the network door now closed, and closing that door changed the verdict.

The first sonnet run "passed" because it could reach GitHub and run `gh pr diff 1225`.
Deny it the network and it has to solve the bug from the code, the same as every other model in this comparison, and it does not.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13, on the user's Claude subscription |
| Tool | real `claude` CLI in a sandbox that denies all network except the local model bridge |
| Model | `claude-sonnet-5` |
| Task | `dynaconf__dynaconf-1225`, dynaconf at base commit `39acdee`, history pruned, graded in a Python 3.12 venv |
| Verdict | FAIL, clean. No network fetch of an answer found in the trace |
| Result | 3 failed, 6 passed. The module-path settings-loader targets stayed red |
| Cost | 23,496,122 tokens (fresh input 39,946, cache read 22,949,534, cache write 381,144, output 125,498), 235 turns, 128 tool calls (Bash 64, Read 39, Edit 24, one subagent), 24 edits across 12 files, $10.3165 at claude-sonnet-5 API list price |

Cost splits as $0.1198 fresh input, $6.8849 cache read, $1.4293 cache write, $1.8825 output.

## What it did

Denied the network, sonnet did the honest thing and wrote a fix.
It read the failing tests and the loader stack, took the same reading everyone took, and threaded an identifier through the format loaders, the dispatch, and `base.py`, twenty-four edits across twelve files.
It even spawned a subagent to work part of it.
This is a real attempt at the fix, not a copied diff, and the [analyzer](/guides/) confirms it: no command in the trace reached an answer.

And it lands where every honest attempt on this task lands.
On grading the module-path loader tests stayed red, the same `test_..._module_path` and `_multi_env` pair that stopped [gpt-5.4-mini](/experiments/2026/07/13/13-49-dynaconf-gpt-5.4-mini-offline-honest-fail/) and [gpt-5.5](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/).
Sonnet's broad refactor did not carry the identifier through the module-path load, so those tests never went green.
Three failed, six passed, no solve.

## What the pair of sonnet runs says

| Sonnet run | Doors | Verdict | Cost | How |
|---|---|---|---|---|
| [open network](/experiments/2026/07/13/12-35-dynaconf-sonnet-answer-fetch/) | git history closed, network open | "pass" | $1.29 | `gh pr diff 1225`, applied the answer |
| this one | both closed | FAIL, clean | $10.32 | wrote its own broad refactor, missed module-path |

The open-door "pass" cost $1.29 because fetching a diff is cheap.
The honest attempt cost $10.32 because actually reasoning about the loader stack over 235 turns is not.
The eight-fold cost gap is the price of doing the work instead of looking up the answer, and the reward for doing the work here is an honest fail.

That is the whole reason the harness closes both doors.
Left open, the network turned a fail into a cheap "pass" and would have ranked sonnet above models that solved less but stayed inside the sandbox.
Closed, sonnet sits exactly where its fix earns it: with the models that could not solve dynaconf-1225, below [the gpt-5.6 family that could](/experiments/2026/07/13/14-15-dynaconf-gpt-5.6-luna-offline-first-clean-solve/).

For tomo there is nothing to imitate and one thing to keep.
tomo already runs with no route to GitHub, so it never had sonnet's open door.
Its honest fails are cheap, and this run is the reminder of what the alternative costs: ten dollars and twenty-three million tokens to reach the same wall a newer model clears for a dollar and a half.

## Reproduce it

```bash
bash ~/data/evals/codex-real/run_offline_claude.sh dynaconf__dynaconf-1225 claude-sonnet-5
go run ./cmd/lab claude analyze \
  ~/.claude/projects/*claude-sonnet-offline-work/*.jsonl
# Expect: VERDICT: FAIL, fair: no network fetch of an answer found in the trace.
```
