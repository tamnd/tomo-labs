---
title: "Six million tokens for three solves: nemotron-3-ultra-free never stops, and it barely helps"
linkTitle: "nemotron-3-ultra-free, fair board"
description: "nemotron-3-ultra-free is the third free zen model on the abort-aware oi harness across all fifteen swebench-live tasks. It scores the same 3 of 15 as the other two free models, and it does it the hard way: it hits the thirty-round ceiling on every single task, spends 5.98 million tokens, and still lands only three. It solves faker-2142, which the other two abandoned in a round or two, and it fails fonttools-3682, the trivial pasted-patch task, after thirty rounds without writing a single edit. It edits the gold file on five tasks and only three of those pass. This is the pathological end of the persistence axis: refusing to quit is not the same as knowing how to finish."
date: 2026-07-20T00:45:00+07:00
---

Reproducibility header: tool=tomo, engine=oi (code-as-action), model=opencode/nemotron-3-ultra-free (free tier via the opencode.ai/zen upstream), suite=swebench-live, tasks=all fifteen.
Reproduce command, per task:

    source ~/data/.local.env   # OPENCODE_API_KEY for the zen upstream
    scripts/campaign_sweep.sh <task-id> \
      --models "opencode/nemotron-3-ultra-free" \
      --max-rounds 30 --timeout 600s --retries 2

This is the third free zen model on the fair board, and it completes a clean progression.
deepseek-v4-flash-free quits early and cheaply for its 3 of 15 (624K tokens).
mimo-v2.5-free grinds harder for the same 3 of 15 (2.07M tokens).
nemotron-3-ultra-free grinds hardest of all, to the point of never once converging before the round limit, and still gets 3 of 15, for 5.98M tokens.

## The board

Fifteen tasks, one attempt each after aborts are retried out, graded by each task's hidden check.sh.
The rounds column is the story: every task, without exception, ran to the thirty-round ceiling.

    task                              verdict  rounds  actions  in_tok   out_tok  secs    edited
    beeware__briefcase-2085           fail      30      30     336326    7761    240.3    no edit
    conan-io__conan-17123             PASS      31      30     268311   10241    195.2    gold
    aws-cloudformation__cfn-lint-3798 fail      30      30     312471    7607    241.9    wrong file
    cyclotruc__gitingest-94           PASS      30      29     287954    5699    143.0    gold
    dynaconf__dynaconf-1225           fail      30      30     368535    4349    239.8    no edit
    fonttools__fonttools-3682         fail      30      30     466783    3951    131.8    no edit
    huggingface__smolagents-285       fail      30      30     382428    9032    247.4    gold, wrong fix
    instructlab__instructlab-2540     fail      30      30     444665    3554     92.1    no edit
    joke2k__faker-2142                PASS      30      30     389316   10763    197.6    gold
    kubernetes-client__python-2303    fail      30      30     406613    5335    146.2    no edit
    projectmesa__mesa-2394            fail      30      30     563449    4796    130.0    gold, wrong fix
    python-control__python-control-1064 fail    32      30     443841    6734    248.7    no edit
    reata__sqllineage-661             fail      30      30     491900    8242    194.1    no edit
    sphinx-doc__sphinx-12975          fail      30      30     194368    3347    148.8    no edit
    stanfordnlp__dspy-1651            fail      30      30     530352    5464    106.8    no edit

Solved 3 of 15.
Total tokens 5,984,187, of which 5,887,312 are input and only 96,875 are output.
That input-to-output ratio, roughly 60 to 1, is the shape of a model that re-reads a growing context every round and emits very little, thirty times over, on every task.

## The three passes

    task              edited file (== gold)                               rounds  actions
    conan-17123       conan/internal/api/config/config_installer.py         31      30
    gitingest-94      src/gitingest/parse_query.py                          30      29
    faker-2142        faker/providers/bank/nl_BE/__init__.py                30      30

conan and gitingest are two of the well-localized tasks, shared with at least one other free model, and nemotron does hit the exact gold file on both.

faker-2142 is the one nemotron owns.
deepseek abandoned faker in three rounds with one action; mimo abandoned it in three rounds with one action.
Both read the task, saw a bank-provider data file, and walked away.
nemotron ground thirty rounds into `faker/providers/bank/nl_BE/__init__.py`, the gold file, and turned the test green.
So the pathological persistence does buy something the quitters cannot: a task that needs someone to actually sit down and edit a fiddly data file.

## The anomaly: fonttools-3682, thirty rounds and no edit

fonttools-3682 is the most damning cell on the board.

It is the easiest task on the suite, the one whose issue report pastes a working patch, and both other free models cleared it by transcribing that patch into `Lib/fontTools/ttLib/reorderGlyphs.py`.
nemotron spent thirty rounds and 466,783 tokens on it and never wrote a single edit.
It read and re-read and explored and hit the ceiling with an untouched tree.
A model that grinds every task to the wall still failed the one task that asks only for transcription, because grinding is not the same as committing an edit, and nemotron's loop kept choosing to look rather than to write.

## Edited the gold file and still failed: smolagents and mesa

Two more tasks show the ceiling from the other side.

On smolagents-285 nemotron edited `src/smolagents/local_python_executor.py`, the gold file, and failed the hidden test.
On mesa-2394 it edited `mesa/model.py`, the gold file, and failed.
So nemotron located the right file on five tasks (the three passes plus these two) and converted only three.
mimo solved smolagents by editing the same gold file; nemotron reached the same file and could not land a fix that satisfied the tests within thirty rounds.
Finding the spot was never the bottleneck here. Writing the correct change was, and more rounds did not fix that.

## Three free models, same score, three different threes

The full picture across all three fair boards:

    task               deepseek-v4-flash   mimo-v2.5   nemotron-3-ultra
    conan-17123        PASS                PASS        PASS
    gitingest-94       PASS                fail        PASS
    fonttools-3682     PASS                PASS        fail (no edit)
    smolagents-285     fail                PASS        fail (gold, wrong fix)
    faker-2142         fail                fail        PASS
    everything else    fail                fail        fail
    board              3/15                3/15        3/15
    tokens             624,323             2,070,985   5,984,187

conan-17123 is the only task all three solve.
Past that, each free model contributes a different third and fourth-and-fifth solvable task, so the union of what these three free models can solve is five tasks, not three.
The token axis is the punchline: for an identical final score, the three models span a 10x range in tokens spent, and the ordering is monotonic in stubbornness, deepseek quits, mimo grinds, nemotron never stops.

nemotron is the cautionary end of that axis.
Its refusal to converge earns exactly one solve the quitters miss (faker) and costs it one solve the finishers get (fonttools, dropped with no edit despite thirty rounds), for a net wash on score and a 10x bill on tokens.
On a priced tier nemotron would be the most expensive way to reach 3 of 15 on this suite.
On the free tier it is the slowest, and its only real edge is on a task where the fix is tedious rather than clever, which is a narrow niche to pay six million tokens for.

All three hit the same six-task hard wall the wider campaign mapped (cfn-lint, dynaconf, sqllineage, instructlab, sphinx, python-control), and a gpt-5.6 codex sweep fails the same six.
No amount of grinding moved that wall.

Metrics: solved 3/15, total 5,984,187 tokens (input 5,887,312, output 96,875), cost $0 on the free tier, engine oi (code-as-action, graded by hidden check.sh), aborts retried up to two times per task, every task ran to the thirty-round ceiling, gold file edited on five tasks with three passing.
