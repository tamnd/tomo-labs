---
title: "Eighteen of eighteen, at radically different prices: six coding harnesses on recent LeetCode with gpt-5.6-luna"
linkTitle: "gpt-5.6-luna, LeetCode agent board"
description: "The same gpt-5.6-luna model solves the same recent easy, medium, and hard LeetCode tasks through leetcode-solver, tomo, pi, opencode, Codex, and Claude Code. Every cell passes LiveCodeBench's hidden execution tests, but the harness tax ranges from one model call and roughly one thousand tokens to eighteen calls and 359 thousand tokens. This is the complete pass@1 board, including cache, reasoning, latency, memory, and the one dropped stream that forced an infrastructure retry."
date: 2026-07-20T23:20:00+07:00
---

Reproducibility header: model=gpt-5.6-luna through the local Codex-subscription bridge at reasoning effort high; suite=LiveCodeBench code_generation_lite v6; dataset revision=`0fe84c3912ea0c4d4a78037083943e8f0c4dd505`, SHA-256=`bb4c364f71921c4495a6ad15abe1a927350b720009f4933e2e71f8af0f6fd1f5`; tomo=`a999294f812f79b2daac681650c32b638525e2bf`; tomo-labs=`c6808df75d9441a75e250f7806ce56cbed2d9dab`; leetcode-solver=`8f2d63e`; attempts=1; attempt timeout=900 seconds; network isolation=on.

Reproduce the board from sibling `leetcode-solver` and `tomo-labs` checkouts:

    go run ./cmd/leetcode-solver agent-bench \
      --skip-build --providers luna \
      --data .cache/agentbench-final

This is a harness comparison, not a model comparison.
Every cell uses the same model, problem statement, starter `solution.py`, hidden grader, one capability attempt, and prepared Python/pytest/Go image.
The tool container has no egress.
Only the trace proxy reaches the bridge.
The private tests live in a sibling host-side oracle and are never mounted into the agent.

## The three tasks

The selector scanned the pinned v6 split and chose the newest eligible functional LeetCode row in each difficulty:

    scenario       difficulty  contest date         hidden cases
    leetcode-3773  easy        2025-04-05 19:30     33
    leetcode-3793  medium      2025-03-29 19:30     44
    leetcode-3777  hard        2025-04-05 19:30     43

The untouched starter fails all three graders.
No reference solution or expected output appears in the prompt.
LiveCodeBench v6 ends in April 2025, so this is a recent held-out slice with real private tests, not a claim that the tasks postdate every model's training cutoff.

## The board

All token columns are the provider's accounting for the winning attempt.
Reasoning is a subset of output, not an extra charge.
Cached tokens are a subset of input.
Calls exclude the proxy's two readiness requests.

    tool             task             verdict  calls  input    cached   output  reason   total    wall   peak RSS
    leetcode-solver  leetcode-3773    PASS       1       428        0      578     412     1,006    11s      7.7MB
    leetcode-solver  leetcode-3793    PASS       1       474        0      620     460     1,094    14s      8.2MB
    leetcode-solver  leetcode-3777    PASS       1       719        0   11,387  10,876    12,106   206s      8.1MB

    tomo             leetcode-3773    PASS      12    33,408    9,216    2,107     882    35,515    56s     18.7MB
    tomo             leetcode-3793    PASS      13    38,394   12,800    2,575   1,001    40,969    69s     23.5MB
    tomo             leetcode-3777    PASS      14    55,480   14,080    7,778   4,338    63,258   169s     19.8MB

    pi               leetcode-3773    PASS       3     4,645        0      439     232     5,084    13s    166.1MB
    pi               leetcode-3793    PASS       5     8,751        0    1,015     591     9,766    29s    155.6MB
    pi               leetcode-3777    PASS       9    25,112    4,608    5,439   3,348    30,551   124s    165.2MB

    opencode         leetcode-3773    PASS      14    95,092   30,464    2,161     915    97,253    64s    557.2MB
    opencode         leetcode-3793    PASS      12    80,886   55,040    2,790   1,207    83,676    69s    601.3MB
    opencode         leetcode-3777    PASS      16   127,006   94,720    8,232   5,324   135,238   192s    586.7MB

    codex            leetcode-3773    PASS       9    69,600   32,512    2,183     667    71,783    53s     93.7MB
    codex            leetcode-3793    PASS       5    33,026   24,576    1,411     587    34,437    32s     97.8MB
    codex            leetcode-3777    PASS      11   108,801   64,256    5,090   2,377   113,891   109s     97.7MB

    claude-code      leetcode-3773    PASS       8   139,099   32,256    1,138     385   140,237    37s    286.8MB
    claude-code      leetcode-3793    PASS       7   121,586   97,792    1,168     490   122,754    38s    290.6MB
    claude-code      leetcode-3777    PASS      18   351,333  200,960    7,882   4,389   359,215   179s    305.1MB

