---
title: "aider"
description: "The Aider polyglot tier: Exercism practice exercises rendered into harness tasks, graded by each exercise's own test suite, run on the language toolchains the base image already carries."
weight: 10
---

The `aider` tier rebuilds the [Aider polyglot benchmark](https://github.com/Aider-AI/polyglot-benchmark), a curated set of [Exercism](https://exercism.org) practice exercises drawn from six languages.
Each exercise is a small, self-contained programming problem: a stub to fill in, a test suite that grades it, and a reference solution that is known to pass those tests.
This makes it a natural fit for the harness, because every exercise already carries its own grader, so the tier does not have to invent how a task is judged, only how it is set up.

## What a task looks like

Each exercise becomes one harness task under `evals/aider/tasks/`.

| Piece | Where it comes from | What it does |
|---|---|---|
| `prompt.txt` | the exercise instructions | tells the agent what to implement |
| `setup.sh` | the exercise stub files | lays the starting files into the work tree |
| `check.sh` | the exercise test suite | runs the tests and exits zero when they pass |

The reference solution is the one thing kept out of the task.
It lives under `evals/aider/answers/<exercise>`, a sibling directory the harness never mounts into the agent's container, so the agent cannot read the answer it is being graded against.
The generator uses that answer only to prove the task before keeping it: it drops the reference solution into a throwaway work tree, runs `check.sh`, and keeps the task only if the tests pass.

## Which languages run

The upstream benchmark spans six languages, but the tier only runs the ones the shared base image already has a toolchain for.

| Language | Graded by | Status |
|---|---|---|
| Go | `go test ./...` | runs |
| Python | `python3 -m unittest` (standard library, no pytest) | runs |
| Rust | `cargo test` | left out, no toolchain in base |
| Java | JUnit | left out, no toolchain in base |
| C++ | the exercise build | left out, no toolchain in base |
| JavaScript | the exercise test runner | left out, no toolchain in base |

The four excluded languages are still in the upstream benchmark; they are skipped here only because grading them would need toolchains the base does not ship.
When the base grows a toolchain, the tier can grow the language: `check.sh` runs on the host, so the grader for a new language is a host command, not a change to the agent's image.

## Running and regenerating

```bash
go run ./cmd/lab scenarios --suite aider              # list the aider tasks
go run ./cmd/lab run tomo --suite aider               # run one tool over the tier
go run ./cmd/lab report --suite aider                 # the tier's comparison table

go run ./cmd/lab gen --suite aider                    # a small validated sample
go run ./cmd/lab gen --suite aider --langs go --all   # every Go exercise
go run ./cmd/lab gen --suite aider --limit 20         # 20 exercises per language
```

`--langs` selects language tracks, `--limit N` takes N exercises per track, and `--all` takes every exercise the benchmark offers.
Every kept task has already been proven against its reference solution, so the tier grades honestly the moment it is written.
See [evals](/evals/) for how a suite is selected and how the trust boundary works across every tier.
