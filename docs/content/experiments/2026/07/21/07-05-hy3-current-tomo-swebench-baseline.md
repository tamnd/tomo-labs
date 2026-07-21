---
title: "Hy3 on current Tomo solves five of fifteen, and unbounded completions consume the run"
linkTitle: "Hy3 + current Tomo, the 15-task baseline"
description: "The post-LeetCode Tomo OI baseline runs all fifteen offline SWE-bench-Live tasks on hy3-free at pass@1. It solves five, including Kubernetes Python after a ten-minute first completion, and records 1.72 million provider-reported tokens. That number is a lower bound: 29.6 MB of response traces include multi-megabyte streams with missing usage, and Sphinx reports zero tokens after 900 seconds. The trace separates three costs—environment preparation, healthy iterative work, and unbounded model completions—and lands four general harness fixes before the old-Tomo and rival arms run."
date: 2026-07-21T07:05:00+07:00
---

Reproducibility header: suite=all 15 checked-in `swebench-live` scenarios; model=`hy3-free` through OpenCode Zen, observed provider=`Novita`, upstream model=`tencent/hy3:free`; tool=`tomo-oi`; Tomo pin=`4947c32b83af8c93e9feaa91f4cbb01c10b5e5ec`; one capability attempt per scenario; scored timeout=900 seconds; environment-prep timeout=300 seconds after the harness fix; concurrency=1; agent network=off; hidden grader on the host; campaign data=`/Users/apple/data/tomo-labs/hy3-swebench-20260721/tomo-before`; actual free-tier invoice=$0; paid-twin list reference=$0.20/M fresh input, $0.05/M cached input, $0.80/M output.

This is the first full arm after Tomo's compact OI system contract landed for the LeetCode work.
It is intentionally a baseline, not the optimized SWE-bench result.
Every task receives the same repository-level prompt, isolated checkout, prepared tools, and hidden host grader.
No task name, gold patch, hidden test, or expected output is added to Tomo's prompt.

The result is five passes of fifteen.
The more important finding is why the other ten consume time.
Some are ordinary capability misses.
Some are long but healthy agent loops.
Five unrelated repositories produce multi-minute, multi-megabyte completions before Tomo can execute the next action.
The engine currently gives one model call no output ceiling, so one bad call can consume two-thirds of the entire task allowance.

## The complete pass@1 board

Input includes cached input; reasoning is a subset of output, not an additional token class.
`Stop` is the runner's recorded termination class.
All token values are provider-reported and therefore lower bounds where a stream did not finish with usage.

| Task | Verdict | Stop | Calls | Input | Cached | Output | Reasoning | Reported total | Wall |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| cfn-lint-3798 | FAIL | turns | 18 | 174,877 | 142,080 | 16,048 | 13,253 | 190,925 | 385s |
| briefcase-2085 | **PASS** | natural | 4 | 9,636 | 6,016 | 1,775 | 1,180 | 11,411 | 47s |
| conan-17123 | **PASS** | natural | 6 | 12,704 | 8,384 | 1,522 | 61 | 14,226 | 44s |
| gitingest-94 | **PASS** | natural | 5 | 9,001 | 5,376 | 1,001 | 532 | 10,002 | 31s |
| dynaconf-1225 | FAIL | timeout | 41 | 490,697 | 444,160 | 10,087 | 1,757 | 500,784+ | 900s |
| fonttools-3682 | **PASS** | natural | 13 | 48,954 | 39,552 | 2,957 | 1,711 | 51,911 | 102s |
| smolagents-285 | FAIL | natural | 6 | 13,266 | 9,664 | 5,368 | 4,300 | 18,634 | 94s |
| instructlab-2540 | FAIL | turns | 16 | 93,100 | 81,280 | 1,116 | 159 | 94,216 | 57s |
| faker-2142 | FAIL | natural | 3 | 51,954 | 23,808 | 22,042 | 4,679 | 73,996+ | 555s |
| kubernetes-python-2303 | **PASS** | natural | 11 | 23,054 | 18,048 | 1,882 | 86 | 24,936+ | 661s |
| mesa-2394 | FAIL | natural | 2 | 87,132 | 0 | 16,549 | 1,472 | 103,681+ | 210s |
| python-control-1064 | FAIL | turns | 35 | 220,834 | 200,576 | 5,478 | 1,980 | 226,312 | 214s |
| sqllineage-661 | FAIL | timeout | 26 | 266,352 | 229,888 | 32,582 | 29,796 | 298,934 | 900s |
| sphinx-12975 | FAIL | timeout | 2 | 0 | 0 | 0 | — | **0+** | 900s |
| dspy-1651 | FAIL | turns | 17 | 92,492 | 78,848 | 10,958 | 9,906 | 103,450 | 271s |
| **total** | **5/15** | 3 timeout, 4 turns | **205** | **1,594,053** | **1,287,680** | **129,365** | **70,872** | **1,723,418+** | **5,371s** |

