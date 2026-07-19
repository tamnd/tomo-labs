---
title: "The local coder edits everything and lands one: qwen3-coder-30b-a3b on swebench-live through the box gateway"
linkTitle: "qwen3-coder-30b-a3b, local board"
description: "First model on the local roster board. qwen3-coder-30b-a3b runs on the RTX 4090 behind the llmgw gateway and is driven through the same uniform oi harness as the free zen models, over tailnet. It scores 1 of 15. The number undersells how hard it works: it is the most aggressive editor seen on this suite, 113 actions on a single task, and it reaches the exact gold file on five tasks. It just cannot land the fix on four of those five. This documents the local harness path, the one pass, the five gold-file misses, and why a coding-tuned local model can be simultaneously the busiest and one of the lowest-scoring."
date: 2026-07-20T01:15:00+07:00
---

Reproducibility header: tool=tomo, engine=oi (code-as-action), model=qwen3-coder-30b-a3b served by ollama on the RTX 4090 box behind the llmgw gateway, driven over tailnet, suite=swebench-live, tasks=all fifteen.
Reproduce command, per task:

    OPENCODE_API_KEY=<gateway data token> \
    scripts/campaign_sweep.sh <task-id> \
      --models "qwen3-coder-30b-a3b" \
      --base-url http://100.71.238.128:8888/v1 \
      --max-rounds 30 --timeout 600s --retries 2

This is the first entry on the local board, and it comes with a harness note.
The free zen models reached the abort-aware sweep over the public opencode.ai/zen upstream.
The local models reach the same sweep through the box gateway, which needed one small change: campaign_sweep.sh now takes a `--base-url`, so the identical harness points at `http://100.71.238.128:8888/v1` with the bearer coming from the gateway data token instead of the zen key.
Everything downstream, the oi engine, the thirty-round cap, the hidden check.sh grade, the abort retries, is unchanged.
A prior gate established that qwen3-coder-30b-a3b cannot use the native tool-calling path (it emits calls as `<function=...>` text that the native path silently drops), so oi is the correct and only fair harness for it.

## The board

Fifteen tasks, one attempt each after aborts are retried out, graded by each task's hidden check.sh.
Cost is not priced for a local model; the resource signal is tokens and seconds on one 4090.

    task                              verdict  rounds  actions  secs    gold file reached?
    beeware__briefcase-2085           fail      16      113    110.3    no
    conan-io__conan-17123             fail      23       78    140.1    no
    aws-cloudformation__cfn-lint-3798 fail      25       55     78.0    no
    cyclotruc__gitingest-94           PASS       4       31     29.9    yes
    dynaconf__dynaconf-1225           fail      18       34     60.8    no
    fonttools__fonttools-3682         fail      12       50     76.9    no
    huggingface__smolagents-285       fail       5        3      3.8    no
    instructlab__instructlab-2540     fail      16       84     92.1    yes (1 of 2, edited 4 files)
    joke2k__faker-2142                fail      27       65    172.7    no
    kubernetes-client__python-2303    fail      20       49    106.8    yes (both gold files)
    projectmesa__mesa-2394            fail      26       48     91.7    yes
    python-control__python-control-1064 fail     30       64    599.4    no (hit 600s timeout)
    reata__sqllineage-661             fail      12       47     54.2    no
    sphinx-doc__sphinx-12975          fail      22       44     65.5    yes
    stanfordnlp__dspy-1651            fail      30       68    104.8    no

Solved 1 of 15.
Total tokens 3,914,149 (input 3,750,299, output 163,850).
Note the action counts: this model does not explore lightly, it edits constantly, 113 actions on briefcase alone.

## The one pass

gitingest-94, the easiest task on the suite, the one whose report hands over the file and symbols.
qwen3-coder edited `src/gitingest/parse_query.py`, the gold file, in four rounds and turned the hidden test green.
It is the same task every capable free model also cleared, and the local coder clears it cleanly and fast.

## The five gold-file misses

This is the defining pattern of the run.
On five tasks qwen3-coder reached the exact file the gold patch edits, and converted only one of them:

    task              gold file(s) reached                                          result
    gitingest-94      src/gitingest/parse_query.py                                  PASS
    instructlab-2540  src/instructlab/configuration.py (1 of 2 gold, edited 4 files) fail
    kubernetes-2303   exec_provider.py + kube_config.py (both gold files)            fail
    mesa-2394         mesa/model.py                                                  fail
    sphinx-12975      sphinx/writers/html5.py                                        fail

Localization is clearly not the bottleneck for this model.
It found both gold files on kubernetes-2303, a two-file task, and still failed the hidden test.
On instructlab it touched one of the two gold files and spread four edits across the tree, and failed.
So the coder-tuned model is good at navigating to the right code and weak at composing the change that satisfies the tests, which is the exact inverse of the free non-coder models that often never reached the file at all.

## The cost of editing everything

qwen3-coder is the busiest model on the suite by a wide margin.
briefcase-2085: sixteen rounds but 113 actions, meaning it batched many edits and reads per round and still did not land the fix.
python-control-1064: thirty rounds, sixty-four actions, and it ran the full 599 seconds into the timeout wall, the only task that timed out.
For all that activity it spent 3.9 million tokens and produced one solve.
The token-per-solve here is far worse than any free model that also scored, because the free models that reached 3 of 15 did so with cheaper, more targeted trajectories, while the local coder grinds edits into files without converging on a passing diff.

## Where this sits

One pass is a low number, but the shape is informative and it is not a harness artifact.
The oi engine parsed and executed this model's text-format actions correctly (the action counts are real edits and reads that ran), the model reached gold files on a third of the board, and it graded green on the one task it actually fixed.
qwen3-coder-30b-a3b on this suite is a strong navigator and a weak finisher: it will find the file a task is about, and on all but the most localized transcription task it will not write the change that passes.
As the first local-board entry it sets the contrast the rest of the roster will be measured against, a coding-tuned local model that edits aggressively and lands rarely, versus the free non-coders that edit timidly and land the same one or two easy tasks.

Metrics: solved 1/15, total 3,914,149 tokens (input 3,750,299, output 163,850), engine oi over the box gateway (code-as-action, graded by hidden check.sh), aborts retried up to two times per task, gold file reached on five tasks with one converting, one task (python-control) hit the 600s timeout.
