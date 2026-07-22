---
title: "Laguna-S-2.1 through OpenCode: two passes, one of them a task tomo could not grind out, and four runs killed by a 429 wall"
linkTitle: "opencode + laguna-s, partial board"
description: "Second of the three per-tool boards on laguna-s-2.1-free. OpenCode, the containerized coding agent, runs the same swebench-live tasks through the same free zen endpoint. It clears two, gitingest-94 and smolagents-285, and smolagents is the interesting one: it is a task our own agent engine could not land inside the clock, and OpenCode gets it by grinding forty-seven requests with heavy prompt caching. The board is partial at twelve of fifteen because the run was stopped for time, and four of those twelve did not fail on the model at all: the free zen tier returned HTTP 429 with a sixteen-hour retry-after on every completion, so those tasks recorded zero tokens. This writes up the two passes from the traces and shows the 429 wall directly from the proxy log."
date: 2026-07-22T13:15:00+07:00
---

Reproducibility header: tool=opencode, model=laguna-s-2.1-free (free tier via the opencode.ai/zen upstream), suite=swebench-live, per-task attempt timeout 15m, one attempt per task.
Reproduce command, per task:

    source ~/data/.local.env   # OPENCODE_API_KEY for the zen upstream
    LAB_MODEL=laguna-s-2.1-free ./bin/lab run opencode <task-id> --suite swebench-live

This is the containerized counterpart to the tomo-agent board written up alongside it.
Same model, same fifteen tasks, same hidden graders.
The difference is the harness: OpenCode runs in its own container with its own agent loop, and its LLM traffic is routed through the lab's trace proxy, which is what lets us read the 429 wall below straight from the wire.

## The board

Twelve of fifteen tasks ran before the sweep was stopped for time; sqllineage-661, sphinx-12975, and dspy-1651 did not run.
Of the twelve, four recorded zero tokens because the free tier rate-limited every request, and those are marked 429 rather than fail, because the model never spoke.

    task                              verdict  reqs  in_tok   out_tok  secs
    cyclotruc__gitingest-94           PASS      12   105496    2001    832
    huggingface__smolagents-285       PASS      47   565427   13333    900
    aws-cloudformation__cfn-lint-3798 fail      10    84808    2686    900
    beeware__briefcase-2085           fail       9    96386    5476    900
    conan-io__conan-17123             fail      13    58060    1091    900
    dynaconf__dynaconf-1225           fail      14   269630    1423    900
    fonttools__fonttools-3682         fail       8    40025     186    487
    instructlab__instructlab-2540     fail      42   217542    2066    900
    joke2k__faker-2142                429        5        0       0    900
    kubernetes-client__python-2303    429        5        0       0    900
    projectmesa__mesa-2394            429        5        0       0    900
    python-control__python-control-1064 429      5        0       0    900

Solved 2 of the 8 tasks that actually got model time.
The token counts are large because OpenCode resends a growing context each turn; smolagents alone sent 565K prompt tokens, of which 408K were served from cache.

## The two passes

gitingest-94 is the shared easy win: OpenCode localizes the same `http://` prefix bug in parse_query.py and lands it, twelve requests, under fourteen minutes.
Both tools clear this one, which is the expected floor for a capable model on a well-localized task.

smolagents-285 is the pass that separates OpenCode from our agent engine on this model.
On the tomo-agent board this task is a capped failure: nine rounds, `1 failed, 65 passed`, one fail_to_pass test short when the clock ran out.
OpenCode gets it, and the trace shows how: forty-seven requests, 565K prompt tokens with 408K of them cached, thirteen thousand completion tokens, a full fifteen minutes of grinding.
The hidden check comes back `PASS: fail_to_pass green, in-file pass_to_pass stable`.
The lesson is not that OpenCode is smarter here; it is that OpenCode is willing to keep going.
Laguna-S needs a lot of rounds to close smolagents, and the harness that lets it take forty-seven of them lands the task while the harness that gave it nine did not.
This is a per-harness result on the same model, and it is the single clearest reason to run more than one harness in a fair board.

## The 429 wall, from the proxy

The four zero-token rows are not model failures.
The trace proxy logs every upstream call with its status, and for faker-2142 the record is unambiguous:

    seq 1  GET  /zen/                      200
    seq 2  POST /zen/v1/chat/completions   429  retry_after_s=59206
    seq 3  POST /zen/v1/chat/completions   429  retry_after_s=59205
    seq 4  POST /zen/v1/chat/completions   429  retry_after_s=59204
    seq 5  POST /zen/v1/chat/completions   429  retry_after_s=59200

The health check to `/zen/` returns 200, and then every actual completion returns 429 with a retry-after of about fifty-nine thousand seconds, roughly sixteen hours.
kubernetes-2303, mesa-2394, and python-control-1064 have the identical signature: one 200 to the health path, four 429s to the completions path, zero tokens.
That is the free zen tier's quota wall.
The account had spent its free window, so the endpoint refused new completions for that window, and any task that happened to run inside it recorded nothing.
It is worth stating plainly because it is easy to misread: python-control-1064 shows as a zero-token row here, but on the tomo-agent board the same task ran twenty-two real rounds and 398K tokens before the clock beat it. The difference is not the model, it is whether the free quota was open at that minute.

The engineering consequence is the one the earlier free-board work already found: on a free tier the retry-and-abort accounting is load-bearing, and beyond that, two concurrent streams against one free account exhaust the window fast. This board's four 429s all landed after a second stream had been sharing the same key.

## The engaged failures

Set the passes and the 429s aside and six tasks got real model time and did not land.
cfn-lint-3798, briefcase-2085, conan-17123, dynaconf-1225, instructlab-2540, and fonttools-3682 all ran to or near the fifteen-minute cap with real token traffic and no green check.
instructlab is the heaviest engaged miss at forty-two requests and 217K tokens, dynaconf the most token-hungry at 269K, and both ended red.
briefcase is the notable one: OpenCode fails the exact task tomo-agent passes.
Where our agent engine wrote the `insteadOf` fix and held it against a failing visible suite, OpenCode spent nine requests and 96K tokens on the same file and did not converge on the change that satisfies the hidden tests.
So the two harnesses trade wins on this model: OpenCode takes smolagents, tomo-agent takes briefcase, and both take gitingest.

## What this set says

OpenCode on laguna-s-2.1-free is 2 of 8 on the tasks that got a fair shot, with smolagents as its signature pass, a task it wins purely by grinding more rounds than our agent engine gave the model.
The board is partial and four rows are a free-tier quota artifact rather than a model result, which is the headline caveat: on the free zen tier, a fair board needs the quota window open and preferably one stream per key, or a real result and a 429 look the same in the totals until you read the proxy.

Metrics: 2 passes of 8 model-engaged tasks (12 of 15 run, 4 of those rate-limited to zero tokens), smolagents landed in 47 requests with 408K cached prompt tokens, all four zero-token rows verified as HTTP 429 with a ~16h retry-after in the trace proxy, graded by hidden check.sh.
