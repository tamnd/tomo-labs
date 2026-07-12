---
title: "evalplus"
description: "The EvalPlus tier: HumanEval+ and MBPP+ function-completion problems graded against the expanded hidden test sets, run in a suite-local Python venv on the host."
weight: 20
---

The `evalplus` tier rebuilds [EvalPlus](https://github.com/evalplus/evalplus), which takes the well-known HumanEval and MBPP function-completion benchmarks and adds far more tests per problem.
The original benchmarks each ship a handful of example tests, enough that a plausible-looking answer often passes; EvalPlus adds a much larger hidden test set, so a completion has to actually be correct to score.
That expansion is the whole point of using EvalPlus over the originals: it is much harder to pass by luck.

## Results

Every tool runs the same problems through the same trace proxy, so the row is the tool: how many completions passed the expanded hidden tests, how many tokens it spent, and what it cost at the reference rates.
`pass` is graded against the full EvalPlus test set, `1st` is how many passed on the first attempt before the retry kicked in, and `cost` prices the tokens at DeepSeek's paid rates even though the run itself was free.
The table below is written by `scripts/eval_docs.go`, so a rerun refreshes it in place.

<!-- eval-results:start -->
Snapshot taken 2026-07-11 on the `deepseek-v4-flash-free` model, every tool over the same tasks through the same trace proxy.
Rows are ordered by total tokens, cheapest first, and `pass` is how many of the 1 tasks the tool got a passing grade on.

| tool | version | pass | 1st | tokens | avg | cost | rss | wall | install |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| tomo | v0.2.4 | 1/1 | 1 | 9,460 | 9,460 | $0.0018 | 13MB | 11s | 21MB |
| pi | 0.80.6 | 1/1 | 1 | 10,288 | 10,288 | $0.0022 | 169MB | 13s | 156MB |
| opencode | 1.17.18 | 1/1 | 1 | 41,451 | 41,451 | $0.0045 | 702MB | 16s | 446MB |
| gemini-cli | 0.52.0-nightly.20260710.ga4c91ce19 | 1/1 | 1 | 41,460 | 41,460 | $0.0057 | 359MB | 21s | 181MB |
| codex | 0.145.0-alpha.4 | 1/1 | 1 | 45,826 | 45,826 | $0.0050 | 90MB | 18s | 426MB |
| hermes | 0.18.2 | 1/1 | 1 | 58,899 | 58,899 | $0.0054 | 127MB | 32s | 221MB |
| claude-code | 2.1.207 | 1/1 | 1 | 82,337 | 82,337 | $0.0078 | 287MB | 18s | 325MB |
| openclaw | 2026.7.1-beta.2 | 1/1 | 1 | 88,936 | 88,936 | $0.0096 | 511MB | 43s | 407MB |

<!-- eval-results:end -->

## The two datasets

The tier covers both EvalPlus datasets, pulled from the Hugging Face dataset viewer.

| Dataset | Source | Size | Shape |
|---|---|---|---|
| HumanEval+ | `evalplus/humanevalplus` | 164 problems | a function signature and docstring to complete |
| MBPP+ | `evalplus/mbppplus` | 378 problems | a prose task plus one worked example, turned into a stub |

HumanEval+ hands the agent a signature and docstring directly, so the stub is the signature with the body left blank.
MBPP+ describes the task in prose, so the generator builds a stub from the prose prompt plus one example, giving the agent a concrete function to fill in rather than a bare description.

## What a task looks like

Each problem becomes one harness task under `evals/evalplus/tasks/`.

| Piece | What it holds |
|---|---|
| `prompt.txt` | the problem, and the name of the function the hidden tests will call |
| `solution.py` | the stub the agent starts from and completes in place |
| `check.sh` | assembles the finished `solution.py` with the hidden test and runs it |

The hidden test body is the answer key, so it never reaches the agent.
It is written under `evals/evalplus/oracle/<problem>/test.py`, a sibling directory the harness never mounts, and `check.sh` reads it from there at grading time.
So the agent sees the function to write and the name the tests will call, but never the tests themselves.

## How grading runs

EvalPlus tests import numpy, which the base image does not carry.
Rather than push numpy into every agent's image, the tier builds a suite-local Python venv with numpy once, on the host, and `check.sh` runs the hidden tests inside it.
This keeps the grading toolchain on the host and entirely separate from whatever the agent had in its container, the same trust boundary every tier holds: the thing that grades a task is never the thing the agent could reach.

The generator proves each task the same way the other tiers do.
It fills `solution.py` with the problem's known-good solution, runs `check.sh`, and keeps the task only if the hidden tests pass, so a problem that cannot be validated never lands as a task.

## Running and regenerating

```bash
go run ./cmd/lab scenarios --suite evalplus              # list the evalplus tasks
go run ./cmd/lab run tomo --suite evalplus               # run one tool over the tier
go run ./cmd/lab report --suite evalplus                 # the tier's comparison table

go run ./cmd/lab gen --suite evalplus                    # a small validated sample
go run ./cmd/lab gen --suite evalplus --all              # the full HumanEval+ and MBPP+ set
go run ./cmd/lab gen --suite evalplus --langs mbppplus   # one dataset only
```

For this tier `--langs` doubles as the dataset selector, so `--langs humanevalplus` or `--langs mbppplus` narrows to one set.
`--all` pulls both datasets in full, and `--limit N` caps the problems taken per dataset.
See [evals](/evals/) for how a suite is selected and how the trust boundary works across every tier.