Solved 18 of 18 at pass@1.
That clean score is the beginning of the result, not the end: the harnesses spent dramatically different amounts of model work to get it.

## Cost, including the cache discount

The bridge runs against a Codex subscription, so no per-token invoice was charged for this campaign.
The table prices the observed usage at the equivalent gpt-5.6-luna metered API list rate from the [shared pricing table](/guides/): $1.00 per million fresh input tokens, $0.10 per million cache-read tokens, and $6.00 per million output tokens.
Reasoning is already included in output and is not charged twice.
Fresh input is `input - cached`; each total is therefore `fresh input + cache read + output`.

| Tool | Task | Fresh input | Cache read | Output | Total | Relative to Pi |
|---|---|---:|---:|---:|---:|---:|
| leetcode-solver | leetcode-3773 | $0.000428 | $0.000000 | $0.003468 | $0.003896 | 0.54x |
| leetcode-solver | leetcode-3793 | $0.000474 | $0.000000 | $0.003720 | $0.004194 | 0.28x |
| leetcode-solver | leetcode-3777 | $0.000719 | $0.000000 | $0.068322 | $0.069041 | 1.29x |
| tomo | leetcode-3773 | $0.024192 | $0.000922 | $0.012642 | $0.037756 | 5.19x |
| tomo | leetcode-3793 | $0.025594 | $0.001280 | $0.015450 | $0.042324 | 2.85x |
| tomo | leetcode-3777 | $0.041400 | $0.001408 | $0.046668 | $0.089476 | 1.67x |
| pi | leetcode-3773 | $0.004645 | $0.000000 | $0.002634 | $0.007279 | 1.00x |
| pi | leetcode-3793 | $0.008751 | $0.000000 | $0.006090 | $0.014841 | 1.00x |
| pi | leetcode-3777 | $0.020504 | $0.000461 | $0.032634 | $0.053599 | 1.00x |
| opencode | leetcode-3773 | $0.064628 | $0.003046 | $0.012966 | $0.080640 | 11.08x |
| opencode | leetcode-3793 | $0.025846 | $0.005504 | $0.016740 | $0.048090 | 3.24x |
| opencode | leetcode-3777 | $0.032286 | $0.009472 | $0.049392 | $0.091150 | 1.70x |
| codex | leetcode-3773 | $0.037088 | $0.003251 | $0.013098 | $0.053437 | 7.34x |
| codex | leetcode-3793 | $0.008450 | $0.002458 | $0.008466 | $0.019374 | 1.31x |
| codex | leetcode-3777 | $0.044545 | $0.006426 | $0.030540 | $0.081511 | 1.52x |
| claude-code | leetcode-3773 | $0.106843 | $0.003226 | $0.006828 | $0.116897 | 16.06x |
| claude-code | leetcode-3793 | $0.023794 | $0.009779 | $0.007008 | $0.040581 | 2.73x |
| claude-code | leetcode-3777 | $0.150373 | $0.020096 | $0.047292 | $0.217761 | 4.06x |

This repricing changes the size, but not the direction, of the harness gap.
Caching makes Codex's medium run relatively inexpensive and cuts the effective price of the context-heavy tools, while output-heavy hard solutions remain costly even when their input prefix is cached.
The current Tomo baseline costs 5.19x, 2.85x, and 1.67x Pi on easy, medium, and hard respectively; those are the numbers the follow-up engine optimization must reverse without losing a hidden test.

