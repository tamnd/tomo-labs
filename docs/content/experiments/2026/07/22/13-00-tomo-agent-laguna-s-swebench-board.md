---
title: "Laguna-S-2.1 on our own engine: two of fifteen, and the free tier sets the ceiling"
linkTitle: "tomo-agent + laguna-s, fair board"
description: "Poolside shipped Laguna-S-2.1, a 118B mixture-of-experts coder, and opencode.ai/zen serves it free as laguna-s-2.1-free. This runs the whole fifteen-task swebench-live board against it through tomo's own agent engine, the one that drives native structured tool_calls. Laguna solves two tasks clean, gitingest-94 and briefcase-2085, both with the edit landed in the exact gold file. The other twelve split into one fast mislocalization and a large cluster that ran the full fifteen-minute clock without landing a fix, because the free endpoint answers at roughly fifty seconds a request and the model never gets enough rounds. This is the first of three per-tool boards on the same model; pi and opencode follow, then the cross-tool comparison."
date: 2026-07-22T13:00:00+07:00
---

Reproducibility header: tool=tomo, engine=agent (native structured tool_calls), model=opencode/laguna-s-2.1-free (free tier via the opencode.ai/zen upstream), suite=swebench-live, tasks=all fifteen, per-task timeout 15m to match the container harness.
Reproduce command, per task:

    source ~/data/.local.env   # OPENCODE_API_KEY for the zen upstream
    ./bin/lab probe <task-id> --engine agent \
      --model laguna-s-2.1-free --grade --timeout 15m --out out/<task-id>

## Setup and feasibility

The original question was whether we can run the two Laguna checkpoints Poolside just published, and whether the free zen account reaches them.

Laguna-S-2.1 is 118B total parameters, roughly 8B active per token as a mixture-of-experts, with a 1M context window.
Its GGUF does not fit the gamingpc's RTX 4090 at any useful quant, so local is out for the S size.
It is, however, live on zen as laguna-s-2.1-free at zero cost, verified against the account, which is what this board runs against.

Laguna-XS-2.1 is the small sibling, 33B total and about 3B active.
Its Q4_K_M GGUF is 20.3GB and fits the 4090 with room for a 32K context, and ollama 0.32.1 has native support for the architecture.
It pulls and runs at 100 percent GPU offload, and as a smoke test it solved gitingest-94 locally through the same probe path, twenty-one rounds, edit in the gold file.
So the feasibility answer is: S is free on zen and too big to host locally, XS is hostable locally on the 4090 and also solves the easy localized tasks.

One harness finding matters before the board.
Laguna emits tool calls in two dialects: the native OpenAI structured tool_calls array, and a text form, `<tool_call>name<arg_key>K</arg_key><arg_value>V</arg_value></tool_call>`.
The oi engine, which sends no tools and parses per-model text dialects, does not recognize that text form and comes back with zero actions.
The agent engine, which offers native tools and reads the structured array, drives it correctly.
So the per-model harness rule for Laguna is: use the agent engine, not oi.
This board is the agent engine.

## The board

Fifteen tasks, one attempt each, graded by each task's hidden check.sh.
cfn-lint-3798 is marked N/A: its setup step fails in the probe path with an unreadable git tree, before the model is ever called, so it is a harness limitation on this path and not a model result. The containerized boards do run it.

    task                              verdict  rounds  in_tok   out_tok  secs
    cyclotruc__gitingest-94           PASS       9     57044     1643    162
    reata__sqllineage-661             fail       3     10508      480     85
    aws-cloudformation__cfn-lint-3798 N/A        -         -        -      -
    beeware__briefcase-2085           PASS       8     39013     2149    480
    conan-io__conan-17123             fail       5     19542    13954    900
    dynaconf__dynaconf-1225           fail      12    214867     2092    900
    fonttools__fonttools-3682         fail      10     68365     2884    900
    huggingface__smolagents-285       fail       9     35387     2256    900
    instructlab__instructlab-2540     fail      11     84523     1781    900
    joke2k__faker-2142                fail      13     96168     6417    429
    kubernetes-client__python-2303    fail       8     39234      953    230
    projectmesa__mesa-2394            fail      10     45030      928    162
    python-control__python-control-1064 fail    22    398537     5009    900
    sphinx-doc__sphinx-12975          fail       5     18439     1673    900
    stanfordnlp__dspy-1651            fail       9     53483     1806    900

Solved 2 of 14 gradeable.
Total tokens across the fourteen 1,224,165, of which 1,180,140 are input.
Eight of the fourteen ran the full fifteen-minute clock and were cut off there.

## The two passes, from the traces

Both passes edited exactly the file the gold patch edits and nothing else.

    task           edited file (== gold)             rounds  secs
    gitingest-94   src/gitingest/parse_query.py         9     162
    briefcase-2085 src/briefcase/commands/base.py       8     480

