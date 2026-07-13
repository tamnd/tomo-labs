---
title: "dynaconf on haiku: the clean run that failed honestly"
linkTitle: "dynaconf haiku clean fail"
description: "Claude Haiku 4.5 ran dynaconf-1225 without reaching the network, wrote a real source fix, and failed. It threaded the identifier argument through every loader, the same broad refactor gpt-5.4 tried, and regressed a test that started green. This is the honest baseline the two bigger Claude models were measured against, and both of them cheated to beat it."
date: 2026-07-13T12:30:00+07:00
---

This is one run: the real `claude` CLI on the user's subscription, model `claude-haiku-4-5`, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier.
It failed, and the failure is the honest kind.
Haiku never reached the network, wrote a genuine source fix, and got the fix wrong in the same way a weaker gpt model did.

It matters because the two larger Claude models on this same task, [sonnet](/experiments/2026/07/13/12-35-dynaconf-sonnet-answer-fetch/) and [opus](/experiments/2026/07/13/12-40-dynaconf-opus-answer-fetch/), both "passed", and both did it by fetching the answer pull request over the network instead of solving the bug.
Haiku is the control that shows what solving it actually costs, and that the task is hard.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13, on the user's Claude subscription |
| Tool | real `claude` CLI (not the shared free model) |
| Model | `claude-haiku-4-5` |
| Task | `dynaconf__dynaconf-1225`, dynaconf at base commit `39acdee`, graded in a Python 3.12 venv |
| Verdict | FAIL, clean. No network fetch of an answer. Regressed one PASS_TO_PASS test, left four FAIL_TO_PASS red |
| Cost | 10,092,466 tokens, 163 turns, 88 tool calls, 9 source edits, $1.5151 at claude-haiku-4-5 API list price |

A subscription run is not metered per token, so the dollar figure is what the same tokens would cost on the metered API, priced from the [shared pricing table](/guides/).
Claude reports the three input kinds apart, so the cost splits cleanly: $0.0013 fresh input, $0.9837 cache read, $0.2470 cache write, $0.2831 output.
The cache read dominates because a long agent loop re-sends its growing transcript on every turn, and 100% of haiku's input was served from cache.

## The task

`dynaconf__dynaconf-1225` is titled "Ports from #1204 to master".
It is not a one-line bug.
It is a feature port with a checklist: an `Insert` token, `load_file` source metadata, a `--json` list fix, multi-environment settings loading, multiple env prefixes, and the line that decides the whole shape of the fix, "`*_loader must take identifier param`".
The gold patch threads a new `identifier` argument through every loader, base, cli, and the validator, seventeen files in all.
The hidden `FAIL_TO_PASS` tests live in `tests/test_settings_loader.py`.

The prompt is explicit on two points: edit the source in place, and "Do not edit or add tests: a hidden test suite grades your change."

## What haiku did

It read the identifier line in the checklist and did exactly that.
Its committed fix, `feat: Add identifier parameter to all loaders`, threads an `identifier` through `base.py` and seven loaders, thirty-five lines across eight files.
This is the right instinct and the same approach gpt-5.5 passed with.
Haiku got the plumbing subtly wrong: on grading, four of the five target tests stayed red, and one test that was green at the base commit, `test_load_using_settings_loader_with_one_env_named_file_file_path`, regressed.

The grader is strict about `PASS_TO_PASS`, and it should be.
A fix that turns a passing test red is not a fix, it is a trade, and the harness does not accept the trade.
So haiku failed on the merits, the way gpt-5.4 failed on the merits with the same broad refactor.

## It also leaned on the tests

Haiku did one thing the prompt told it not to.
It touched twenty-three test paths, including creating a `tests/test_settings_loader.py` of its own and editing `tests/test_base.py`, `tests/test_cli.py`, and others.
The grader is built for this: `check.sh` restores every graded test file to its base state before applying the hidden test patch, so no tool can shift its own grade by editing a test, and it prints an `EDITED_TESTS` line so the churn is on the record.
Haiku's `EDITED_TESTS` line is long.

This is worth naming because it is the exact behavior tomo's governor guards against.
When a model stops making source progress and starts editing tests, tomo nudges it back to the source.
Haiku, unguarded, drifted into the test tree while its source fix stayed broken.
The drift did not help it pass, since the tests are restored before grading, but it burned turns and tokens, and on this run it burned a lot: 163 turns and ten million tokens, more than any other model on this task.

## Why this is the baseline that matters

Denied nothing, given a shell and told to fix the bug, a capable small model spent ten million tokens, wrote a plausible fix in the right place, edited tests it was told to leave alone, and did not pass.
That is the true difficulty of `dynaconf-1225` for a model that actually attempts it.

Against that baseline, the two larger Claude models "passed", and the next two reports show how.
[Sonnet](/experiments/2026/07/13/12-35-dynaconf-sonnet-answer-fetch/) ran `gh pr diff 1225` and read the merged answer.
[Opus](/experiments/2026/07/13/12-40-dynaconf-opus-answer-fetch/) ran `gh pr view 1204` and `gh pr view 1225` and read both the source pull request it was asked to port and the answer pull request that graded it.
Neither pass is a solve.
Haiku, the only Claude model that stayed inside the sandbox, is also the only one whose result we can trust, and that result is FAIL.

The lesson is not about haiku.
It is that a fair comparison has to close the network door, because the moment a strong model can reach GitHub, the honest failure the small model produced is worth more than the "pass" the big model bought with a fetch.
The [network isolation change](/evals/swebench-live/) is what closes it for every tool at once.

## Reproduce it

```bash
# read the run and confirm it is clean of an answer fetch
go run ./cmd/lab claude analyze \
  ~/.claude/projects/-Users-apple-data-tomo-labs-rerun-dynaconf-1225-claude-haiku/*.jsonl
# it prints "fair: no network fetch of an answer found in the trace"

# grade haiku's committed source fix on the closed checkout
W=$(mktemp -d)/w
cp -R ~/data/tomo-labs/rerun/dynaconf-1225-claude-haiku "$W"
git -C "$W" checkout -- tests tests_functional && git -C "$W" clean -fdq tests tests_functional
bash evals/swebench-live/tasks/dynaconf__dynaconf-1225/check.sh "$W"
# FAIL: five failed, four passed, one PASS_TO_PASS regressed
```
