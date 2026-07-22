---
title: "Laguna-S-2.1, three harnesses, one free key: tomo-agent, OpenCode, and pi on swebench-live"
linkTitle: "laguna-s three-way comparison"
description: "Poolside's Laguna-S-2.1 is a 118B mixture-of-experts coder, free to call on the opencode.ai/zen tier as laguna-s-2.1-free. This is the cross-tool writeup that sits on top of the three per-tool boards: our own agent engine, OpenCode, and pi, all driving the same model against the same fifteen swebench-live tasks. The headline is that the two harnesses that got a fair shot trade wins rather than one dominating, tomo-agent takes briefcase and OpenCode takes smolagents while both take gitingest, and that the single biggest determinant of a task's outcome was not the tool but whether the free tier's rate-limit window was open when it ran. It also settles the two feasibility questions: Laguna-S is free on zen and too large to self-host, and Laguna-XS is small enough to run on the local 4090."
date: 2026-07-22T13:45:00+07:00
---

This is the comparison that sits on top of three per-tool boards run the same day:
[tomo-agent]({{< relref "13-00-tomo-agent-laguna-s-swebench-board" >}}), [OpenCode]({{< relref "13-15-opencode-laguna-s-swebench-board" >}}), and [pi]({{< relref "13-30-pi-laguna-s-swebench-board" >}}).
One model, laguna-s-2.1-free, fifteen swebench-live tasks, three harnesses, graded by the same hidden checks.

## Feasibility, first, because it was the original question

Can we run the two Laguna checkpoints Poolside published, and does the free zen key reach them.

Laguna-S-2.1 is 118B total parameters, about 8B active as a mixture-of-experts, 1M context.
Its GGUF does not fit the gamingpc's RTX 4090 at a useful quant, so the S size is not self-hostable on that box.
It is live and free on zen as laguna-s-2.1-free, verified against the account, which is what all three boards run against.

Laguna-XS-2.1 is the small sibling, 33B total and about 3B active.
Its Q4_K_M GGUF is 20.3GB, fits the 4090 with a 32K context, and ollama 0.32.1 supports the architecture natively.
It pulls, runs at full GPU offload, and as a local smoke test it solved gitingest-94 through the same probe path, edit in the gold file.
So: S is free on zen and cloud-only for us, XS is local-hostable on the 4090 and clears the easy localized tasks on its own hardware.

One harness fact underpins every board here.
Laguna emits tool calls in two dialects, the native OpenAI structured `tool_calls` array and a text form, `<tool_call>name<arg_key>K</arg_key><arg_value>V</arg_value></tool_call>`.
Our oi engine, which sends no tools and parses per-model text, does not recognize the text form and comes back with zero actions.
The agent engine, which offers native tools and reads the structured array, drives it correctly.
So the per-model rule for Laguna is: agent engine, not oi. All three tools here use native tool calls.

## The board, side by side

Verdicts on all fifteen tasks. `429` marks a run the free tier rate-limited to zero tokens, `N/A` a task the harness could not start, and a blank a task that never ran before the sweep was stopped for time.

    task                              tomo-agent  opencode  pi
    cyclotruc__gitingest-94              PASS        PASS     -
    beeware__briefcase-2085              PASS        fail     -
    huggingface__smolagents-285          fail        PASS     -
    conan-io__conan-17123                fail        fail     -
    dynaconf__dynaconf-1225              fail        fail     -
    fonttools__fonttools-3682            fail        fail     -
    instructlab__instructlab-2540        fail        fail     -
    aws-cloudformation__cfn-lint-3798    N/A         fail     -
    joke2k__faker-2142                   fail        429      -
    kubernetes-client__python-2303       fail        429      -
    projectmesa__mesa-2394               fail        429      -
    python-control__python-control-1064  fail        429     fail
    reata__sqllineage-661                fail         -       -
    sphinx-doc__sphinx-12975             fail         -       -
    stanfordnlp__dspy-1651               fail         -      fail

    solved                                2/14       2/8    0/2
    (gradeable / model-engaged tasks)

tomo-agent completed all fifteen (cfn-lint N/A on its path) and solved two.
OpenCode ran twelve before the stop, four of them rate-limited to zero, and solved two of the eight that got model time.
pi ran last, inherited a spent quota window, and got only two tasks of model time before the run was stopped.

## The two harnesses that got a fair shot trade wins

