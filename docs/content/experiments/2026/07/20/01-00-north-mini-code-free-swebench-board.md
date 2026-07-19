---
title: "An honest zero: north-mini-code-free reads the whole suite and never writes a line"
linkTitle: "north-mini-code-free, fair board"
description: "north-mini-code-free is the fourth and last free zen model on the abort-aware oi harness across all fifteen swebench-live tasks, and it is the one that breaks the pattern. It scores 0 of 15. A third of the board dies on a persistent upstream 400 that survives the retries, which is the free-tier infrastructure failure the fair protocol was built to catch. But the other two thirds is not infrastructure: on every task that ran clean, north-mini emitted actions in a structured JSON dialect that the harness parsed and executed fine, and it never once wrote a source edit. It inspects, greps, and prints for twenty-seven rounds and leaves the tree untouched. This documents why the zero is real, and why abort-retries could not rescue it."
date: 2026-07-20T01:00:00+07:00
---

Reproducibility header: tool=tomo, engine=oi (code-as-action), model=opencode/north-mini-code-free (free tier via the opencode.ai/zen upstream), suite=swebench-live, tasks=all fifteen.
Reproduce command, per task:

    source ~/data/.local.env   # OPENCODE_API_KEY for the zen upstream
    scripts/campaign_sweep.sh <task-id> \
      --models "opencode/north-mini-code-free" \
      --max-rounds 30 --timeout 600s --retries 2

This is the fourth free zen model and the one that does not land 3 of 15.
The premise going in, from the earlier campaign, was that all four weak free models had been unfairly scored: their runs were dominated by free-tier infra aborts, and with the abort-aware harness retrying those aborts the real number should surface.
For deepseek-v4-flash-free, mimo-v2.5-free, and nemotron-3-ultra-free that premise held, all three came out at a real 3 of 15.
north-mini-code-free is where the premise only half holds, and the honest result is a zero.

## The board

Fifteen tasks, one attempt each after aborts are retried out, graded by each task's hidden check.sh.
Two columns matter here: the error column, which flags tasks killed by a provider 400, and the fact that every single row's edited-file set is empty.

    task                              verdict  rounds  actions  in_tok   out_tok  secs    outcome
    beeware__briefcase-2085           fail       1       0      2464      22      1.5     instant non-start
    conan-io__conan-17123             fail       7       2      5131     313      7.4     provider 400
    aws-cloudformation__cfn-lint-3798 fail       8       3      7021     462     11.5     provider 400
    cyclotruc__gitingest-94           fail      13       8     49823    4265     38.4     provider 400
    dynaconf__dynaconf-1225           fail      30      30    211618   16325    149.5     explored, no edit
    fonttools__fonttools-3682         fail       1       0      1401     129      1.5     instant non-start
    huggingface__smolagents-285       fail       1       0      1320      13      1.0     instant non-start
    instructlab__instructlab-2540     fail      13       8     24201    1427     18.2     provider 400
    joke2k__faker-2142                fail       8       6     37619    3897     33.1     explored, no edit
    kubernetes-client__python-2303    fail      27      57    244710   26231    269.7     explored, no edit
    projectmesa__mesa-2394            fail       1       0      1082     209      1.9     instant non-start
    python-control__python-control-1064 fail     1       0      1349      28      1.3     instant non-start
    reata__sqllineage-661             fail      15      13    136181    9901    101.6     explored, no edit
    sphinx-doc__sphinx-12975          fail      19      14     57669    6444     60.1     provider 400
    stanfordnlp__dspy-1651            fail       3       1      7156     824      7.4     explored, no edit

Solved 0 of 15.
Total tokens 859,235.
edited_files is null on all fifteen. north-mini did not write a source edit anywhere on the suite.

## The infrastructure third: five persistent 400s

Five tasks (conan, cfn-lint, gitingest, instructlab, sphinx) died mid-run on the same error:

    openai: 400 Bad Request: {"error":{"message":"Error from provider (Console): Upstream request failed", ...}}

