---
title: "Laguna-S-2.1 through pi: an incomplete board, and why it stayed incomplete"
linkTitle: "pi + laguna-s, incomplete"
description: "Third of the three per-tool boards on laguna-s-2.1-free, and the one that did not finish. pi was the last tool in the sweep, and by the time it started the free zen account was already deep into its rate-limit window from the two streams ahead of it. Only two tasks got genuine model time before the run was stopped for time, python-control-1064 and dspy-1651, and both engaged and failed the hidden tests without landing an edit. This is a short and honest writeup: pi's board is not comparable to the other two, and the reason is the free tier, not the tool."
date: 2026-07-22T13:30:00+07:00
---

Reproducibility header: tool=pi, model=laguna-s-2.1-free (free tier via the opencode.ai/zen upstream), suite=swebench-live, per-task attempt timeout 15m, one attempt per task.
Reproduce command, per task:

    source ~/data/.local.env   # OPENCODE_API_KEY for the zen upstream
    LAB_MODEL=laguna-s-2.1-free ./bin/lab run pi <task-id> --suite swebench-live

This board did not complete, and the honest thing to do is report exactly what ran and stop there rather than dress two data points up as a board.

## What ran

    task                              verdict  reqs  in_tok  out_tok  secs
    python-control__python-control-1064 fail     23   50657    2079    151
    stanfordnlp__dspy-1651            fail       11   34443    3181    286

Two tasks got real model time.
Both engaged, both ran a batch of quick requests, and both came back `FAIL: hidden tests not satisfied` with no edit captured in the gold file.
The remaining thirteen tasks either recorded a rate-limited zero-token non-attempt during the concurrent phase and were cleared for a clean rerun, or never reached that rerun before the sweep was stopped.

## Why it stayed incomplete

pi ran last, and by design.
The free zen account has one quota window, and the plan kept at most two streams on it at a time to avoid starving it.
pi was chained to start only after the opencode board finished, which meant it inherited whatever was left of the free window after two tools had already spent against it.

That is the same wall the opencode board documents from the proxy: HTTP 429 with a roughly sixteen-hour retry-after on the completions path once the window is spent.
pi's two engaged results, python-control and dspy, are the tasks that happened to land in an open slice of the window; both ran fast, twenty-three and eleven requests in a couple of minutes each, which is itself a sign the endpoint was briefly responsive for them.
The rest did not get that slice, and the run was stopped for time rather than wait out a sixteen-hour retry-after or spread the remaining tasks across days.

## The two failures

Both engaged failures are the explored-but-did-not-land shape.
python-control-1064 ran twenty-three requests in a hundred and fifty seconds and never wrote a gold-file edit, the same task that beat both other tools on this model, our agent engine on the clock and OpenCode on the 429 wall.
dspy-1651 ran eleven requests, produced the most completion tokens of the two, and also ended without a landed edit.
Neither is a clean read on pi's ceiling, because two tasks is not a board.

## What this set says

pi on laguna-s-2.1-free is not measured here.
Two tasks ran, both failed, and the reason the other thirteen did not run is the free-tier quota window, not anything pi did.
The takeaway is a process one for the next attempt: a fair three-tool board on a single free account cannot be run as three back-to-back sweeps, because the third tool starves.
Either give each tool its own key, or run one tool per quota window across separate days, or move to a paid tier where the window is not the binding constraint.
Until then, the pi column of this comparison is a placeholder, and the cross-tool writeup treats it as such.

Metrics: 2 of 15 tasks ran with model time, both engaged failures with no landed edit, the other 13 blocked by the free-tier rate-limit window; board intentionally not completed.
