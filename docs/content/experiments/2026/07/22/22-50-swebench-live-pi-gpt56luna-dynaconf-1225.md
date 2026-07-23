---
title: "pi on gpt-5.6-luna inside the faithful container: same two of five, twice the reasoning, no regressions"
linkTitle: "pi + gpt-5.6-luna on dynaconf-1225"
description: "The second luna note on dynaconf-1225: the pi CLI driving gpt-5.6-luna through the subscription bridge, same faithful container. Pi runs for twenty-four minutes across 88 calls, writes a 536-line patch over 13 source files, and lands exactly where codex did, two of the five hidden fail-to-pass tests with no regressions. It burns nearly twice codex's reasoning tokens to get there and misses the same Python-module-path loader, which says the wall on this task is the model's, not the harness's."
date: 2026-07-22T22:50:00+07:00
---

This is the second of three luna notes that hold `gpt-5.6-luna` fixed and swap the harness on one unsolved task, `dynaconf__dynaconf-1225`.
The first was [codex]({{< relref "22-40-swebench-live-codex-gpt56luna-dynaconf-1225" >}}).
This one is pi.
The last is tomo's [oi engine]({{< relref "23-00-swebench-live-tomo-oi-gpt56luna-dynaconf-1225" >}}).

The container is the same faithful per-instance image from the [deepseek three-way]({{< relref "18-45-faithful-swebench-live-container-deepseek" >}}), the same no-egress topology, the same offline apply-test-grade flow, and the same subscription bridge pinning the model to `gpt-5.6-luna` at effort `high`.
Pi speaks OpenAI chat completions, so the proxy sees an `OpenAI/JS` client and the bridge translates chat completions to and from the Responses wire the ChatGPT backend expects.

## The task

Same porting checklist as the codex note, thirteen items from issue #1204, graded on five `FAIL_TO_PASS` tests in `tests/test_settings_loader.py`.
The pass-to-pass guard in this luna round is the four other tests in that same file, so the no-regression line is scoped to `test_settings_loader.py`.

## What pi did

Pi ran for twenty-four minutes and sixteen seconds across 88 model calls, and its patch is 536 lines over 13 files:

```
dynaconf/base.py            dynaconf/loaders/redis_loader.py
dynaconf/cli.py             dynaconf/loaders/toml_loader.py
dynaconf/loaders/__init__.py  dynaconf/loaders/vault_loader.py
dynaconf/loaders/env_loader.py dynaconf/loaders/yaml_loader.py
dynaconf/loaders/ini_loader.py dynaconf/utils/__init__.py
dynaconf/loaders/json_loader.py dynaconf/utils/parse_conf.py
dynaconf/validator.py
```

That is the same neighborhood codex chose: every loader, `base.py`, `cli.py`, `validator.py`, the parse helpers.
Notably it does not touch `dynaconf/loaders/py_loader.py`, which is where the tests it fails actually live.
Its closing message is measured:

```
- Python loader support for multiple environments
- Protection of internal attributes during populate_obj

Validated with targeted tests and compilation checks.
```

## The grade: two of five, no regressions, resolved false

| metric | value |
| --- | --- |
| model calls | 88 (1 usage-less row) |
| prompt tokens | 6,587,463 (5,304,320 cache-hit, 1,283,143 cache-miss) |
| output tokens | 59,172 (48,728 reasoning) |
| cache hit rate | 80.5% |
| wall clock | 24:16 |
| peak RSS | 179 MB |
| patch | 536 lines, 13 files (source only) |
| FAIL_TO_PASS | 2 / 5 passed |
| PASS_TO_PASS | 4 / 4 passed (scoped to test_settings_loader.py) |
| resolved | **false** |

Pi passed the base `test_load_using_settings_loader` and the `file_path_multi_env` variant, and failed `multi_temporary_env` and both `module_path` variants.
That is the exact split codex produced.
Two different agents, same paid model, same three failures on the same seam: loading an env-named settings file addressed by a Python module path.
Pi's self-report of "validated with targeted tests" is the same false green in a quieter register, honest about the compilation checks it ran and blind to the module-path requirement it never covered.

## What pi spent to get there

Pi and codex land on the same score, and the interesting difference is what each paid for it.
Codex reached two of five in 13 minutes with 27K output tokens.
Pi took 24 minutes and 59K output tokens, of which 49K were reasoning, to reach the same two of five.
Pi's prompt volume is a shade lower (6.59M vs 7.28M) and its cache hit rate a little worse (80.5% vs 88.3%), so pi is not cheaper on input and is nearly twice as expensive on output for an identical result.
The two harnesses converge on the same answer because the answer is bounded by the model: `gpt-5.6-luna` does not, in either loop, discover that the module-path loader needs the same multi-environment treatment as the file-path loader.

## Cost

There is no per-run line item: `gpt-5.6-luna` is billed by the flat ChatGPT subscription, not per token.
Priced at an assumed GPT-5-class rate of $1.25 per million cache-miss input, $0.125 per million cache-hit input, and $10 per million output, purely to give the token volume a shape, pi's run would bill about **$2.86**: roughly $1.60 for the 1.28M fresh input tokens, $0.66 for the 5.30M cached ones, and $0.59 for the 59K output tokens.
That is a third more than codex's $2.14 reference for the same score, driven by pi's heavier reasoning-token burn.
Read the dollar figure as token volume, not a bill.

## What this run says

Two independent harnesses, same model, same two of five, same missed loader.
When the strongest general agents available both stall at the same corner of the same task, that corner is a model limitation, not a harness one.
The third note drives the same model through tomo's oi engine and its LSP-backed context pack, which points the model at the right file for far fewer tokens, and shows what happens when a lean loop lets a confident model rewrite too much.
