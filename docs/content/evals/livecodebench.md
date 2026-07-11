---
title: "livecodebench"
description: "The LiveCodeBench tier: contamination-free competition problems from LeetCode, AtCoder, and Codeforces, graded by LiveCodeBench's own test runner in a suite-local Python venv on the host."
weight: 30
---

The `livecodebench` tier rebuilds [LiveCodeBench](https://livecodebench.github.io), a benchmark of programming-competition problems scraped from LeetCode, AtCoder, and Codeforces.
Its selling point is contamination control: every problem carries the date it was published, so a run can be scoped to problems that appeared after a model's training cutoff, which is the honest way to ask whether a model is solving or remembering.
The tier pulls the pruned [`code_generation_lite`](https://huggingface.co/datasets/livecodebench/code_generation_lite) dataset, which keeps a representative sample of each problem's tests rather than the full set, so a task stays small enough to render into the harness.

## Results

Every tool runs the same tasks through the same trace proxy, so the row is the tool: how many problems it got green, how many tokens it spent getting there, and what it cost at the reference rates.
`pass` is graded by LiveCodeBench's own runner, `1st` is how many it passed on the first attempt before the retry kicked in, and `cost` prices the tokens at DeepSeek's paid rates even though the run itself was free.
The table below is written by `scripts/eval_docs.go`, so a rerun refreshes it in place.

<!-- eval-results:start -->
Snapshot taken 2026-07-11 on the `nemotron-3-ultra-free` model, every tool over the same tasks through the same trace proxy.
Rows are ordered by total tokens, cheapest first, and `pass` is how many of the 6 tasks the tool got a passing grade on.

| tool | version | pass | 1st | tokens | avg | cost | rss | wall | install |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| aider | 0.86.2 | 6/6 | 6 | 29,018 | 4,836 | $0.0268 | 239MB | 35s | 621MB |
| pi | 0.80.6 | 5/6 | 5 | 55,547 | 9,257 | $0.0283 | 165MB | 36s | 156MB |
| tomo | v0.2.4 | 6/6 | 4 | 79,980 | 13,330 | $0.0430 | 13MB | 65s | 21MB |
| opencode | 1.17.18 | 3/6 | 2 | 108,006 | 18,001 | $0.0312 | 594MB | 47s | 433MB |
| codex | 0.145.0-alpha.4 | 3/6 | 3 | 300,989 | 50,164 | $0.0800 | 93MB | 59s | 424MB |
| gemini-cli | 0.52.0-nightly.20260710.ga4c91ce19 | 5/6 | 5 | 338,667 | 56,444 | $0.0733 | 302MB | 51s | 206MB |
| kilocode | 7.4.5 | 6/6 | 6 | 374,848 | 62,474 | $0.0775 | 596MB | 59s | 591MB |
| openclaw | 2026.7.1-beta.2 | 5/6 | 5 | 384,333 | 64,055 | $0.0838 | 457MB | 71s | 404MB |
| hermes | 0.18.2 | 5/6 | 5 | 439,269 | 73,211 | $0.0992 | 129MB | 80s | 221MB |
| copilot | 1.0.70 | 2/6 | 1 | 478,102 | 79,683 | $0.0916 | 387MB | 61s | 418MB |
| claude-code | 2.1.207 | 6/6 | 6 | 1,155,121 | 192,520 | $0.2110 | 293MB | 109s | 325MB |

<!-- eval-results:end -->

## The two problem shapes

LiveCodeBench problems come in two shapes, and the tier renders both, because they are graded differently.

| Shape | Where it comes from | What the agent writes | How it is graded |
|---|---|---|---|
| stdin | Codeforces, AtCoder | a whole program in `solution.py` that reads stdin and writes stdout | the runner feeds each test's input on stdin and diffs stdout |
| functional | LeetCode | the completed `class Solution` in `solution.py` | the runner imports the class and calls the method, comparing return values |

A functional problem carries a `func_name` in its metadata and a starter `class Solution`; a stdin problem carries neither.
The generator renders a balanced sample of both so a verification run exercises both grading paths, not just one.

## Choosing difficulty

The dataset tags every problem `easy`, `medium`, or `hard`, and the generator can pin the tier with `--difficulty`.
This is the lever for what a run is meant to show.
Draw an `easy` set to get a clean green sweep that showcases a tool at its strongest, a `medium` set for a fair middle, or a `hard` set to stress a tool where the weaker ones start to fall off.
Pass one tier or a comma list (`--difficulty easy,medium`), and an empty flag takes whatever difficulty comes first, the way it did before.
Each rendered task records its tier in its `desc` line, so a report table shows at a glance which difficulty a run covered.

## What a task looks like

Each problem becomes one harness task under `evals/livecodebench/tasks/`.

| Piece | What it holds |
|---|---|
| `prompt.txt` | the problem statement, and for a functional problem the starter class and the method the hidden tests will call |
| `solution.py` | the stub the agent completes: an empty stdin program, or the starter class |
| `check.sh` | grades `solution.py` with LiveCodeBench's own runner in the numpy venv |

The hidden tests are the answer key, so they never reach the agent.
They are written under `evals/livecodebench/oracle/<problem>/`, a sibling directory the harness never mounts, in the exact wire form the dataset ships: a plain JSON array for the public cases, and the base64/zlib/pickle blob for the private cases.
`grade.py` decodes them at grading time, the same way LiveCodeBench's own loader does.

## How grading runs

The tier does not reimplement LiveCodeBench's grader; it vendors it.
LiveCodeBench's `testing_util.py` is checked into the harness and dropped under `evals/livecodebench/oracle/_lcb/` at generation time, so a solution is judged by the exact code the upstream benchmark uses, feeding stdin and diffing stdout for a stdin problem or calling the method and comparing return values for a functional one.
That runner imports numpy, which the base image does not carry, so, like the [evalplus](/evals/evalplus/) tier, `check.sh` builds one suite-local venv with numpy on the host and reuses it.
The grader is on the host and entirely separate from the agent's container, the same trust boundary every tier holds.

## How a task is proven

The other tiers prove a task by grading a known-good solution and dropping any that fails.
LiveCodeBench ships no reference solutions, on purpose, since a held-out answer key is what keeps the benchmark contamination-free.
So the tier proves a task the other way around: it grades the untouched stub and keeps the task only if the runner ran to completion and correctly rejected it.
That does not prove a correct solution exists, but it proves the grader is wired end to end and does not pass for free, which is the failure mode a silent grader would hide.

## Running and regenerating

```bash
go run ./cmd/lab scenarios --suite livecodebench            # list the livecodebench tasks
go run ./cmd/lab run tomo --suite livecodebench             # run one tool over the tier
go run ./cmd/lab report --suite livecodebench               # the tier's comparison table

go run ./cmd/lab gen --suite livecodebench                  # a small validated sample
go run ./cmd/lab gen --suite livecodebench --langs v5       # draw from a later release window
go run ./cmd/lab gen --suite livecodebench --limit 12       # more problems from the sample
go run ./cmd/lab gen --suite livecodebench --difficulty easy   # keep only the easy tier
```

For this tier `--langs` selects the release version (`v1` through `v6`), which is how LiveCodeBench pins a date window: a higher version adds later problems.
`--limit N` caps how many problems are drawn, split between the two shapes, and `--all` takes as many as the scan budget allows.
`--difficulty easy|medium|hard` keeps only the named tier or tiers (comma separated), which is how a run is tuned to showcase or to stress a tool.
See [evals](/evals/) for how a suite is selected and how the trust boundary works across every tier.
