---
title: "The MoE that the code-as-action path underserves: qwen3-30b-a3b on swebench-live through the box gateway"
linkTitle: "qwen3-30b-a3b, local board"
description: "Second model on the local roster board. qwen3-30b-a3b is a general MoE, not a coder tune, and it runs on the RTX 4090 behind the llmgw gateway driven through the uniform oi code-as-action harness. It scores 1 of 15. Three of the fifteen were not real attempts: the local gateway returned an empty completion late in the board and the harness recorded a zero-token no-op as a fail, which this run turned into a one-line harness fix. On the twelve tasks it genuinely attempted it still lands only gitingest, and the shape it draws is the mirror image of the coder tune published just before it."
date: 2026-07-20T01:30:00+07:00
---

Reproducibility header: tool=tomo, engine=oi (code-as-action), model=qwen3-30b-a3b served by ollama on the RTX 4090 box behind the llmgw gateway, driven over tailnet, suite=swebench-live, tasks=all fifteen.
Reproduce command, per task:

    OPENCODE_API_KEY=<gateway data token> \
    scripts/campaign_sweep.sh <task-id> \
      --models "qwen3-30b-a3b" \
      --base-url http://100.71.238.128:8888/v1 \
      --max-rounds 30 --timeout 600s --retries 2

This is the second entry on the local board and the first one that caught a harness bug in the act.

## A harness bug, found and fixed mid-board

Three tasks on this board, the last three in run order, came back identical: `input_tokens=0, output_tokens=0, rounds=1, error=null`, exactly 60 seconds, graded fail.
Zero tokens means the model never actually answered.
What happened is a local-gateway eviction: partway through a long single-model board the 4090 came under memory pressure, the gateway returned an empty completion, and the oi probe finished after one round with nothing to do.
The task's hidden check.sh then ran against the untouched repository and recorded the repo's baseline test failure, so a pure infrastructure abort was miscounted as a capability fail on sqllineage-661, sphinx-12975, and dspy-1651.

That is a harness defect, not a model result, so it was fixed rather than reported around.
The abort-aware sweep already retries provider 429/400 and no-route errors; it now also treats a not-passed run with zero input and zero output tokens as a retryable abort, the same class as a provider error, while still recording genuine timeouts once.
The fix shipped before the rest of the roster ran, so later models do not inherit these phantom fails.
For honesty this board is reported as it was recorded, with the three aborts marked, which makes the real denominator twelve attempted tasks, not fifteen.

## The board

Fifteen tasks, graded by each task's hidden check.sh.
Cost is not priced for a local model; the resource signal is tokens and seconds on one 4090.

    task                              verdict  rounds  actions  secs    gold file reached?
    beeware__briefcase-2085           fail      27      25     600.0    no (600s timeout)
    conan-io__conan-17123             fail      18      16     176.1    no
    aws-cloudformation__cfn-lint-3798 fail      23      21     340.5    no
    cyclotruc__gitingest-94           PASS      21      25     206.6    yes
    dynaconf__dynaconf-1225           fail      24      22     427.7    no
    fonttools__fonttools-3682         fail       6       5     103.0    edited gold (ttFont.py), test errored
    huggingface__smolagents-285       fail      11      10     134.0    yes (local_python_executor.py)
    instructlab__instructlab-2540     fail      19      18     256.4    edited gold (chat.py + defaults.py)
    joke2k__faker-2142                fail      19      17     374.4    no
    kubernetes-client__python-2303    fail      30      29     517.3    no
    projectmesa__mesa-2394            fail       4       3      47.3    yes (model.py)
    python-control__python-control-1064 fail    23      21     600.0    no (600s timeout)
    reata__sqllineage-661             abort      1       0      60.2    zero-token gateway abort
    sphinx-doc__sphinx-12975          abort      1       0      60.2    zero-token gateway abort
    stanfordnlp__dspy-1651            abort      1       0      60.2    zero-token gateway abort

Solved 1 of 15 (1 of 12 attempted).
Total tokens 1,367,052 (input 711,331, output 655,721).
The output share is unusually high, near half the total: this MoE talks as much as it reads.

## The one pass

gitingest-94, the easiest task on the suite, the one whose report hands over the file and symbols.
qwen3-30b-a3b edited `src/gitingest/parse_query.py`, the gold file, and turned the hidden test green.
It is the single task every capable model on this suite clears, free or local, and this MoE clears it too.

## The mirror image of the coder tune

The entry published just before this one, qwen3-coder-30b-a3b, reached five gold files and converted one.
qwen3-30b-a3b, the general MoE at the same size, reaches gold on three tasks (gitingest, smolagents, mesa) and edits the gold file on two more (fonttools, instructlab), and also converts exactly one.
So the two 30B siblings land the same single task from opposite habits: the coder navigates aggressively and edits everything, the general model localizes less often and writes less.
Neither composes the passing change on anything past the most localized task.
On smolagents and mesa this model put an edit into the exact gold file and still failed the hidden test, the same reach-but-miss pattern the coder showed, which says localization is not the wall for either sibling; composing the fix is.

## The harness-path contrast

An earlier run of this same model through the native OpenAI function-calling path scored higher on this suite than the 1 of 15 it scores here through the oi code-as-action path.
The two paths are not the same experiment: native tool-calling hands the model typed edit and shell tools, while oi asks it to emit actions as fenced code that the harness parses and runs.
The takeaway is not a single number but a direction: qwen3-30b-a3b is a model the code-as-action dialect underserves relative to native tools, the opposite of the coder tunes that emit their calls as `<function=...>` text the native path silently drops and therefore need oi to be scored fairly at all.
This is why the roster is run through one uniform harness and the path is stated plainly: the harness is a variable, and for this model it is a costly one.

## Where this sits

One pass is a low number and it is honest.
The three zero-token rows are not model failures and are not counted as attempts; the two 600-second rows are real timeouts on a slow single-GPU model, not aborts; the nine remaining fails are genuine, five of them after the model reached or edited the right file.
qwen3-30b-a3b on this suite is a competent localizer and a weak finisher whose score is further depressed by being run through the dialect it handles worst.
As the second local-board entry it sets up the reading for the rest of the roster: the coder tunes edit hard and need oi, the general models localize and lose less legibly, and both siblings at 30B land the same one easy task.

Metrics: solved 1/15 (1/12 attempted), total 1,367,052 tokens (input 711,331, output 655,721), engine oi over the box gateway (code-as-action, graded by hidden check.sh), three zero-token gateway aborts (sqllineage, sphinx, dspy) excluded as non-attempts and fixed in the sweep, two tasks (briefcase, python-control) hit the 600s timeout, gold reached or edited on five tasks with one converting.
