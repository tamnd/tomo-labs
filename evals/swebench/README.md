# swebench

[SWE-bench](https://www.swebench.com) Lite is a set of real GitHub issues, each
paired with the commit that fixed it and the tests that fixing it made pass. A task
hands the agent the project checked out at the buggy commit and the issue text, and
asks for a source change that resolves it. The agent never sees the fix or the
tests. `lab gen --suite swebench` renders each instance into a lab task: `setup.sh`
clones the repository at the instance's base commit into the work tree, and the
prompt is the issue.

Grading applies the instance's hidden test patch on top of whatever the agent left,
then runs the two named test sets: `FAIL_TO_PASS`, the tests the fix should turn
green, and `PASS_TO_PASS`, the tests that must stay green so a fix does not break
the rest. A task passes only if every test in both sets passes.

The fix, the test patch, and the two test lists encode the answer, so they never
reach the agent. They live under `oracle/`, a sibling of `tasks/` the harness never
mounts, and `check.sh` reads them from there at grading time.

## Grading environment

These instances are years old, and their packages neither build nor import on a
current host Python. So `check.sh` builds a throwaway virtualenv per grade on the
interpreter the instance's era targets, using [uv](https://docs.astral.sh/uv) to
fetch that interpreter, installs the era's pinned dependencies and then the
checked-out project into it, and tears it down after. The interpreter version and
the dependency pins are transcribed from SWE-bench's own per-repo specs, so the
environment matches the era rather than drifting to today's versions.

Unlike the upstream SWE-bench harness, which grades inside a per-instance x86_64
image, this tier grades on the host in a uv-built venv. That keeps it inside the
lab's one-tool-image-plus-host-grading model and off the slow x86_64 emulation an
arm64 host would need for those images. The trade is fidelity: an instance is kept
only if its own gold fix, applied to a clean checkout, actually makes the tests
pass in this environment. An instance that does not provision here, or whose
`PASS_TO_PASS` set leans on something the host cannot supply (a live network
endpoint, say), fails that proof and is dropped with a reason. The rendered suite
is a validated subset, not the whole of Lite.

The tier grades the repositories whose suites run under a plain `pytest <node id>`:
requests, flask, pylint, pytest, sphinx, and xarray. The django and sympy instances
drive their own test runners rather than pytest, so they are skipped up front.

## Run it

```sh
go run ./cmd/lab scenarios --suite swebench
go run ./cmd/lab run tomo --suite swebench
go run ./cmd/lab report --suite swebench
```

A run needs `git` and `uv` on the host: `git` to clone each repo at its base commit,
`uv` to build the era's Python. The first run of a repo clones its full history into
`evals/swebench/.cache` (gitignored) so later tasks and reruns clone from local
disk.

## Regenerate

```sh
go run ./cmd/lab gen --suite swebench                 # small validated sample
go run ./cmd/lab gen --suite swebench --limit 40       # a larger sample
go run ./cmd/lab gen --suite swebench --all            # every gradeable instance
```

The generator fetches from the
[Hugging Face dataset viewer](https://huggingface.co/datasets/princeton-nlp/SWE-bench_Lite),
writes each task, then proves it by grading the instance's own gold fix; an instance
whose gold fix does not pass here is dropped and reported, never committed. `--all`
ignores the pytest-repo allowlist and lets the gold proof decide on its own. Pass
`--no-validate` to skip the proof when you are only inspecting output.

## License

The rendered tasks are derived from SWE-bench, which is MIT licensed, and reference
public GitHub repositories under their own licenses. The provenance of each task is
its directory name: `pallets__flask-4045` is SWE-bench Lite instance
`pallets__flask-4045`, drawn from `pallets/flask`.
