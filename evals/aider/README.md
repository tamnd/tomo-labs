# aider polyglot

The [Aider polyglot benchmark](https://github.com/Aider-AI/polyglot-benchmark) is
a set of [Exercism](https://exercism.org) practice exercises across six languages.
Each exercise ships a stub to fill in, a test suite that grades it, and a reference
solution. `lab gen --suite aider` renders each one into a lab task: the stub and
tests go into the work tree, and the task passes when the exercise's own tests are
green.

This tier runs the languages the shared base image already carries a toolchain
for:

- **Go**, graded by `go test ./...`
- **Python**, graded by `python3 -m unittest` (standard library, no pytest)

Rust, Java, C++, and JavaScript are in the upstream benchmark but need toolchains
the base does not ship, so they are left out until the base grows them.

Exercism deliberately gives the solver the tests, so the test files live in the
work tree and it is not a leak that the agent can read them. The reference
solution is different: it is kept under `answers/`, which the harness never mounts,
and used only to validate that a generated task grades correctly.

## Run it

```sh
go run ./cmd/lab scenarios --suite aider
go run ./cmd/lab run tomo --suite aider
go run ./cmd/lab report --suite aider
```

The task dirs are committed, so a run needs no network.

## Regenerate

```sh
go run ./cmd/lab gen --suite aider                     # small validated sample
go run ./cmd/lab gen --suite aider --langs go --all     # every Go exercise
go run ./cmd/lab gen --suite aider --limit 20           # 20 exercises per language
```

The generator fetches each exercise from the upstream repo, writes the task dir,
then proves it: it applies the reference solution and runs `check.sh`. A task whose
reference solution does not pass is dropped and reported, never committed. Pass
`--no-validate` to skip the proof when you are only inspecting output.

You need the toolchains the checkers call: a Go install for the Go tasks and a
Python 3 for the Python tasks.

## License

The rendered tasks are derived from Exercism exercises, which are MIT licensed, and
from the Aider polyglot benchmark. The provenance of each task is the exercise name
in its directory: `go-bowling` is the `bowling` practice exercise from the Go
track.
