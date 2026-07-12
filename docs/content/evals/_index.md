---
title: "Evals"
linkTitle: "Evals"
description: "The eval tiers beside the core scenarios: whole public benchmarks rendered into the same task shape, selected with --suite, materialized and validated by lab gen, and graded on the host."
weight: 18
featured: true
---

The core [scenarios](/guides/scenarios/) are hand-written tasks, each exercising one behaviour and small enough to read at a glance.
The eval tiers are the other end of the scale: whole public benchmarks, rendered into the same task shape, so the harness can run an agent over hundreds of problems without any of them being bespoke.
Everything else stays identical: the same base image, the same trace proxy, the same determinism, the same grading from files on disk.

Four tiers ship today, each with its own page.

- [aider](/evals/aider/) rebuilds the Aider polyglot benchmark, a set of Exercism practice exercises graded by their own tests.
- [evalplus](/evals/evalplus/) rebuilds EvalPlus, the HumanEval+ and MBPP+ function-completion problems with their expanded hidden tests.
- [livecodebench](/evals/livecodebench/) rebuilds LiveCodeBench, competitive-programming problems graded by their public and hidden test cases.
- [swebench](/evals/swebench/) rebuilds SWE-bench Lite, real GitHub issues graded by the tests their fix made pass.

## Selecting a tier

Every command that touches tasks takes `--suite <name>` to point at one tier instead of the core scenarios.

```bash
go run ./cmd/lab scenarios --suite aider      # list the aider tasks
go run ./cmd/lab run tomo --suite evalplus    # run tomo over evalplus
go run ./cmd/lab report --suite aider         # report just that tier
```

A suite reads its tasks from `evals/<name>/tasks/` and lands its results in a separate tree, so a suite run never mixes into the core report and one suite never mixes into another.
The task directories are committed, so running a suite needs no network.

## How a benchmark becomes a suite

A generator's whole job is to render each upstream problem into the shape the harness already runs: a `prompt.txt`, a `setup.sh` that lays the starting files into the work tree, and a `check.sh` that grades the work tree and exits zero when it is correct.
The generators live in Go alongside the rest of the harness in `pkg/lab/gen_*.go`, reached through `lab gen`.

Two things are true of every generated tier, and both are about trust.

The grader runs on the host, not in the agent's container, so `check.sh` uses host tools like `go test` or a Python venv rather than anything from the base image.
This keeps the toolchain that grades a task separate from the toolchain the agent had.

The expected answers must never reach the agent.
Whatever encodes them, a reference solution for aider or a hidden test body for evalplus, or a gold fix and hidden tests for swebench, is kept in a sibling directory the harness does not mount, `answers/` for aider and `oracle/` for the rest.
The generator proves each task before keeping it: it applies the known-good solution, runs `check.sh`, and drops the task if it does not pass.
A task that cannot be validated is a task that cannot be trusted to grade, so it never lands.
The swebench tier leans on this hardest: its instances are years old, so most do not provision on a current host and are dropped, and the tier that ships is the validated subset that does.

## Regenerating a tier

The committed task dirs are enough to run a suite, but `lab gen` is there to refresh them or pull a wider sample.

```bash
go run ./cmd/lab gen --suite aider                    # a small validated sample
go run ./cmd/lab gen --suite aider --langs go --all   # every Go exercise
go run ./cmd/lab gen --suite aider --limit 20         # 20 exercises per language
go run ./cmd/lab gen --suite evalplus --all           # the full HumanEval+ and MBPP+ set
```

The flags after `gen` tune the pull.
`--limit N` takes N problems per track, `--all` takes every problem the benchmark offers, `--langs a,b` selects language tracks for aider or datasets for evalplus, and `--no-validate` skips the reference-solution proof for a quick inspection.
Without `--no-validate`, every kept task has been graded against a known-good answer, so the tier grades honestly the moment it is written.