On the seven tasks both tomo-agent and OpenCode actually engaged and could be graded, the record is a 2 to 2 tie that splits on which task each one wins.

    task            tomo-agent   opencode
    gitingest-94       PASS         PASS      both clear the localized floor
    briefcase-2085     PASS         fail      tomo holds a fix against failing visible tests
    smolagents-285     fail         PASS      opencode grinds 47 rounds to land it
    conan-17123        fail         fail
    dynaconf-1225      fail         fail
    fonttools-3682     fail         fail
    instructlab-2540   fail         fail

Both clear gitingest, the well-localized `http://` prefix fix, in a handful of moves.

briefcase is tomo-agent's win and it is a reasoning win.
Its trace makes the `insteadOf`/`old_url` fix, runs the visible tests, watches them fail because they assert the old behavior, and correctly keeps the change on the reasoning that the hidden suite grades the new behavior.
OpenCode spent nine requests on the same file and did not converge on that change.

smolagents is OpenCode's win and it is an endurance win.
tomo-agent got nine rounds and ended one fail_to_pass test short at `1 failed, 65 passed`.
OpenCode took forty-seven requests, 565K prompt tokens with 408K served from cache, and landed it.
The model can close smolagents, but only if the harness lets it take enough rounds; nine was not enough and forty-seven was.

That trade is the actual finding of the comparison.
On this model neither harness is strictly better; they fail the same hard middle and each converts one task the other misses, one by holding a correct fix and one by refusing to stop early.

## The real confound was the free tier, not the tool

The single biggest determinant of a row's outcome was whether the free quota window was open when it ran.
The proxy log is unambiguous on the four OpenCode zero-token rows: one 200 to the health path, then four 429s to the completions path with a retry-after around fifty-nine thousand seconds, roughly sixteen hours.

python-control-1064 is the clean illustration, because all three tools touched it and got three different stories from the same model:

    tomo-agent   22 rounds, 398K tokens, clock expired mid-fix   (engaged fail)
    opencode     4x HTTP 429, zero tokens                        (rate-limited)
    pi           23 requests in 151s, no landed edit             (engaged fail)

Same task, same model, three outcomes driven by timing and harness, not by reasoning.
Two concurrent streams against one free key spent the window fast, and everything downstream of that point recorded 429s that look like failures in the totals until you read the wire.

## Cost and leanness

Every task here cost zero dollars, because the tier is free; the meaningful axis is tokens, and the two harnesses have very different appetites.
OpenCode resends a growing context each turn and leans on prompt caching: smolagents alone sent 565K prompt tokens, dynaconf 269K, with large cached fractions.
tomo-agent's agent engine is leaner on the easy tasks, 57K in on the gitingest pass and 39K on briefcase, but it too balloons on the hard ones where it circles without landing, 398K on python-control.
The pattern: both stay lean when they solve quickly and both spend heavily when they get stuck, but OpenCode's floor is higher because of full-context resends, and its cache hit rate is what keeps that affordable in wall terms.

## What to do differently next time

The comparison is real for tomo-agent versus OpenCode and unmeasured for pi, and the fix for all of it is the same.
A fair multi-tool board on a single free key cannot be three back-to-back sweeps, because the third tool starves and the second gets rate-limited mid-run.
Give each tool its own key, or run one tool per quota window across separate days, or move to a paid tier where the window is not the binding constraint.
And keep the two harness rules this run established: drive Laguna through the agent engine because its text tool-call dialect is invisible to oi, and match the per-task timeout across the container tools and the probe so a slow endpoint does not fail one harness at eight minutes while another gets fifteen.

## Bottom line

Laguna-S-2.1 on the free zen tier is a real but modest solver on swebench-live: a clean 2 of 14 through our own agent engine, a matching 2 of 8 through OpenCode on the tasks that got model time, and a trade of wins between the two, tomo-agent on briefcase and OpenCode on smolagents, with gitingest shared.
Its one visible edge over the small free models is briefcase, a precise behavioral fix it both writes and defends.
Everything else on the board was governed less by the tool than by the free tier: a slow endpoint that eats the clock, and a rate-limit window that turns whole runs into zero-token rows.
The model is worth a fair board; the free key is not the way to get one.

Metrics: tomo-agent 2/14 gradeable, OpenCode 2/8 model-engaged (4 rate-limited to zero, 3 unrun), pi 2/15 engaged and incomplete; per-harness win trade on the same model (briefcase to tomo-agent, smolagents to OpenCode, gitingest shared); all four OpenCode zero-token rows verified as HTTP 429 with ~16h retry-after; Laguna-S free on zen and cloud-only, Laguna-XS Q4 local on the RTX 4090.
