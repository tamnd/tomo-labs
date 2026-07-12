---
title: "swebench-live"
description: "The SWE-bench-Live tier: recent GitHub issues from the continuously updated live set, each graded on the host in a uv-built venv by running the bug's own test files and reading per-test outcomes from the pytest log."
weight: 41
---

The `swebench-live` tier rebuilds [SWE-bench-Live](https://swe-bench-live.github.io), the actively maintained cousin of SWE-bench.
Where the original Lite set was frozen in 2023, the live set keeps adding fresh issues drawn from recent commits and decontaminates them against training data, so the tier does not go stale.
Each instance still pairs a real GitHub issue with the commit that fixed it and the tests that fixing it made pass.
A task hands the agent the project checked out at the buggy commit and the issue text, and asks for a source change that resolves it.
The agent never sees the fix or the tests, so the tier measures whether a tool can navigate a recent codebase and land a working change.

## Results

<!-- eval-results:start -->
No sweep captured yet.
Run `go run scripts/eval_docs.go swebench-live` to run every tool over the tier and write the table here.
<!-- eval-results:end -->

## What a task looks like

Each instance becomes one harness task under `evals/swebench-live/tasks/`.

| Piece | What it holds |
|---|---|
| `prompt.txt` | the issue text, plus the ground rules that keep the run gradable: edit the source in place, do not touch the tests |
| `setup.sh` | clones the repository at the instance's base commit into the work tree, so the agent edits the real project |
| `check.sh` | builds a modern Python venv, installs the project, applies the hidden test patch, runs the bug's test files, and reads per-test outcomes from the log |

The first clone of a repository fetches its full history into `evals/swebench-live/.cache` (gitignored), so later tasks on the same repo, and reruns, clone from local disk instead of the network.

## How grading runs

Grading applies the instance's hidden test patch on top of whatever the agent left, then runs the test files the bug's tests live in with `pytest -rA`.
It reads the per-test outcomes from that run and checks two things: every `FAIL_TO_PASS` test, the ones the fix should turn green, is now passing, and every `PASS_TO_PASS` test in those same files, the ones that must stay green, still passes.

Two details of the live set force this log-reading approach rather than passing the tests to pytest by name.

The set stores a parametrized test id truncated at its first space, so `test_validate[Invalid Type - foo]` is recorded as `test_validate[Invalid`.
A truncated id can never be matched by name on the command line, so the grader instead matches each recorded id as a prefix against the ids pytest actually reported.

The set's `PASS_TO_PASS` lists are large, a thousand tests and more, and reach across a project's whole suite including integration and end-to-end tests that a plain host venv cannot run.
So the tier scopes the regression check to the test files the bug's own tests live in: the local regressions a fix could plausibly cause, checked from one bounded pytest run rather than a project's entire suite.
A `PASS_TO_PASS` id that did not run in that scope is left alone; only a test that actually regressed fails the grade.

The fix, the test patch, and the two test lists are the answer key, so they never reach the agent.
They are written under `evals/swebench-live/oracle/<instance>/`, a sibling directory the harness never mounts, alongside the repository, base commit, target interpreter, and the list of test files `check.sh` reads.
The grader is on the host and entirely separate from the agent's container, the same trust boundary every tier holds.

## Fidelity, and why it is a subset

Upstream SWE-bench-Live grades inside a prebuilt per-instance image with the whole environment baked in.
This tier grades on the host in a uv-built venv instead, which keeps it inside the lab's one-tool-image-plus-host-grading model and off the slow x86_64 emulation an arm64 host would need for those images.
Because these instances are recent, a plain `uv pip install -e .` on a current interpreter provisions far more of them than the era-frozen Lite set ever could.

Two things narrow the set that lands.
The tier keeps only the instances whose test command is a plain `pytest`, since a project that drives its tests through poetry, hatch, tox, or a native toolchain needs that toolchain and its lockfile, which a plain venv does not reconstruct.
And, as in every tier, an instance is kept only if its own gold fix, applied to a clean checkout, actually makes the tests pass here.
The generator proves every instance this way and drops the ones that do not, with a reason: a project that will not install on a plain venv, a test that needs a native dependency the host does not carry, a suite that will not collect.
So the rendered suite is the validated, host-provisionable subset of the live set, not the whole of it.

## Running and regenerating

```bash
go run ./cmd/lab scenarios --suite swebench-live         # list the tasks
go run ./cmd/lab run tomo --suite swebench-live          # run one tool over the tier
go run ./cmd/lab report --suite swebench-live            # the tier's comparison table

go run ./cmd/lab gen --suite swebench-live               # a small validated sample
go run ./cmd/lab gen --suite swebench-live --limit 40    # validate up to 40 candidates
go run ./cmd/lab gen --suite swebench-live --langs cfn-lint  # narrow to one repo
go run ./cmd/lab gen --suite swebench-live --all         # draw from the full split
```

A run needs `git` and `uv` on the host: `git` to clone each repo at its base commit, `uv` to build the venv and fetch the interpreter.
`--limit N` caps how many candidate instances the generator validates, `--langs a,b` narrows the pull to named repos by short name, and `--all` draws from the full split instead of the curated lite one.
See [evals](/evals/) for how a suite is selected and how the trust boundary works across every tier.
