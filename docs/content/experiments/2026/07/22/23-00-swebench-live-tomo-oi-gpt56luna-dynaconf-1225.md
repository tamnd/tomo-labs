---
title: "tomo-oi on gpt-5.6-luna: the LSP pack finds the right file, the model breaks it, and a compile check that is not enough"
linkTitle: "tomo-oi + gpt-5.6-luna on dynaconf-1225"
description: "The third luna note on dynaconf-1225: tomo's oi engine driving gpt-5.6-luna, with the symbol-anchored context pack resolving symbols through pyright. The pack does its job, it points the model at loaders/__init__.py where the failing tests live, and it does it in 149K prompt tokens against codex's 7.3M and pi's 6.6M, forty times leaner. Then the model rewrites the loaders too broadly and leaves a dangling name, a NameError that fails all nine tests in the file including the four that passed at base. The oi loop's own ast.parse check waved it through because a NameError is a runtime error, not a syntax error. Zero of five, with regressions, and a clear lesson about what a compile check cannot catch."
date: 2026-07-22T23:00:00+07:00
---

This is the last of three luna notes on `dynaconf__dynaconf-1225`, holding `gpt-5.6-luna` fixed and swapping the harness.
The first two were [codex]({{< relref "22-40-swebench-live-codex-gpt56luna-dynaconf-1225" >}}) and [pi]({{< relref "22-50-swebench-live-pi-gpt56luna-dynaconf-1225" >}}), both two of five, both missing the Python-module-path loader.
This one is tomo's oi engine, the code-as-action loop, driven with the symbol-anchored context pack the [pack]({{< relref "20-22-swebench-live-tomo-oi-gpt56sol-dynaconf-1225" >}}) and LSP work built.
It is the run the whole pack and LSP effort was for, and it is the most interesting result of the three, because it fails in a way the other two do not.

The container and the bridge are unchanged from the other luna notes: faithful per-instance image, no-egress internal network, offline grade, `gpt-5.6-luna` at effort `high` through the subscription bridge.
The only new pieces are on tomo's side.

## Two harness changes this run needed

The oi loop feeds the model output back and loops until the model stops emitting code.
A strong model can keep going, and `gpt-5.6-luna` does: it front-loads enormous rounds, one response here carried 14,616 tokens of code editing a dozen files at once, and it does not converge on its own inside a sensible wall-clock.
So this run added `TOMO_MAX_ROUNDS`, an env cap that ends the loop at a clean round boundary where the model's last executed code was compile-checked, rather than snapshotting a workspace mid-edit.
The cap was set to twelve.
The model finished on its own at round five, well under the cap, so the cap did not bind here, but it is why the captured patch is a coherent 372-line diff and not a half-applied one.

The second piece is the LSP-backed context pack, and it worked exactly as designed.
The engine's stderr from inside the box:

```
oi: context pack via LSP (pyright-langserver): resolved 4/9 symbols
```

The pack resolved the task's symbols through pyright and put the right file in front of the model.
Its patch touches `dynaconf/loaders/__init__.py`, which is where `settings_loader` lives and where every failing test bottoms out.
The retrieval was not the problem.

## What tomo-oi did

Five model calls, twenty-five minutes of wall-clock (`gpt-5.6-luna` spends minutes per round on reasoning), and a 372-line patch across 12 source files:

```
dynaconf/base.py               dynaconf/loaders/redis_loader.py
dynaconf/cli.py                dynaconf/loaders/toml_loader.py
dynaconf/loaders/__init__.py     dynaconf/loaders/vault_loader.py
dynaconf/loaders/env_loader.py   dynaconf/loaders/yaml_loader.py
dynaconf/loaders/ini_loader.py   dynaconf/utils/__init__.py
dynaconf/loaders/json_loader.py  dynaconf/utils/parse_conf.py
```

The patch applied cleanly, every file, no rejected hunks.

## The grade: zero of five, and it regressed the pass-to-pass

