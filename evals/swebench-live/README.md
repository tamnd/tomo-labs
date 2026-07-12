# swebench-live

[SWE-bench-Live](https://swe-bench-live.github.io) is the actively maintained cousin
of SWE-bench: it keeps adding recent GitHub issues drawn from fresh commits and
decontaminates them against training data, so the tier does not go stale the way the
2023-frozen Lite set has. Each instance still pairs a real issue with the commit
that fixed it and the tests that fixing it made pass. A task hands the agent the
project checked out at the buggy commit and the issue text, and asks for a source
change that resolves it. The agent never sees the fix or the tests.
`lab gen --suite swebench-live` renders each instance into a lab task: `setup.sh`
clones the repository at the instance's base commit into the work tree, and the
prompt is the issue.

Grading applies the instance's hidden test patch on top of whatever the agent left,
then runs the test files the bug's tests live in with `pytest -rA` and reads the
per-test outcomes from that run. `FAIL_TO_PASS`, the tests the fix should turn
green, must all pass, and the `PASS_TO_PASS` tests in those same files, the ones
that must stay green, must not regress.

The fix, the test patch, and the two test lists encode the answer, so they never
reach the agent. They live under `oracle/`, a sibling of `tasks/` the harness never
mounts, and `check.sh` reads them from there at grading time.

## Why grading reads the log instead of naming tests

Two facts about the live set force this approach.

It stores a parametrized test id truncated at its first space, so
`test_validate[Invalid Type - foo]` is recorded as `test_validate[Invalid`. A
truncated id can never be matched by name on the command line, so the grader matches
each recorded id as a prefix against the ids pytest actually reported.

Its `PASS_TO_PASS` lists run to a thousand tests and more, reaching across a
project's whole suite including integration and end-to-end tests a plain host venv
cannot run. So the tier scopes the regression check to the test files the bug's own
tests live in: the local regressions a fix could plausibly cause, checked from one
bounded pytest run rather than the entire suite. A `PASS_TO_PASS` id that did not run
in that scope is left alone; only a test that actually regressed fails the grade.

## Grading environment

These instances are recent, so a plain `uv pip install -e .` on a current
interpreter provisions far more of them than the era-frozen Lite set ever could.
`check.sh` builds a throwaway virtualenv per grade with [uv](https://docs.astral.sh/uv),
installs the checked-out project (trying the test extra, then a plain editable
install, then a non-editable one, so a project's layout does not decide the grade),
applies the test patch, runs the bug's test files, and tears the venv down after.

Unlike the upstream SWE-bench-Live harness, which grades inside a prebuilt
per-instance image, this tier grades on the host in a uv-built venv. That keeps it
inside the lab's one-tool-image-plus-host-grading model and off the slow x86_64
emulation an arm64 host would need for those images. The trade is fidelity, and the
tier is honest about it. It keeps only the instances whose test command is a plain
`pytest`, since a project that drives its tests through poetry, hatch, tox, or a
native toolchain needs that toolchain and its lockfile a plain venv does not
reconstruct. And, as in every tier, an instance is kept only if its own gold fix,
applied to a clean checkout, actually makes the tests pass here; one that will not
provision, needs a native dependency the host does not carry, or will not collect is
dropped with a reason. The rendered suite is the validated, host-provisionable
subset of the live set, not the whole of it.

## Run it

```sh
go run ./cmd/lab scenarios --suite swebench-live
go run ./cmd/lab run tomo --suite swebench-live
go run ./cmd/lab report --suite swebench-live
```

A run needs `git` and `uv` on the host: `git` to clone each repo at its base commit,
`uv` to build the venv and fetch the interpreter. The first run of a repo clones its
full history into `evals/swebench-live/.cache` (gitignored) so later tasks and
reruns clone from local disk.

## Regenerate

```sh
go run ./cmd/lab gen --suite swebench-live                  # small validated sample
go run ./cmd/lab gen --suite swebench-live --limit 40       # validate up to 40 candidates
go run ./cmd/lab gen --suite swebench-live --langs cfn-lint # narrow to one repo
go run ./cmd/lab gen --suite swebench-live --all            # draw from the full split
```

The generator fetches from the
[Hugging Face dataset viewer](https://huggingface.co/datasets/SWE-bench-Live/SWE-bench-Live),
keeps the plain-pytest instances, writes each task, then proves it by grading the
instance's own gold fix; an instance whose gold fix does not pass here is dropped and
reported, never committed. `--all` draws from the full split instead of the curated
lite one. Pass `--no-validate` to skip the proof when you are only inspecting output.

## License

The rendered tasks are derived from SWE-bench-Live, released under the MIT license,
and reference public GitHub repositories under their own licenses. The provenance of
each task is its directory name: `conan-io__conan-17092` is SWE-bench-Live instance
`conan-io__conan-17092`, drawn from `conan-io/conan`.