## One call is enough when the task surface is narrow

`leetcode-solver` is deliberately specialized.
It sends the statement and starter to the model once, requires one Python code fence, writes that exact body, and lets the hidden runner decide.
There is no shell loop, repository search, package discovery, or conversational repair in this benchmark mode.

That specialization dominates the easy and medium cells.
Both finish in about one thousand total tokens, versus 5,084 for the next-smallest easy run and 9,766 for the next-smallest medium run.
The hard problem is different: Luna spends 10,876 reasoning tokens and does not produce first output for 197 seconds.
The call is long because the math is hard, but it is still one call, 12,106 total tokens, and a 43-case pass.

This is not evidence that a one-shot harness is universally better.
It is evidence that a LeetCode-shaped task benefits enormously from a LeetCode-shaped interface.
The general coding agents pay for capabilities the task does not need.

## Pi is the lean general agent

Pi is the clear efficiency winner among the five general coding agents.
It clears easy in three calls and 5,084 tokens, medium in five and 9,766, and hard in nine and 30,551.
Its memory footprint is higher than tomo or Codex because of the Node runtime, around 155–166 MB, but its model usage remains compact.

Tomo is second on model work, not memory.
It uses 12–14 calls and 35–63k tokens, with only 19–24 MB peak RSS.
Its plan primitive appears four or five times per task and the trace shows an explicit inspect/edit/test loop.
That loop costs context but converges on every tier.

## OpenCode and Claude Code pay the largest harness tax

OpenCode uses 83–135k tokens and roughly 557–601 MB RSS.
Claude Code uses 123–359k tokens and roughly 287–305 MB.
Both pass everything, so this is not a capability criticism.
It is the cost of their orchestration shape on a tiny one-file task: large system/tool schemas, repeated context, and many small tool turns around work that ultimately changes one method.

The hard Claude Code cell is the extreme.
Its successful attempt consumes 359,215 tokens across eighteen model calls.
Before that successful attempt, the first infrastructure attempt consumed another 168,470 tokens across nine completed calls, then received first bytes for call ten and stalled for 784 seconds without a final usage event.
The proxy marked the stream truncated; the 900-second attempt ceiling killed it and the harness retried without spending another capability attempt.
The retry then passed in 179 seconds.

That incident matters for interpreting the table.
The board reports the winning attempt so capability comparisons stay like for like.
The actual campaign also paid the discarded 168,470-token infrastructure attempt.
The trace preserves both; the report does not silently charge the model with a transport failure or silently erase the operational cost.

## Codex is cached heavily

Codex sits between tomo and the two heavier Node agents in total tokens, but much of its input is cached: 32,512 of 69,600 on easy, 24,576 of 33,026 on medium, and 64,256 of 108,801 on hard.
The initial harness revision recorded its tokens and request bodies but excluded native `/v1/responses` rows from normalized latency.
That measurement gap was fixed immediately afterward in tomo-labs PR #122; the raw timestamps still provide the wall figures above, and future boards carry native Responses TTFB and total latency directly.

## What this board says

Luna is strong enough that correctness does not separate these harnesses on this slice.
Efficiency does.

For an exact one-file algorithm contract, specialization wins by one to two orders of magnitude in token use.
Among general agents, pi is the leanest, tomo is the lightest in memory and methodical in orchestration, Codex benefits substantially from prompt caching, and OpenCode plus Claude Code spend the most context to reach the same green judge.

The important qualification is sample size.
Three problems cannot establish a universal ranking, and all three are Python functional tasks from the same April 2025 dataset window.
The point of this report is narrower and reproducible: on these exact hidden suites, under the same model and isolation contract, all six solve everything, and the harness alone changes total model work from roughly one thousand tokens to hundreds of thousands.

Metrics: 18/18 pass@1; 120 hidden cases per tool; model gpt-5.6-luna, effort high; one capability attempt; no agent egress; private tests never mounted; winning-attempt tokens shown; one Claude Code hard infrastructure retry preserved separately at 168,470 discarded tokens and a 784-second truncated stream.