This is exactly the free-tier failure the abort-aware harness exists to absorb, and on the other three free models it did absorb the analogous aborts.
The difference here is that north-mini's 400s are persistent, not transient.
The harness retried each of these tasks up to twice, and the 400 came back every time, so what survives into the board is a recorded failure rather than a clean retry.
Note these were not instant: gitingest ran thirteen rounds and eight actions before the upstream cut it off, sphinx nineteen rounds.
The model was working the task when the provider dropped the request, repeatedly.
So a third of the board is a genuine free-tier infrastructure loss that no reasonable retry budget cleared.

## The behavioral two-thirds: it never writes

The other ten tasks are not an infrastructure story, and this is the part abort-retries cannot fix.

Five of them are instant non-starts: briefcase, fonttools, smolagents, mesa, python-control each ran a single round, zero actions, a handful of output tokens, and quit in under two seconds.
The model read the prompt and produced no first action at all.

The other five ran, some of them hard.
kubernetes-2303 went twenty-seven rounds and fifty-seven actions across 269 seconds.
dynaconf went the full thirty rounds and thirty actions.
sqllineage went fifteen rounds and thirteen actions.
And every one of them ended with an untouched tree.

I checked this directly rather than trusting the summary alone.
north-mini emits its actions not as markdown code fences but as a structured JSON envelope, one object per turn of the form `{"contents":[{"language":"sh","text":"ls -la"},{"language":"python","text":"..."}]}`.
The oi harness parsed and executed these fine: the fifty-seven counted actions on kubernetes are real commands that ran, so this is not a dialect the parser silently dropped.
But reading the actual command texts, they are all inspection: `ls`, `grep`, `python` one-liners that print, redirects into `2>/dev/null` inside search pipelines.
There is not a single `cat >`, `sed -i`, `apply_patch`, or file-open-for-write to a source path in any of the fifteen transcripts.
north-mini-code-free, under the code-as-action harness, is a model that inspects and never commits a change.

## Why the zero is real and not a harness artifact

The original goal of this whole free-model pass was to make sure tomo harnesses each model correctly, so a capable model is not scored zero because the harness dropped its output.
north-mini is the case that most looked like it might be a harness false-fail, because it uses a non-standard JSON action dialect and it scores zero.
It is not a false-fail.
The dialect is parsed, the commands execute, the action counts are non-trivial (fifty-seven on one task), and the workspace diff is empty because the model issued no write commands, not because writes were lost.
The five 400s are real provider failures, reproduced across retries, visible in the error field.
Put together, north-mini-code-free is an honest 0 of 15: one third lost to persistent free-tier infrastructure, two thirds to a model that reads the codebase and never edits it.

## The four free models, end to end

    model                    board   tokens      shape
    deepseek-v4-flash-free   3/15      624,323   quits early, cheap
    mimo-v2.5-free           3/15    2,070,985   grinds, converges sometimes
    nemotron-3-ultra-free    3/15    5,984,187   grinds every task to the wall
    north-mini-code-free     0/15      859,235   inspects, never edits; a third lost to 400s

The fair protocol did its job in both directions.
For three of the four models it corrected an unfairly-zero record up to a real 3 of 15, proving the earlier failures were infrastructure.
For the fourth it confirmed that the zero was earned: the retries were applied, the dialect was parsed, and the model still solved nothing, because half its problem is a provider that keeps 400ing and the other half is a model that will not write an edit.
Reaching for north-mini-code-free on this suite buys inspection without action, on top of a flaky endpoint, which is the one free model of the four with no case for routing to it.

Metrics: solved 0/15, total 859,235 tokens, cost $0 on the free tier, engine oi (code-as-action, graded by hidden check.sh), aborts retried up to two times per task, five tasks lost to persistent provider 400s, edited_files null on all fifteen, JSON contents dialect parsed and executed (not a harness drop).
