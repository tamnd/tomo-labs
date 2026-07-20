# evals

The core `scenarios/` are hand-written tasks that exercise one behaviour each and
stay small enough to read at a glance. This directory holds the other end: whole
public benchmarks, rendered into the same task shape so the harness can run an
agent over hundreds of problems without any of them being bespoke.

Each subdirectory is a separate tier, and every command that touches tasks takes
`--suite <name>` to point at one:

```sh
go run ./cmd/lab scenarios --suite aider     # list the aider tasks
go run ./cmd/lab run tomo --suite evalplus    # run tomo over evalplus
go run ./cmd/lab report --suite aider         # report just that tier
```

A suite reads its tasks from `evals/<name>/tasks/` and lands its results under
`Data/evals/<name>/`, so a suite run never mixes into the core report and one
suite never mixes into another. Everything else is identical: the same base
image, the same trace proxy, the same grading from files on disk.

The tiers today:

- [`aider`](aider) rebuilds the Aider polyglot benchmark, Exercism practice
  exercises graded by their own test suites.
- [`evalplus`](evalplus) rebuilds EvalPlus, the HumanEval+ and MBPP+ function
  completion problems with their expanded hidden tests.
- [`terminal-bench`](terminal-bench) renders a validated, host-gradable cut of
  Terminal-Bench file and shell tasks with hidden pytest graders.

Each tier is materialized by `lab gen --suite <name>`, which fetches the upstream
benchmark and writes the task dirs. The task dirs are committed, so running a suite
needs no network, but the generator is there to refresh them or pull a wider
sample. See each tier's README for what it covers, what it needs installed, and how
to regenerate it.

## How a benchmark becomes a suite

A generator's whole job is to render each upstream problem into the shape the
harness already runs: a `prompt.txt`, a `setup.sh` that lays the starting files
into the work tree, and a `check.sh` that grades the work tree and exits zero when
it is correct. The generators live in Go alongside the rest of the harness
(`pkg/lab/gen_*.go`), reached through `lab gen`. The two hard parts are the same
for every benchmark.

The grader runs on the host, not in the agent's container, so `check.sh` uses host
tools (`go test`, a Python venv) rather than anything from the base image.

The expected answers must never reach the agent. Whatever encodes them, the
reference solution or the hidden test body, is kept in a sibling directory that
the harness does not mount (`answers/` for aider, `oracle/` for evalplus), and the
generator proves each task before keeping it: apply the known-good solution, run
`check.sh`, and drop the task if it does not pass. A task that cannot be validated
is a task that cannot be trusted to grade, so it never lands.