| metric | value |
| --- | --- |
| model calls | 5 |
| prompt tokens | 148,733 (29,440 cache-hit, 119,293 cache-miss) |
| output tokens | 82,087 (47,190 reasoning) |
| cache hit rate | 19.8% |
| wall clock | 25:25 |
| peak RSS | 246 MB |
| patch | 372 lines, 12 files (source only) |
| LSP | resolved 4/9 symbols, live |
| FAIL_TO_PASS | 0 / 5 passed |
| PASS_TO_PASS | 0 / 4 passed (all four regressed) |
| resolved | **false** |

This is the worst grade of the three and the most informative.
Codex and pi each passed two of five and broke nothing.
Tomo-oi passed none and broke the four pass-to-pass tests too, so all nine tests in `test_settings_loader.py` fail, including the base `test_load_using_settings_loader` that passes at base with no patch at all.
When a patch turns a green test red, the regression is in shared code every test in the file runs through, and the grade confirms it: the failure is identical across all nine.

The failure is one line:

```
E   NameError: name 'global_mod_file' is not defined
```

The model's rewrite of `settings_loader` in `loaders/__init__.py` referenced a name it never bound.
Because `settings_loader` is the single entry point every test in the file calls, the `NameError` takes down all nine at once.
The LSP pack found the right file, the model wrote a plausible refactor of it, and the refactor was broken in a way that had nothing to do with the task's actual requirement.

## The compile check that was not enough

Here is the part worth keeping.
The oi loop validates its own edits before it moves on: each round runs `ast.parse` (equivalently `py_compile`) over the edited files, and its stdout shows the model doing exactly this, ending a round with a loop over `Path("dynaconf").rglob("*.py")` that parses each file and prints `syntax ok`.
That check passed.
`global_mod_file` is an undefined name, and an undefined name is a `NameError` at runtime, not a `SyntaxError` at parse time, so `ast.parse` accepts the file and the loop's self-check waves it through.
The model told itself the edit was valid, the engine agreed, the loop ended clean at round five with `exit_code=0`, and the code still crashes the moment `settings_loader` runs.

This is the concrete argument for the [adaptive harness]({{< relref "20-22-swebench-live-tomo-oi-gpt56sol-dynaconf-1225" >}}) work: a syntax check is a floor, not a gate.
A loop that ran the file's own tests, or even imported the edited module, before declaring the round done would have caught this on the spot and handed the `NameError` back to the model to fix.
The cheapest correct next step for the oi engine on tasks like this is not more retrieval, it is a post-edit smoke check stronger than `ast.parse`.

## Cost, and the leanness that survives

There is no per-run line item: `gpt-5.6-luna` is billed by the flat ChatGPT subscription, not per token.
Priced at an assumed GPT-5-class rate of $1.25 per million cache-miss input, $0.125 per million cache-hit input, and $10 per million output, this run would bill about **$0.97**: roughly $0.15 for the 119K fresh input tokens, negligible for the 29K cached ones, and $0.82 for the 82K output tokens.

The leanness result is real and worth stating even next to a zero grade.
Tomo-oi used 149K prompt tokens.
Codex used 7.28M and pi used 6.59M for the same task.
The symbol-anchored context pack keeps the model from re-reading the repo into a giant linear thread, so tomo-oi runs on roughly one-fortieth the input tokens of the two general agents.
On this task that leaner loop let a confident model over-rewrite and ship a `NameError`.
The fix is not to abandon the lean loop, it is to give it a verification step, and the pack has already done the expensive half of the job by pointing the model at the right file for almost no tokens.

## What the three luna runs say together

| harness | calls | prompt tok | output tok | wall | patch | FAIL_TO_PASS | regressions |
| --- | --- | --- | --- | --- | --- | --- | --- |
| codex | 75 | 7.28M | 27K | 13:05 | 579 L / 13 f | 2 / 5 | none |
| pi | 88 | 6.59M | 59K | 24:16 | 536 L / 13 f | 2 / 5 | none |
| tomo-oi | 5 | 0.15M | 82K | 25:25 | 372 L / 12 f | 0 / 5 | 4 / 4 |

Same paid model, three harnesses.
The two general agents converge on two of five and stall at the module-path loader, which is the model's ceiling on this task.
Tomo-oi reaches the right file for forty times fewer prompt tokens and then trips over its own broad rewrite, because its round-end check verifies syntax and not behavior.
The retrieval half of the oi engine is done and cheap.
The verification half is the next thing to build.
