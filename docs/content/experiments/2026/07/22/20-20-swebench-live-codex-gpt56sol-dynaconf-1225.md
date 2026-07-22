---
title: "codex on gpt-5.6-sol inside the faithful container: the broadest patch, two of five, and a false green"
linkTitle: "codex + gpt-5.6-sol on dynaconf-1225"
description: "The same faithful SWE-bench-Live container that ran the deepseek three-way, now running the real Codex CLI on gpt-5.6-sol through a subscription bridge, on the same unsolved task dynaconf-1225. Codex speaks the Responses wire natively, so the bridge forwards it verbatim to the ChatGPT backend and pins the model and effort. It runs for twenty-five minutes across 126 model calls, writes the largest patch of anyone (1156 lines over 26 files, source and tests and docs), and passes two of the five hidden fail-to-pass tests with no regressions. Then it tells us it passed 537 tests. It did not. This is a close read of how a strong model produces a confident wrong answer, and of the one grader sharp edge this run exposed and fixed."
date: 2026-07-22T20:20:00+07:00
---

This is the first of three single-run notes on one unsolved task, `dynaconf__dynaconf-1225`, each with a different harness driving the same paid model, `gpt-5.6-sol`.
The environment is the faithful SWE-bench-Live container built for the [deepseek three-way]({{< relref "18-45-faithful-swebench-live-container-deepseek" >}}): the per-instance prebuilt image, dynaconf installed editable at `/testbed` on its own Python 3.9.22, the raw problem statement as the only prompt, and the upstream apply-test-grade flow run offline in a fresh `--network none` container.
This note is codex.
The two that follow are tomo's own [agent engine]({{< relref "20-21-swebench-live-tomo-agent-gpt56sol-dynaconf-1225" >}}) and its [oi engine]({{< relref "20-22-swebench-live-tomo-oi-gpt56sol-dynaconf-1225" >}}).

## The task

The prompt is a porting checklist, thirteen items, verbatim from the issue:

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

The gold fix is 961 lines across 17 files.
This is not a one-line bug, it is a feature port with a dozen moving parts, and the hidden grade is five `FAIL_TO_PASS` tests in `tests/test_settings_loader.py` plus 522 `PASS_TO_PASS` tests that must stay green.

## Running a subscription model through the faithful container

`gpt-5.6-sol` is a paid model, and the way we have access to it is the ChatGPT subscription, not a metered key.
So the run chains three containers on the same no-leak topology the deepseek run used:

```
codex (swelive-int, no egress)
  -> usage proxy  (swelive-int)         captures Responses token usage
  -> codex bridge (swelive-int+egress)  injects the subscription token, pins model+effort
  -> chatgpt.com/backend-api/codex/responses
```

Codex speaks `POST /v1/responses` natively, so the bridge forwards the request verbatim, adds the subscription OAuth token, sets the model to `gpt-5.6-sol` and the effort to `high`, and refreshes the token if the backend says it is stale.
Codex can reach nothing but the proxy, only the bridge has egress, and only to the codex backend.
The one wrinkle worth recording: the bridge is the `lab bridge` subcommand, and the lab binary refuses to start without a container runtime it never uses in this mode, so the bridge container runs with `LAB_RUNTIME=noop` to satisfy that check and serve HTTP and nothing else.

The token counts here are real usage read off the response's `response.completed` event, per call, from inside the internal network.

## What codex did

Codex ran for twenty-five minutes and twenty seconds, 126 model calls, and its context grew monotonically: the first call carries 8,460 prompt tokens, the last carries 189,207, and prompt caching absorbs most of the growth, so by the end each call reuses ~180,000 cached tokens and pays for a few thousand fresh ones.
That shape is why the aggregate prompt token count is enormous even though the conversation is one linear thread.

The patch it produced is the largest of the three by a wide margin: 1156 lines across 26 files.
It touched the source the task is about (`dynaconf/base.py`, `parse_conf.py`, all seven loaders, `cli.py`, `validator.py`, `contrib/django_dynaconf_v2.py`), and it also edited five docs files and seven test files.
That breadth matters twice below, once for the grade and once for the grader.

Its closing message is confident and specific:

