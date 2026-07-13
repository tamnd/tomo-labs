---
title: "dynaconf on opus with the door shut: $47 to fail and break a test that was green"
linkTitle: "dynaconf opus offline"
description: "The honest opus run on dynaconf-1225 is the most expensive fail in this comparison and the only run that ends with the repo worse than it started. Opus writes a twenty-three-file identifier refactor, misses the module-path loader like everyone else, and on top of that turns a previously green settings-loader test red. It spends $47.18. Breadth and spend are not the lever here. Not regressing working behavior is."
date: 2026-07-13T14:35:00+07:00
---

This is one run: the real `claude` CLI on the user's subscription, model `claude-opus-4-8`, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier, under the both-doors-closed harness.
It failed, cleanly, and it is the re-run of the [opus run that passed by fetching the answer](/experiments/2026/07/13/12-40-dynaconf-opus-answer-fetch/) with the network door now closed.

It is also the run worth studying most, because it fails in a way none of the others do.
Every other honest fail on this task left the codebase where it found it: the fix-to-pass tests stayed red, the already-green tests stayed green.
Opus turned a green test red.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13, on the user's Claude subscription |
| Tool | real `claude` CLI in a sandbox that denies all network except the local model bridge |
| Model | `claude-opus-4-8`, the flagship |
| Task | `dynaconf__dynaconf-1225`, dynaconf at base commit `39acdee`, history pruned, graded in a Python 3.12 venv |
| Verdict | FAIL, clean. No network fetch of an answer found in the trace |
| Result | 5 failed, 4 passed. `PASS_TO_PASS regressed: test_load_using_settings_loader_with_one_env_named_file_file_path`, a test that was green at the base commit |
| Cost | 20,686,551 tokens (fresh input 12,459, cache read 20,229,499, cache write 296,763, output 147,830), 194 turns, 97 tool calls (Bash 44, Read 28, Edit 25), 25 edits across 13 files, $47.1827 at claude-opus-4-8 API list price |

Cost splits as $0.1869 fresh input, $30.3442 cache read, $5.5643 cache write, $11.0872 output.
Cache read alone, the context re-sent every turn, is $30 of the $47.

## What it did

Opus took the same reading as every model here and wrote the broad refactor: an identifier threaded through the loader stack, twenty-five edits across thirteen files, right in the range luna, sonnet, and gpt-5.5 all landed in.
Denied the network, it wrote its own change, and the [analyzer](/guides/) confirms no answer was fetched.

Then two things went wrong at once.
It missed the module-path loader variant, so `test_..._module_path` and `_multi_env` stayed red, the same miss as everyone below the gpt-5.6 line.
And its change to the file-path load path broke `test_load_using_settings_loader_with_one_env_named_file_file_path`, a test that passed at the base commit before opus touched anything.
Five failed, four passed.
The refactor did not just fail to fix the target, it damaged working behavior on the way.

## Why this is the sharp lesson

Line the honest both-doors-closed runs up and the naive reading is that this is about model size or effort.

| Model | Files | Tokens | Cost | Verdict |
|---|---|---|---|---|
| [gpt-5.6-luna](/experiments/2026/07/13/14-15-dynaconf-gpt-5.6-luna-offline-first-clean-solve/) | 25 | 10.1M | $1.46 | PASS |
| gpt-5.4-mini | 9 | 4.83M | $0.78 | FAIL |
| [gpt-5.5](/experiments/2026/07/13/14-01-dynaconf-gpt-5.5-offline-honest-fail/) | 19 | 6.08M | $4.49 | FAIL |
| [sonnet-5](/experiments/2026/07/13/14-25-dynaconf-sonnet-offline-honest-broad-fail/) | 22 | 23.5M | $10.32 | FAIL |
| opus-4.8 | 23 | 20.7M | $47.18 | FAIL, regressed a green test |

Breadth is not the lever: luna went the widest of anyone, twenty-five files, and passed.
Spend is not the lever: opus spent the most of anyone, $47, and produced the worst result, a repo that is worse than the base commit.
The one thing that separates the pass from the fails is whether the broad change kept the green tests green and carried the identifier all the way through, and opus is the clearest counter-example: it broke a green test.

That points at a tomo lever that is model-independent.
tomo's [convergence guard](/experiments/2026/07/13/10-05-dynaconf-tomo-guard-vs-pi-runaway/) stops it running away searching for a fix, but nothing yet stops it shipping a broad edit that regresses working behavior.
A do-no-harm check, running the in-repo tests the change plausibly touched and treating a green-to-red flip as a stop signal, is exactly what opus lacked.
It would not have penalized luna, whose broad refactor kept every green test green.
It would have caught opus before the twenty-third file.
That lesson is recorded for tomo in the [experiment journal](/experiments/2026/07/13/14-55-dynaconf-doors-closed-lessons-for-tomo/).

## Reproduce it

```bash
bash ~/data/evals/codex-real/run_offline_claude.sh dynaconf__dynaconf-1225 claude-opus-4-8
go run ./cmd/lab claude analyze \
  ~/.claude/projects/*claude-opus-offline-work/*.jsonl
# Expect: VERDICT: FAIL with a PASS_TO_PASS regression, fair: no answer fetch.
```
