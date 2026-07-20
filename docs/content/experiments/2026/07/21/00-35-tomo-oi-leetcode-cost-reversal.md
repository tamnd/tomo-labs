---
title: "Tomo reverses the Pi cost gap on LeetCode: 24.5% of the tokens, 49.5% of the price"
linkTitle: "Tomo OI reverses the Pi LeetCode cost gap"
description: "The first Luna LeetCode board found Tomo correct but expensive: twelve to fourteen model calls and up to 63 thousand tokens per problem. A trace-led engine shootout selects OI, removes redundant inspection and acknowledgement rounds, observes edits in Git and plain workspaces, and hardens malformed fence handling. The passing three-task run keeps all 120 hidden cases green while using 11,125 tokens and $0.037475 of API-equivalent list cost—24.5 percent of Pi's tokens and 49.5 percent of its cost. Failed arms and unrelated harness checks are included so the result is not a prompt tuned to three answers."
date: 2026-07-21T00:35:00+07:00
---

Reproducibility header: task set=the same three LiveCodeBench code_generation_lite v6 rows as the [six-agent Luna board](/experiments/2026/07/20/23-20-luna-leetcode-agent-board/); model=gpt-5.6-luna through the local Codex-subscription bridge at reasoning effort high; dataset revision=`0fe84c3912ea0c4d4a78037083943e8f0c4dd505`, SHA-256=`bb4c364f71921c4495a6ad15abe1a927350b720009f4933e2e71f8af0f6fd1f5`; base tomo=`a999294f812f79b2daac681650c32b638525e2bf`; optimized implementation merged as tomo PR #88, merge=`4357c0feff1cc9df53479d0a48079284dd915259`; one capability attempt per recorded cell; timeout=900 seconds; agent network=off; hidden grader outside the container.

This is the answer to the uncomfortable line in the first board.
Tomo solved all three problems, but used 5.19x, 2.85x, and 1.67x Pi's API-equivalent cost.
The target for this follow-up was not a cosmetic prompt trim.
It was to keep every hidden case green while bringing Tomo to at most 70 percent of Pi's cost on easy, medium, and hard, using changes that remain useful on ordinary coding work.

The result clears that ceiling in all three cells.
Easy costs 52.1 percent of Pi, medium 37.3 percent, and hard 52.5 percent.
Across the slice, optimized Tomo uses 24.5 percent of Pi's tokens and 49.5 percent of its price.

## The passing result

The hidden grader is unchanged from the original board.
The prompt contains the statement, required `Solution` method, and starter file, but no reference answer or hidden expected values.
Reasoning is included in output rather than added to it; cached input is included in input.

| Task | Hidden verdict | Model calls | Input | Cached | Output | Reasoning | Total tokens | Wall | Peak RSS |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| leetcode-3773, easy | PASS 33/33 | 2 | 1,582 | 0 | 368 | 155 | 1,950 | 8s | 15.0 MB |
| leetcode-3793, medium | PASS 44/44 | 2 | 1,724 | 0 | 635 | 368 | 2,359 | 13s | 15.3 MB |
| leetcode-3777, hard | PASS 43/43 | 2 | 2,549 | 0 | 4,267 | 3,655 | 6,816 | 81s | 15.1 MB |
| **total** | **PASS 120/120** | **6** | **5,855** | **0** | **5,270** | **4,178** | **11,125** | **102s** | — |

These are provider usage events, not token estimates from text length.
The second call in each trace is a short acknowledgement after a successful action.
PR #88 subsequently teaches OI to stop after an observed edit ends in a green verification, so that acknowledgement is no longer required by the merged engine.
It is deliberately still charged here: the table reports what the passing trace spent, not a projection of what the later guard might save.

## Cost breakdown

The bridge uses a Codex subscription, so this is API-equivalent list pricing rather than an invoice.
The shared gpt-5.6-luna rates are $1.00 per million fresh input tokens, $0.10 per million cached input tokens, and $6.00 per million output tokens.
The optimized traces have no cache reads, so `total = input + 6 × output`, in million-token dollars.

