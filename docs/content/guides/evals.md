---
title: "Evals"
description: "The eval tiers beside the core scenarios: whole public benchmarks rendered into the same task shape, selected with --suite, materialized and validated by lab gen, and graded on the host."
weight: 25
---

The core [scenarios](/guides/scenarios/) are hand-written tasks, each exercising one behaviour and small enough to read at a glance.
The eval tiers are the other end of the scale: whole public benchmarks, rendered into the same task shape, so the harness can run an agent over hundreds of problems without any of them being bespoke.
Everything else stays identical: the same base image, the same trace proxy, the same determinism, the same grading from files on disk.

## Selecting a tier

Every command that touches tasks takes `--suite <name>` to point at one tier instead of the core scenarios.

```bash
go run ./cmd/lab scenarios --suite aider      # list the aider tasks
go run ./cmd/lab run tomo --suite evalplus    # run tomo over evalplus
go run ./cmd/lab report --suite aider         # report just that tier
```

A suite reads its tasks from `evals/<name>/tasks/` and lands its results in a separate tree, so a suite run never mixes into the core report and one suite never mixes into another.
The task directories are committed, so running a suite needs no network.

## The tiers today

Two tiers ship with the harness.

- `aider` rebuilds the [Aider polyglot benchmark](https://github.com/Aider-AI/polyglot-benchmark), a set of [Exercism](https://exercism.org) practice exercises.
  Each exercise ships a stub to fill in, a test suite that grades it, and a reference solution.
  A task passes when the exercise's own tests are green.
  The tier runs the languages the shared base already carries a toolchain for: Go, graded by `go test ./...`, and Python, graded by `python3 -m unittest`.
  Rust, Java, C++, and JavaScript are in the upstream benchmark but need toolchains the base does not ship, so they are left out until the base grows them.
- `evalplus` rebuilds [EvalPlus](https://github.com/evalplus/evalplus), the HumanEval+ and MBPP+ function-completion problems with their expanded hidden tests.
  Each task gives the agent a function to complete, and grades the completion against the larger hidden test set that EvalPlus adds on top of the original benchmark.

## How a benchmark becomes a suite

A generator's whole job is to render each upstream problem into the shape the harness already runs: a `prompt.txt`, a `setup.sh` that lays the starting files into the work tree, and a `check.sh` that grades the work tree and exits zero when it is correct.
The generators live in Go alongside the rest of the harness in `pkg/lab/gen_*.go`, reached through `lab gen`.

Two things are true of every generated tier, and both are about trust.

The grader runs on the host, not in the agent's container, so `check.sh` uses host tools like `go test` or a Python venv rather than anything from the base image.
This keeps the toolchain that grades a task separate from the toolchain the agent had.

The expected answers must never reach the agent.
Whatever encodes them, a reference solution for aider or a hidden test body for evalplus, is kept in a sibling directory the harness does not mount, `answers/` for aider and `oracle/` for evalplus.
The generator proves each task before keeping it: it applies the known-good solution, runs `check.sh`, and drops the task if it does not pass.
A task that cannot be validated is a task that cannot be trusted to grade, so it never lands.

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
