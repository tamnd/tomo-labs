---
title: "evalplus"
description: "The EvalPlus tier: HumanEval+ and MBPP+ function-completion problems graded against the expanded hidden test sets, run in a suite-local Python venv on the host."
weight: 20
---

The `evalplus` tier rebuilds [EvalPlus](https://github.com/evalplus/evalplus), which takes the well-known HumanEval and MBPP function-completion benchmarks and adds far more tests per problem.
The original benchmarks each ship a handful of example tests, enough that a plausible-looking answer often passes; EvalPlus adds a much larger hidden test set, so a completion has to actually be correct to score.
That expansion is the whole point of using EvalPlus over the originals: it is much harder to pass by luck.

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