| Task | Fresh input | Cache read | Output | Total | Related to Tomo baseline | Related to Pi |
|---|---:|---:|---:|---:|---:|---:|
| leetcode-3773 | $0.001582 | $0.000000 | $0.002208 | **$0.003790** | 0.10x | **0.52x** |
| leetcode-3793 | $0.001724 | $0.000000 | $0.003810 | **$0.005534** | 0.13x | **0.37x** |
| leetcode-3777 | $0.002549 | $0.000000 | $0.025602 | **$0.028151** | 0.31x | **0.53x** |
| **total** | **$0.005855** | **$0.000000** | **$0.031620** | **$0.037475** | **0.22x** | **0.49x** |

“Related to Tomo baseline” uses the original board's Tomo row for the same task: $0.037756, $0.042324, and $0.089476.
“Related to Pi” uses $0.007279, $0.014841, and $0.053599.
The reductions against Tomo are 90.0 percent on easy, 86.9 percent on medium, and 68.5 percent on hard.
Nothing was padded to force the medium cell into a 50–70 percent band; 37.3 percent is a better result than the 70 percent ceiling.

Token totals make the direction even clearer:

| Task | Tomo baseline | Pi | Tomo OI after | After / Pi | Token reduction from Tomo |
|---|---:|---:|---:|---:|---:|
| leetcode-3773 | 35,515 | 5,084 | 1,950 | 0.38x | 94.5% |
| leetcode-3793 | 40,969 | 9,766 | 2,359 | 0.24x | 94.2% |
| leetcode-3777 | 63,258 | 30,551 | 6,816 | 0.22x | 89.2% |
| **total** | **139,742** | **45,401** | **11,125** | **0.25x** | **92.0%** |

## The trace said orchestration, not environment setup

The baseline easy trace is the cleanest diagnosis.
It takes twelve model calls around one small method:

    plan
    grep solution.py
    read solution.py
    plan
    edit solution.py
    plan
    run `python` (missing in the prepared image)
    rerun with `python3` and an invented wrong expectation
    plan
    rerun after correcting that expectation
    plan
    final acknowledgement

The container was already prepared with Python 3, pytest, Go, Node, Git, and the task file.
There was no dependency installation, network lookup, or slow repository checkout in this path.
The wasted time and tokens came from the agent loop: a plan primitive before and after obvious one-file actions, rereading content already present in the task surface, selecting the absent `python` alias, writing an untrusted ad-hoc expected value, and buying a final model call after the executable check was green.

The hard baseline is slower because the model does real mathematical reasoning—4,338 reasoning tokens in the original passing trace—but it has the same orchestration multiplier around that work.
The environment is not the reason fourteen model calls are required.

## Agent, CX, or OI?

Before changing the loop, all three Tomo engines ran the same easy task with Luna.
This is an engine selection, not three differently prompted LeetCode solvers.

| Engine | Verdict | Calls | Input | Cached | Output | Total tokens | List cost |
|---|---:|---:|---:|---:|---:|---:|---:|
| agent rerun | PASS 33/33 | 11 | 29,827 | 8,192 | 1,758 | 31,585 | $0.033002 |
| CX | PASS 33/33 | 14 | 45,922 | 15,616 | 2,292 | 48,214 | $0.045620 |
| OI before the compact loop | PASS 33/33 | 4 | 7,109 | 0 | 1,421 | 8,530 | $0.015635 |

CX is worse than the baseline on this narrow task, and agent remains plan-heavy.
OI starts with no tool-schema tax and represents actions as fenced code, cutting the easy trace to four calls before any task-specific tuning.
That is why the optimized path is OI.

## What changed

The largest changes sit outside any benchmark-specific system prompt.

1. **Stop on observed, verified completion.** OI fingerprints the workspace before and after an action. If the response changed files and its last action was a successful test, build, type check, lint, compile, or assertion, the engine returns immediately instead of asking the model to summarize success.
2. **Observe the work that benchmarks actually use.** Raw `git status --porcelain` cannot distinguish two versions of an already-untracked starter file: both are just `?? solution.py`. Dirty Git paths now carry content hashes. Plain non-Git directories receive a bounded, content-sensitive filesystem fingerprint, excluding `.git` and Tomo's own `.tomodata` state.
3. **Keep edit and verification distinct.** A Python heredoc is not automatically a test merely because it ran. The guard recognizes `assert` and real check commands, and a regression test proves an edit-only heredoc does not trigger early completion.
4. **Recover valid actions from noisy fences.** Luna occasionally emitted a correct closing fence followed by a short stray suffix such as `mbilu` or a non-Latin fragment. The old parser appended that suffix to the Python program and created a syntax error. The parser now accepts a same-length close with a bounded suffix, after the existing glued-fence cases, with ASCII and Unicode regression tests.
5. **Use a short, global OI contract.** The system prompt says to act directly when the user already supplies one target and its contents, to end an edit with the smallest relevant executable check, to use `python3` in shell, and not to invent unstated expected results. It does not mention LeetCode, these problem numbers, hidden tests, dynamic programming, palindromes, products, or any expected answer.

