---
title: "codex on gpt-5.6-luna inside the faithful container: thirteen files, two of five, another false green"
linkTitle: "codex + gpt-5.6-luna on dynaconf-1225"
description: "The same faithful SWE-bench-Live container, now driving the real Codex CLI on gpt-5.6-luna through the subscription bridge, on the same unsolved task dynaconf-1225. This is the first of three luna notes that hold the model fixed and swap the harness. Codex runs for thirteen minutes across 75 calls, writes a 579-line patch over 13 source files, passes two of the five hidden fail-to-pass tests with no regressions, and tells us its own 227 focused tests passed. The three it fails all live on one seam: loading an env-named settings file addressed by a Python module path."
date: 2026-07-22T22:40:00+07:00
---

This is the first of three single-run notes that hold the model fixed at `gpt-5.6-luna` and swap the harness driving it, on one unsolved task, `dynaconf__dynaconf-1225`.
It is the luna companion to the earlier [sol three-way]({{< relref "20-20-swebench-live-codex-gpt56sol-dynaconf-1225" >}}), same container, same task, a different model on the other side of the subscription bridge.
This note is codex.
The two that follow are tomo's [oi engine]({{< relref "23-00-swebench-live-tomo-oi-gpt56luna-dynaconf-1225" >}}) with the LSP-backed context pack, and [pi]({{< relref "22-50-swebench-live-pi-gpt56luna-dynaconf-1225" >}}).

The environment is the faithful per-instance container from the [deepseek three-way]({{< relref "18-45-faithful-swebench-live-container-deepseek" >}}): dynaconf installed editable at `/testbed` on its own Python 3.9.22, the agent boxed on a no-egress internal network, and the upstream apply-test-grade flow run offline in a fresh `--network none` container.

## The task

The prompt is a porting checklist, thirteen items, wrapped in a plain instruction to edit source in place and make the smallest change that fixes the issue:

```
Ports from #1204 to master
- [x] Insert token (and related tests)
- [x] load_file source_metadata
- [x] populate_obj internal attr
- [x] `--json` on list must not echo
- [x] New way to find Django settings instance
- [x] json.dumps defaults to repr on CLI
- [x] settings_loader must load multiple environments (also build_env_list)
- [x] env_loader must allow multiple prefixes
- [x] *_loader must take identifer param
- [x] redis_loader fix for None prefix
- [x] Validator set call must take identifier
- [x] all tests from #1204
- [x] Docs for all the above
```

This is not a one-line bug, it is a feature port with a dozen moving parts.
The hidden grade is five `FAIL_TO_PASS` tests in `tests/test_settings_loader.py`.
The pass-to-pass guard in this luna round is the four other tests in that same file, narrower than the 522-test guard the sol run used, so read the no-regression line as scoped to `test_settings_loader.py` rather than the whole suite.
The fail-to-pass verdict is unaffected by that scope, and it is the number this note is about.

## Running the subscription model through the container

`gpt-5.6-luna` is a paid model reached through the ChatGPT subscription, not a metered key, so the run chains three containers on the same no-leak topology:

```
codex (swelive-int, no egress)
  -> usage proxy  (swelive-int)         captures Responses token usage
  -> codex bridge (swelive-int+egress)  injects the subscription token, pins model+effort
  -> chatgpt.com/backend-api/codex/responses
```

Codex speaks `POST /v1/responses` natively, so the bridge forwards the request verbatim, adds the subscription OAuth token, and pins the model to `gpt-5.6-luna` at effort `high`.
Codex can reach nothing but the proxy, only the bridge has egress, and only to the codex backend.
The token counts below are real usage read off each response, from inside the internal network.

## What codex did

Codex ran for thirteen minutes and five seconds across 75 model calls.
Its context grew the way a single linear thread does, and prompt caching absorbed most of that growth: of 7.28M total prompt tokens across the run, 6.42M were cache hits, so 88 percent of the input was reused rather than re-encoded.

The patch is 579 lines across 13 files, all source, no tests, no docs.
It touched `dynaconf/base.py`, `parse_conf.py`, the loaders, `cli.py`, `validator.py`, and the Django contrib path, which is the right neighborhood for most of the checklist.
Its closing message is confident and specific:

```
Implemented the requested source changes across loaders, environment handling,
CLI JSON output, Django integration, validation metadata, populate_obj, and the
new @insert token.

Verification:
- Focused tests passed: 227 tests.
- Ruff and diff checks passed.
- Full suite collection is blocked by pre-existing functional-test import/Django
  configuration conflicts.
```

## The grade: two of five, no regressions, resolved false

| metric | value |
| --- | --- |
| model calls | 75 |
| prompt tokens | 7,275,003 (6,422,528 cache-hit, 852,475 cache-miss) |
| output tokens | 26,865 (15,305 reasoning) |
| cache hit rate | 88.3% |
| wall clock | 13:05 |
| peak RSS | 122 MB |
| patch | 579 lines, 13 files (source only) |
| FAIL_TO_PASS | 2 / 5 passed |
| PASS_TO_PASS | 4 / 4 passed (scoped to test_settings_loader.py) |
| resolved | **false** |

Of the five hidden fail-to-pass tests, codex passed the base `test_load_using_settings_loader` and the `one_env_named_file_file_path_multi_env` variant, and failed the `multi_temporary_env` variant and both `module_path` variants.
The three failures cluster on one seam: loading an env-named settings file addressed by a Python module path (`test_load_using_settings_loader_with_one_env_named_file_module_path` and its multi-env sibling), and stacking multiple temporary environments.
Codex got the file-path cases right and the module-path cases wrong, which means its `@insert` and merge work is correct and its Python-module-path loader plumbing is incomplete.
This is a genuine partial: a broad, mostly-correct port that misses one related corner.

Codex's own "227 focused tests passed" is a false green, the same shape the sol run showed.
Its in-sandbox pytest was green because it exercised the paths codex implemented.
The hidden fail-to-pass tests exercise the module-path loading it never wired, and codex never saw them, so its self-report is honest about what it ran and wrong about what the task asked.
An agent grading itself against the surface it chose to cover cannot discover the requirement it failed to infer.

## Cost

There is no per-run line item here: `gpt-5.6-luna` is billed by the flat ChatGPT subscription, not per token, so the marginal dollar cost of this attempt is not metered.
To give the token volume a dollar shape anyway, priced at an assumed GPT-5-class rate of $1.25 per million cache-miss input, $0.125 per million cache-hit input, and $10 per million output, this run would bill about **$2.14**: roughly $1.07 for the 0.85M fresh input tokens, $0.80 for the 6.42M cached ones, and $0.27 for the 27K output tokens.
Read that as a statement about token volume, not a bill.
The real headline is 7.28M prompt tokens and 27K output tokens over thirteen minutes, and an 88 percent cache hit rate that keeps the fresh input small even as the thread grows.

## What this run says

Codex on luna lands where it landed on sol: two of five, no regressions, a strong partial that misses the Python-module-path loader.
It is fast and cheap in output tokens, front-loaded in prompt tokens, and confidently wrong about its own result.
The next two notes drive the same model through leaner harnesses and read what changes when the retrieval is symbol-anchored instead of self-directed.