The table's call total uses completed model calls from orchestration traces.
The raw HTTP request counter is 220 because it also sees readiness and incomplete traffic.
The lab aggregate calls seven capped cells out separately and reports 5/8 among uncapped attempts; for a full pass@1 capability board the denominator remains all fifteen, so the headline is 5/15.

The passing set is Briefcase, Conan, Gitingest, FontTools, and Kubernetes Python.
Kubernetes is the revealing pass: the first useful action does not begin until an approximately ten-minute completion closes, then Tomo fixes the task in about one additional minute.
The model and loop can solve it, but the completion boundary wastes most of the allowance before the loop can work.

## Cost: zero invoice, $0.229151 reported-token floor

`hy3-free` billed nothing.
Dollar comparisons between free arms are therefore all exactly $0 and cannot establish that one tool is cheaper than another.
For a consumption comparison, the lab applies the public paid `tencent/hy3` twin's list rates to the tokens the free route reported.

| Component | Tokens | Rate per million | Reference cost |
|---|---:|---:|---:|
| Fresh input | 306,373 | $0.20 | $0.061275 |
| Cached input | 1,287,680 | $0.05 | $0.064384 |
| Output | 129,365 | $0.80 | $0.103492 |
| **reported-token total** | **1,723,418** | — | **$0.229151+** |
| **actual free-route invoice** | — | — | **$0.000000** |

The plus sign matters.
This is not a complete cost estimate.
The fifteen traces contain 29,614,467 response bytes, while Sphinx records zero usage and several other tasks omit usage for their largest response.
Charging only finalized provider events makes the reference number a strict floor, not a bargain.

| Task | Fresh input | Cached input | Output | Paid-twin reference |
|---|---:|---:|---:|---:|
| cfn-lint | 32,797 | 142,080 | 16,048 | $0.026502 |
| briefcase | 3,620 | 6,016 | 1,775 | $0.002445 |
| conan | 4,320 | 8,384 | 1,522 | $0.002501 |
| gitingest | 3,625 | 5,376 | 1,001 | $0.001795 |
| dynaconf | 46,537 | 444,160 | 10,087 | $0.039585+ |
| fonttools | 9,402 | 39,552 | 2,957 | $0.006224 |
| smolagents | 3,602 | 9,664 | 5,368 | $0.005498 |
| instructlab | 11,820 | 81,280 | 1,116 | $0.007321 |
| faker | 28,146 | 23,808 | 22,042 | $0.024453+ |
| kubernetes-python | 5,006 | 18,048 | 1,882 | $0.003409+ |
| mesa | 87,132 | 0 | 16,549 | $0.030666+ |
| python-control | 20,258 | 200,576 | 5,478 | $0.018463 |
| sqllineage | 36,464 | 229,888 | 32,582 | $0.044853 |
| sphinx | 0 | 0 | 0 | **$0.000000+** |
| dspy | 13,644 | 78,848 | 10,958 | $0.015438 |

## The trace has three different kinds of slow

### 1. Environment preparation

Smolagents initially looked like a model stall before the model ran.
Its `test` extra recursively includes every integration: Torch, Triton, transformers, UI, audio, and more.
The old prep order selected that extra first, filled the host, and never reached Hy3.

The general fix prefers the normal project dependency set, installs pytest separately, and uses extras only as fallbacks.
The normal Smolagents package still declares TorchVision as a core dependency, so its shared cache reaches 4.7 GiB.
Because cache and venv are separate bind mounts, uv could not hardlink and copied another 3.4 GiB into one throwaway attempt.
Setting the prep container's link mode to symlink reduced that active venv to 2.6 MiB, approximately 1,300 times smaller, while the stable cache remained mounted for the attempt.