PR #88 carries the implementation as three reviewable commits: fence recovery, verified convergence, and the compact edit/verify contract.
Its full Go race suite and GitHub lint/test jobs pass.

## The failed arms are part of the result

The final table is not the first draw that happened to be green.
The traces preserve the rejected directions:

- One easy run ended with a noisy closing fence. Before the parser fix, valid Python became a syntax error.
- Another easy run invented the expectation `[3,2,1] == 2`, discovered it was wrong, and spent a repair round. That produced the general “do not hard-code unstated expected results” rule.
- A hard candidate returned `11` for the hidden singleton case `nums=[11], k=11, limit=10`, where the limit makes `-1` correct. This is a real capability failure, not infrastructure, and is not counted as a pass.
- An attempted prompt requiring a hand-built exhaustive oracle overcorrected. The hard run exceeded $0.071 before completion and the easy run grew to 7,198 tokens. It was removed. A universal demand for brute-force differential testing is expensive and inappropriate for many repositories.
- A later hard draw spent 26,436 tokens and still did not produce a grade. It is also excluded from the passing board and retained as evidence that pass@1 variance remains.

These failures constrain the claim.
The passing slice proves the cost target is attainable without changing the grader; it does not prove three-task pass@1 stability at arbitrary sampling seeds.

## General harness checks

The anti-overfitting check runs the merged engine shape on three unrelated, offline, non-Git workspaces with the free `hy3-free` model.
These tasks do not contain LeetCode statements or hidden algorithm judges.

| Harness | Work | Verdict | Calls | Tokens | What the trace shows |
|---|---|---:|---:|---:|---|
| 03-bugfix-fizzbuzz | inspect and fix a Python mean calculation | PASS | 3 | 2,002 | inspection is retained because file contents were absent; edit then executable check |
| 06-codegen-primes | create, build, and run a Go program | PASS | 2 | 1,253 | correct program on the first action; one small acknowledgement in this measured build |
| 07-refactor-dedupe | remove a duplicate JS function without changing exports | PASS | 6 | 7,157 | the first edit accidentally removed exports; the failing test is read, repaired, and rerun green |

The refactor is important because it shows the loop was not reduced to “write once and declare victory.”
When a real test fails, OI keeps going.
The code-generation task is the opposite boundary: before the plain-workspace fingerprint, it had already produced the correct program after its first action but repeated build and run until nineteen calls and about 27 thousand tokens.
After the fingerprint it passes with two measured calls, and PR #88 removes the final acknowledgement when the last action is a recognized verification.

## Reproduction boundary

The three passing Luna traces were produced during the optimization worktree based on tomo `a999294`, before that worktree was split into commits and merged.
The recorded trace includes a second acknowledgement call; the merged `4357c0f` contains the same compact prompt and solution-action path plus the later, stricter verified-completion guard.
That distinction is why this page does not pretend the passing usage was generated from the merge SHA.

An exact post-merge Luna rerun was attempted on 2026-07-21.
The Codex subscription bridge returned `429 usage_limit_reached` on four consecutive requests, with reset time 2026-07-27 22:23:13 +07:00.
The failed infrastructure attempt has zero model tokens and is not a capability result.
Until the quota resets, the honest evidence is the complete passing candidate trace, deterministic Go tests for the merged behavior, and the unrelated live Zen harness runs—not a fabricated post-merge number.

Metrics: optimized Luna candidate PASS 120/120 hidden cases; 11,125 provider tokens; $0.037475 API-equivalent list cost; 24.5% of Pi tokens; 49.5% of Pi cost; merged Tomo PR #88; full `go test -race -count=1 ./...` green; unrelated offline bug-fix/codegen/refactor harnesses 3/3 green; post-merge Luna rerun blocked by a recorded subscription 429 until 2026-07-27.