gitingest-94 is the clean localization.
The trace is four moves and a fix: grep for `https://` scoped to parse_query.py, read the function, grep the callers, read the test file, and then the model states the bug in one line, that a URL starting with `http://` gets `https://` blindly prepended and becomes `https://http://...`.
It writes the exact three-line guard, replaces the prefix instead of prepending it, verifies with a python one-liner across `http://`, `https://` and bare URLs, then runs the visible suite and gets 17 of 17 green.
This is the shape the free tier as a whole clears: the issue names the file, the fix is small and contiguous, and the model reads, edits, and verifies without wandering.

briefcase-2085 is the more telling pass, because it turns on a piece of reasoning rather than a piece of localization.
The model greps `set_url`, lands on base.py:1017, reads the surrounding block, and correctly diagnoses that with a git `insteadOf` config rule `remote.url` returns the rewritten URL, so `git remote set-url` with an `old_url` argument fails because that URL no longer exists.
It removes the `old_url` argument and wraps the call in a try/except that logs a warning.
Then it runs the visible tests and they fail, because those tests still assert the old `old_url` behavior.
The model does not flinch: it reasons out loud that it must not edit the tests and that the hidden suite grades the new behavior, keeps its change, confirms the module still imports, and ends.
That is the one place on this board where the 118B model shows a real edge over the small free models: deepseek-v4-flash-free finds this same file and cannot write the fix, and Laguna-S both writes it and holds onto it against a failing visible suite.
It timed out at the eight-minute cap on that first run and passed anyway, because the edit was already correct.

## The clock is the story, not the ceiling

The single most important axis on this board is wall time, because the free endpoint is slow.
laguna-s-2.1-free answers at roughly fifty seconds per request under a light concurrent load, sometimes faster, often not.
At that latency a fifteen-minute budget buys about nine to twelve model rounds, and eight of the fourteen tasks hit that wall.

This is a real and deliberate correction to a first pass at this board.
The first run gave the probe an eight-minute timeout while the container harness gives every tool fifteen, and under eight minutes four tasks died at exactly 480 seconds with four or five rounds on the clock.
Those were timeout artifacts, not model failures, so the board was rerun at a matched fifteen-minute cap.
At fifteen minutes those same tasks run to nine, ten, twelve rounds and still fail, but now they fail on their merits and the number is fair against the container tools.
The clearest example is python-control-1064: twenty-two rounds, 398K input tokens, the full clock, and still no landing fix.
smolagents-285 is the closest of the capped failures, ending `1 failed, 65 passed`, one fail_to_pass test short.

## The other failures, in two shapes

Set the eight capped runs aside and the remaining failures fall into the two familiar buckets.

Mislocalized, fast.
sqllineage-661 is the sharp case, and the trace is brutally short: two tool calls total.
It reads forty lines of `sqllineage/runner.py`, edits the sort key inside `get_column_lineage` on the first plausible spot, declares the ordering deterministic, and stops at eighty-five seconds without ever running a test.
runner.py is not the gold file and the guessed sort change is not the fix, so the hidden run came back `1 failed, 18 passed`.
This is the model's worst habit on this suite: a snap edit on the first thing that looks related, with no reproduce and no check.
mesa-2394 edited `mesa/model.py` and also missed, in ten rounds under three minutes.

Ran the clock without a captured gold-file edit.
The eight capped tasks plus faker and kubernetes ran real read and grep and edit rounds and ended without an edit recorded in the gold file.
python-control-1064 is the honest version of this shape.
Its trace shows genuine engagement: the model greps for `zpk`, follows it to `zpk2tf`, writes a reproduce script, and confirms from the output that the zpk-built system has an exploding impulse response while the equivalent tf-built system is stable.
It had the bug reproduced and understood.
But each round re-sends the growing transcript, the input token count climbed to 398K, and at roughly fifty seconds a request the fifteen-minute clock expired while it was still circling a fix.
Twenty-two rounds, no committed edit, `edited_files: null`.
This is the finding for most of the board: on laguna-s-2.1-free the free-tier latency, not the model's reasoning, is the binding constraint, and a task the model can reproduce and explain is one it can still run out of clock on before it writes the patch.

## What this set says

Laguna-S-2.1 through our own agent engine is a clean 2 of 14 on swebench-live, and both solves are the exact-gold-file localized tasks.
Its one genuine edge over the small free models is briefcase, a precise behavioral fix on the right file that the weaker free tier misses.
Everything below that is governed by the free endpoint's latency: eight of fourteen tasks never got the rounds they needed inside a fair fifteen-minute cap.

The harness takeaway is the dialect rule.
Laguna must be driven through the agent engine, because its text tool-call dialect is invisible to oi, and a board run on the wrong engine would read as a flat zero rather than a real 2 of 14.

Metrics: solved 2/14 gradeable (cfn-lint N/A on this path), 1,224,165 total tokens, engine agent (native structured tool_calls, graded by hidden check.sh), per-task cap 15m to match the container harness, eight tasks cut off at the cap, both passes edited the exact gold file.
