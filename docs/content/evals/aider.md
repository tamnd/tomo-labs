---
title: "aider"
description: "The Aider polyglot tier: Exercism practice exercises rendered into harness tasks, graded by each exercise's own test suite, run on the language toolchains the base image already carries."
weight: 10
---

The `aider` tier rebuilds the [Aider polyglot benchmark](https://github.com/Aider-AI/polyglot-benchmark), a curated set of [Exercism](https://exercism.org) practice exercises drawn from six languages.
Each exercise is a small, self-contained programming problem: a stub to fill in, a test suite that grades it, and a reference solution that is known to pass those tests.
This makes it a natural fit for the harness, because every exercise already carries its own grader, so the tier does not have to invent how a task is judged, only how it is set up.

## Results

Every tool runs the same exercises through the same trace proxy, so the row is the tool: how many it got green, how many tokens it spent, and what it cost at the reference rates.
`pass` is graded by each exercise's own test suite, `1st` is how many passed on the first attempt before the retry kicked in, and `cost` prices the tokens at DeepSeek's paid rates even though the run itself was free.
The table below is written by `scripts/eval_docs.go`, so a rerun refreshes it in place.

<!-- eval-results:start -->
Snapshot taken 2026-07-11 on the `nemotron-3-ultra-free` model, every tool over the same tasks through the same trace proxy.
Rows are ordered by total tokens, cheapest first, and `pass` is how many of the 9 tasks the tool got a passing grade on.

| tool | version | pass | 1st | tokens | avg | cost | rss | wall | install |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| hermes | 0.18.2 | 0/7 | 0 | 0 | 0 | - | 117MB | 0s | 221MB |
| kilocode | 7.4.5 | 0/9 | 0 | 0 | 0 | - | 544MB | 0s | 591MB |
| openclaw | 2026.7.1-beta.2 | 1/2 | 1 | 143,850 | 0 | $0.0214 | 394MB | 0s | 407MB |
| aider | 0.86.2 | 3/9 | 2 | 156,629 | 0 | $0.0863 | 242MB | 0s | 621MB |
| opencode | 1.17.18 | 2/2 | 2 | 194,561 | 0 | $0.0306 | 684MB | 0s | 446MB |
| tomo | v0.2.4 | 2/2 | 2 | 231,018 | 0 | $0.0387 | 87MB | 0s | 21MB |
| pi | 0.80.6 | 2/2 | 2 | 430,537 | 0 | $0.0793 | 163MB | 0s | 156MB |
| gemini-cli | 0.52.0-nightly.20260710.ga4c91ce19 | 2/9 | 2 | 686,062 | 0 | $0.0928 | 265MB | 0s | 206MB |
| copilot | 1.0.70 | 7/9 | 7 | 1,270,523 | 0 | $0.1787 | 402MB | 0s | 418MB |
| codex | 0.145.0-alpha.4 | 8/9 | 4 | 2,420,092 | 0 | $0.3408 | 96MB | 0s | 424MB |
| claude-code | 2.1.207 | 9/9 | 9 | 3,547,432 | 0 | $0.4702 | 294MB | 0s | 325MB |

<!-- eval-results:end -->

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
