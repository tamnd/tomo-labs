# evalplus

[EvalPlus](https://github.com/evalplus/evalplus) takes the HumanEval and MBPP
benchmarks and adds far more tests per problem, so a solution that only fits the
original handful of cases gets caught. Each problem is a single function to
complete. `lab gen --suite evalplus` renders each one into a lab task whose stub is
`solution.py`, a signature and docstring for the agent to fill, graded by the
problem's expanded hidden test suite.

Two datasets are covered:

- **HumanEval+** (`humanevalplus-*`), 164 problems, each a signature and docstring
  to complete.
- **MBPP+** (`mbppplus-*`), 378 problems, each a prose task plus one worked
  example.

Unlike the Aider tier, the tests encode the expected outputs, so they must not
reach the agent. Each problem's test body is written under `oracle/`, which the
harness never mounts; `check.sh` reads it from there, concatenates it with the
finished `solution.py`, and runs the result.

EvalPlus tests import numpy for float comparison, which the base image does not
carry. So `check.sh` builds a small numpy venv beside the tasks the first time it
runs (`evals/evalplus/.venv`) and reuses it. The venv is a local cache, gitignored
and rebuilt on demand, so the tier stays self-contained without changing the shared
base image. This is the only Python the tier needs on the host: the language the
problems are written in.

## Run it

```sh
go run ./cmd/lab scenarios --suite evalplus
go run ./cmd/lab run tomo --suite evalplus
go run ./cmd/lab report --suite evalplus
```

The task dirs are committed, so a run needs no network. It does need a host
`python3` to build the grading venv.

## Regenerate

```sh
go run ./cmd/lab gen --suite evalplus                          # small sample of both
go run ./cmd/lab gen --suite evalplus --langs humanevalplus     # one dataset
go run ./cmd/lab gen --suite evalplus --all                     # every problem
```

Here `--langs` selects the dataset (`humanevalplus` or `mbppplus`). The generator
fetches from the [Hugging Face dataset viewer](https://huggingface.co/datasets/evalplus/humanevalplus),
writes each task, then proves it by grading the canonical solution; a problem whose
canonical solution does not pass is dropped and reported, never committed. Pass
`--no-validate` to skip the proof when you are only inspecting output.

## License

The rendered tasks are derived from EvalPlus, which is Apache-2.0 licensed, and in
turn from HumanEval (MIT) and MBPP (CC-BY-4.0). The provenance of each task is its
directory name: `humanevalplus-humaneval-0` is problem `HumanEval/0`, and
`mbppplus-2` is MBPP task 2.
