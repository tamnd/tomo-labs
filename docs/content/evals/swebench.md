---
title: "swebench"
description: "The SWE-bench Lite tier: real GitHub issues, each graded by the tests its fix made pass, run in a per-instance era Python venv built on the host with uv."
weight: 40
---

The `swebench` tier rebuilds [SWE-bench](https://www.swebench.com) Lite, a set of real GitHub issues drawn from well-known Python projects.
Each instance pairs an issue with the commit that fixed it and the tests that fixing it made pass.
A task hands the agent the project checked out at the buggy commit and the issue text, and asks for a source change that resolves it.
The agent never sees the fix or the tests, so the tier measures whether a tool can navigate a real codebase and land a working change, not whether it can complete a stub.

## Results

<!-- eval-results:start -->
No sweep captured yet.
Run `go run scripts/eval_docs.go swebench` to run every tool over the tier and write the table here.
<!-- eval-results:end -->

## What a task looks like

Each instance becomes one harness task under `evals/swebench/tasks/`.

| Piece | What it holds |
|---|---|
| `prompt.txt` | the issue text, plus the ground rules that keep the run gradable: edit the source in place, do not touch the tests |
| `setup.sh` | clones the repository at the instance's base commit into the work tree, so the agent edits the real project |
| `check.sh` | builds the era's Python venv, installs the project, applies the hidden test patch, and runs the two test sets |

The first clone of a repository fetches its full history into `evals/swebench/.cache` (gitignored), so later tasks on the same repo, and reruns, clone from local disk instead of the network.

## How grading runs

Grading applies the instance's hidden test patch on top of whatever the agent left, then runs the two named test sets: `FAIL_TO_PASS`, the tests the fix should turn green, and `PASS_TO_PASS`, the tests that must stay green so a fix does not break the rest.
A task passes only if every test in both sets passes.

The fix, the test patch, and the two test lists are the answer key, so they never reach the agent.
They are written under `evals/swebench/oracle/<instance>/`, a sibling directory the harness never mounts, alongside the repository and base commit `setup.sh` reads and the era pins and install recipe `check.sh` builds from.

These instances are years old, and their packages neither build nor import on a current host Python.
So `check.sh` builds a throwaway virtualenv per grade on the interpreter the instance's era targets, using [uv](https://docs.astral.sh/uv) to fetch that interpreter, installs the era's pinned dependencies and then the checked-out project, and tears it down after.
The interpreter version and the dependency pins are transcribed from SWE-bench's own per-repo specs, so the environment matches the era rather than drifting to today's versions.
The grader is on the host and entirely separate from the agent's container, the same trust boundary every tier holds.

## Fidelity, and why it is a subset

Upstream SWE-bench grades inside a per-instance x86_64 image with the whole environment baked in.
This tier grades on the host in a uv-built venv instead, which keeps it inside the lab's one-tool-image-plus-host-grading model and off the slow x86_64 emulation an arm64 host would need for those images.
The trade is fidelity, and the tier is honest about it.

An instance is kept only if its own gold fix, applied to a clean checkout, actually makes the tests pass in this environment.
The generator proves every instance this way and drops the ones that do not, with a reason.
An instance whose packages do not build on the era Python here, or whose `PASS_TO_PASS` set leans on something the host cannot supply, a live network endpoint for a `requests` test, say, fails that proof and never lands.
So the rendered suite is a validated subset of Lite, not the whole of it.
The tier grades the repositories whose suites run under a plain `pytest`: `requests`, `flask`, `pylint`, `pytest`, `sphinx`, and `xarray`.
The `django` and `sympy` instances drive their own test runners rather than pytest, so they are skipped up front.

## Running and regenerating

```bash
go run ./cmd/lab scenarios --suite swebench            # list the swebench tasks
go run ./cmd/lab run tomo --suite swebench             # run one tool over the tier
go run ./cmd/lab report --suite swebench               # the tier's comparison table

go run ./cmd/lab gen --suite swebench                  # a small validated sample
go run ./cmd/lab gen --suite swebench --limit 40       # a larger sample
go run ./cmd/lab gen --suite swebench --all            # every gradeable instance
```

A run needs `git` and `uv` on the host: `git` to clone each repo at its base commit, `uv` to build the era's Python.
`--limit N` caps how many instances the generator validates, `--all` ignores the pytest-repo allowlist and lets the gold proof decide on its own, and `--no-validate` skips the proof when you are only inspecting output.
See [evals](/evals/) for how a suite is selected and how the trust boundary works across every tier.