InstructLab adds a second case: a native dependency needs a compiler absent from the common prep image.
The prep path now tries a source-only editable install before broad optional extras, leaving the checkout and pytest usable rather than repeating an impossible dependency graph.

SQLLineage adds a third: package metadata launches `npm run build`, and alternative install forms repeat it.
Environment prep is now bounded at 300 seconds through `LAB_PREP_TIMEOUT`; a timeout removes the prep container and starts the agent with the partial environment.
This is outside scored wall time and consumes no capability attempt.

These changes landed as tomo-labs PRs #131, #132, #133, and #134.
They apply to every tool and task.

### 2. Healthy but expensive iteration

Python-control is not a streaming anomaly.
It makes 35 completed model calls in 214 seconds and reports 226,312 tokens.
The loop keeps investigating and editing until its turn guard ends the run, but hidden tests remain red.
That is a convergence-governor problem: many normal calls, real work, no correct finish.

SQLLineage mixes both shapes.
It advances in bursts, then waits on individual long calls, reaching 26 completed calls and 298,934 tokens before the 900-second ceiling.
The task still fails.

### 3. Unbounded individual completions

Five tasks make the clearest case:

| Task | Largest raw response | Usage events / HTTP requests | Scored wall | Verdict |
|---|---:|---:|---:|---:|
| dynaconf | 2,871,673 bytes | 39 / 42 | 900s | FAIL, timeout |
| faker | 1,942,921 bytes | 3 / 4 | 555s | FAIL |
| kubernetes-python | 2,990,713 bytes | 10 / 12 | 661s | **PASS** |
| mesa | 1,629,929 bytes | 2 / 3 | 210s | FAIL |
| sphinx | 2,527,515 bytes | 0 / 3 | 900s | FAIL, timeout |

The response files are raw SSE, not a text-token estimate.
Their size proves substantial output occurred even when the provider never finalized usage.
On Dynaconf and Kubernetes, request two runs for about ten minutes before Tomo can execute the next action.
Sphinx spends the full ceiling across two such streams and reports zero tokens.

This points to a global engine control: bound one completion while leaving the total task budget intact.
It does not justify shortening the 900-second task ceiling, changing one task's prompt, or leaking a hidden test.
The same model behavior appears across five unrelated repositories.

## Campaign-integrity fixes discovered before the board

The first Dynaconf timeout exposed a pass@1 bug.
Killing a live SSE connection at the wall ceiling leaves a truncated response, and the runner mistook that truncation for a retryable gateway drop.
It started an impossible `attempt 2/1`.

Tomo-labs PR #130 now excludes timeout exit code 124 from infrastructure stream retries.
A real dropped stream remains retryable; a timeout grades the partial tree once and stops.
The regression test covers both classes.

The invalid pre-fix retry was stopped and never entered this board.
One separately interrupted InstructLab setup produced an exit `-1` row with no grader verdict; its whole timestamped directory was moved, intact, under `invalid-interrupted` before aggregation.
The published dataset contains exactly fifteen canonical result files.

## What this baseline does and does not say

It says current Tomo OI plus the current Hy3 free route solves five of fifteen at pass@1.
It says setup was a real source of delay on heavyweight historical packages, and those general harness problems are now bounded.
It says the dominant scored latency is currently unbounded model completions, not Python setup, hidden grading, or a mathematical task needing more thought.

It does not say the run cost only $0.229151 at paid rates.
That number prices only reported tokens and is a floor.
It does not say 5/8 merely because the lab excludes capped cells from its uncapped median population.
The capability denominator is fifteen.
It does not yet say Tomo beats Pi or OpenCode.
Those full arms have not run in this new campaign.

Next comes the requested historical control: all fifteen tasks on exact pre-LeetCode Tomo OI commit `08f88f6`, still on Hy3 and the corrected common harness.
Then Pi and OpenCode run the same grid.
Only after those reports merge will Tomo be optimized, across multiple harnesses, for more passes and lower paid-twin reference consumption than both rivals.

Metrics: 15 canonical pass@1 scenarios; 5 passes; 1,723,418 provider-reported tokens plus missing usage; 29.6 MB raw response traces; 5,371 scored seconds; 220 HTTP requests; actual invoice $0; paid-twin reference floor $0.229151; harness PRs #130 through #134 merged; no task-specific prompt or hidden-test change.