```
Implemented the #1204 ports to master, including:
 - @insert INDEX VALUE token
 - Loader/source identifiers and load_file metadata
 - Multiple environments and environment prefixes
 ...
Verification:
 - 537 tests passed, 2 skipped, 1 expected failure
 - Ruff and formatting checks passed
```

## The grade: two of five, no regressions, resolved false

| metric | value |
| --- | --- |
| model calls | 126 |
| prompt tokens | 16,196,133 (7,973,632 cache-hit, 8,222,501 cache-miss) |
| output tokens | 37,891 (16,877 reasoning) |
| wall clock | 25:20 |
| peak RSS | 121 MB |
| patch | 1156 lines, 26 files (source + tests + docs) |
| FAIL_TO_PASS | 2 / 5 passed |
| PASS_TO_PASS | 522 / 522 passed |
| resolved | **false** |

Of the five hidden fail-to-pass tests, codex passed the base `test_load_using_settings_loader` and the `one_env_named_file_file_path_multi_env` variant, and failed the `multi_temporary_env` variant and both `module_path` variants.
The failures cluster on one seam: loading an env-named settings file addressed by a Python module path, and stacking multiple temporary environments.
The assertions that fail are the list-and-dict merge ones, `assert settings.A_LIST == [0, 1, 2]` and `assert settings.A_DICT.teams == ["dev"]`, so codex's insert-token and merge implementation is correct for the file-path cases and incomplete for the module-path and multi-temporary cases.
This is a genuine partial: a strong, broad port that gets most of the surface and misses two related corners.

Codex's own "537 tests passed" is a false green.
Its in-sandbox pytest was green because codex wrote its own tests alongside the port, and those tests exercise the paths it implemented.
The hidden fail-to-pass tests exercise the module-path and multi-temporary paths it got wrong, and codex never saw them, so its self-report is honest about what it ran and wrong about what the task asked.
The lesson is not that codex lied, it is that an agent grading itself against tests it authored cannot discover the requirements it failed to infer.

## The grader sharp edge this run exposed

The first grade of this run returned zero of five, every fail-to-pass test `MISSING`, which is the grader saying the test was never collected.
The cause was not codex, it was the grader, and it is worth documenting because it would have silently understated any agent that edits test files.

The upstream flow resets the test files the hidden patch touches back to base, then applies the hidden test patch.
The reset was one command, `git checkout BASE -- <all test files>`.
One of those files, `tests/test_settings_loader.py`, is new in the hidden patch and does not exist at base, so `git checkout` aborted the whole multi-path checkout with a pathspec error and reset nothing.
Codex had edited `tests/test_inspect.py` as part of its broad port, that edit therefore survived, and `git apply` of the hidden test patch is atomic, so one conflicting hunk in `test_inspect.py` rejected the entire test patch and no hidden tests were written at all.
Gold passes this grader because gold never touches test files, so the reset had nothing to clean and the hidden patch applied to pristine files.

The fix is to reset each graded test file individually: restore the ones that exist at base, delete the agent-created ones so the hidden patch can recreate them.
After that fix the hidden patch applies cleanly, the 522 pass-to-pass tests all run, and codex's real grade is two of five.
The fair number is the one in the table.
The harness change ships with this note.

## Cost

Under the ChatGPT subscription the marginal cost of this run is zero, the tokens are covered by the flat plan, and that is the honest headline for a subscription model.
`gpt-5.6-sol` has no public per-token list price, so a metered figure is hypothetical.
Priced at an assumed GPT-5-class rate of $1.25 per million input, $0.125 per million cached input, and $10 per million output, purely to give the token volume a shape, codex's run would be about $11.66, almost all of it the 8.2 million cache-miss input tokens its twenty-five-minute linear context accumulated.
Read that as a statement about token volume, not a bill.

## What this run says

Codex is the strongest single result on this task: the broadest correct coverage, two of five, and no regressions, from a model driven hard for twenty-five minutes.
It is also the most expensive by a large factor and the most confidently wrong in its self-report.
The next two notes run the same model through leaner harnesses and get less coverage for far fewer tokens, which is the trade this whole exercise is trying to measure.
