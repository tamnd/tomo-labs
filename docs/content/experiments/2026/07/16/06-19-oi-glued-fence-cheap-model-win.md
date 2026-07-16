---
title: "A glued fence in the oi block parser, and the cheap-model pass it cost"
linkTitle: "oi glued fence, a cheap-model harness fix"
description: "tomo's oi engine acts by writing fenced code blocks, so its block parser is on the hot path of every round. A cheap model routinely writes a closing fence glued straight onto the next opening fence, with no blank line between them, so two fence lines arrive as one. The old parser read that as code, swallowed the next block's body, and handed python3 -c a source that starts with a literal fence marker, which throws SyntaxError. On smolagents-285 that ate four of the model's eight rounds before it could reason about the fix at all, and the graded run missed by one test. Splitting a glued close-then-open fence fixes it: the re-run passes in six rounds with zero fenced-marker SyntaxErrors, and the oi grid goes from four of fifteen to five, level with codex-real."
date: 2026-07-16T06:19:00+07:00
---

tomo's oi engine is code-as-action.
The model does not call a structured tool, it writes a fenced code block, and the harness runs the block and feeds the output back.
That makes the block parser load-bearing: every round of every task passes through it, and a parser bug is a bug on the model's only way to act.
This run is one such bug, found on a task tomo lost by a single test, and the general fix that flipped the loss to a pass.

## Setup

The task is `smolagents__smolagents-285` from the fifteen-task SWE-bench-Live grid, run through the flat-rate bridge on `gpt-5.6-luna` at effort medium, graded, with a fourteen-minute cap.
The bridge drives one authorised subscription, so runs are serialized one consumer at a time.

    lab bridge --model gpt-5.6-luna --effort medium --port 8790 &

    lab probe smolagents__smolagents-285 \
      --engine oi --grade \
      --model gpt-5.6-luna --base-url http://localhost:8790/v1 \
      --out /tmp/oi-smol

The parser change is verified without spending a token, because it is deterministic.

    go test ./pkg/engine/oi -run TestParseBlocks

## The bug

The model means to write two blocks with a blank line between them.

    ```python
    print("ready")
    ```
    ```sh
    ls -la
    ```

A cheap model often drops the blank line and writes the closing fence glued straight onto the next opening fence, so the two fence lines collide into one.

    ```python
    print("ready")
    ``````sh
    ls -la
    ```

The old parser recognised a closing fence as a bare run of the fence character with no trailing tag.
The glued line carries a tag after the run, the `sh`, so it did not read as a close.
It fell through to code, and the trailing fence plus the whole next block's body were captured as the first block's body.
That body then failed to run, because `python3 -c` chokes on a literal ` ```sh ` sitting in its source and returns a `SyntaxError`.
The model reads a syntax error it did not write, spends a round recovering, and the same glue happens again on the next reply.

## The result

Both runs are the same task, model, bridge, and grader, differing only in the binary's parser.

| Run | Binary | Rounds | Fenced-marker SyntaxErrors | fail_to_pass | Graded |
|---|---|---|---|---|---|
| Graded grid | before fix | 8 | 4 | red | FAIL |
| Re-run | after fix | 6 | 0 | green | PASS |

Before the fix, four of the eight rounds died on `SyntaxError` from the glued fence, not on the fix.
The graded run still edited the right file and reached sixty-five of sixty-six tests, missing only the one case the model never got the budget to reason about.
After the fix, the re-run spends none of its rounds on fence noise, finishes in six, and the `fail_to_pass` test comes back green.

## The fix is a parser split, not task tuning

The change splits a glued close-then-open fence.
Inside an open block, a line whose leading run both closes the current fence and, in what follows, opens a fresh fence carrying a language tag, ends the current block and starts the next one.
It fires only when the trailing run is itself a tagged opener.
A bare over-long close such as ` `````` ` on its own is still just a long close, and a lone ` ```sh ` with no preceding close is still code, which is what CommonMark already does.
So the fix adds one real case and changes nothing else, and it is locked by `TestParseBlocksGluedCloseOpen`, `TestParseBlocksGluedChain`, and `TestParseBlocksBareLongCloseIsNotReopen` in `codeblock_test.go`.

This is why it is a harness fix and not a smolagents fix: any reply that chains fences without a blank line now parses and runs the way the model meant, on every task and every cheap model that writes fences this way.

## What it does to the grid

smolagents-285 was tomo-oi's one honest correctness loss on the grid, a task both rivals pass.
It read like a last-mile model miss until you read where the eight rounds went, and half of them went to fence noise the parser created.
With the parser fixed the task passes, which takes tomo-oi from four of fifteen to five of fifteen.

    PASS: tomo-oi 5/15   tomo-cx 2/15   codex-real 5/15

That is level with codex-real and still ahead of tomo-cx.
The passing re-run cost 96,072 fresh and 42,240 cached input tokens over its six rounds, against the failing graded draw's higher effective cost, so the fix is cheaper as well as correct.
There is now no task on this grid the model can reach where tomo-oi trails both rivals.

## Lessons

- The parser is on the hot path of a code-as-action engine, so a parser bug is a bug on the model's only way to act, and it shows up as wasted rounds rather than a clean error.
- Cheap models are exactly the ones that glue fences together, and the oi harness exists to serve cheap models, so this class of bug is not an edge case for it.
- A single-test loss is worth reading round by round before calling it a model ceiling. Here four of eight rounds were spent on fence noise, not on the fix, and the ceiling was never the real limit.
- The fix stays inside CommonMark. It only splits a close that is immediately followed by a tagged opener, so a bare long close and a lone opener behave exactly as before, and the whole change is covered by deterministic unit tests that need no bridge spend.

## Reproduce

The parser change is deterministic, so the correctness claim is a unit-test run with no model in the loop.

1. Build the lab against the local tomo checkout: `go build -o /tmp/lab ./cmd/lab`.
2. Run the parser tests: `go test ./pkg/engine/oi -run TestParseBlocks`. The three glued-fence cases pass on the fixed binary and the bare-long-close case confirms a plain long close is left alone.
3. For the graded run, start `lab bridge` on `gpt-5.6-luna` first, then point the probe at it with `--base-url`. The bridge drives one subscription, so run one consumer at a time.
4. Read the run back with `lab probe analyze /tmp/oi-smol`, which prints the round-by-round token curve and the per-round blocks without spending a token, so the four fenced-marker SyntaxErrors on the old binary and zero on the new one are visible in the transcript.
5. Every run writes `summary.json` with the graded result and token counts, `trace.jsonl` with each call, and `transcript.md` with the readable turns.
