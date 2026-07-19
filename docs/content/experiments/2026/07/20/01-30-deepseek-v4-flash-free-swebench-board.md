---
title: "Three of fifteen, and the aborts were the story: a fair swebench-live board for deepseek-v4-flash-free"
linkTitle: "deepseek-v4-flash-free, fair board"
description: "The earlier read on the free zen models was that deepseek-v4-flash-free never produced a clean multi-task pass. That read was an artifact of the free tier, not the model. Rerun across all fifteen swebench-live tasks with the abort-aware harness, which retries a 429 or a gateway 400 instead of scoring it as a task failure, deepseek-v4-flash-free lands three clean passes: conan-17123, gitingest-94, and fonttools-3682, each with the edit dropped into the exact gold file. The whole fifteen-task board costs nine cents. This writes up the board, the three passes, the one near-miss where it found the right file and wrote the wrong fix, and the two failure shapes that account for the other eleven."
date: 2026-07-20T01:30:00+07:00
---

Reproducibility header: tool=tomo, engine=oi (code-as-action), model=opencode/deepseek-v4-flash-free (free tier via the opencode.ai/zen upstream), suite=swebench-live, tasks=all fifteen.
Reproduce command, per task:

    source ~/data/.local.env   # OPENCODE_API_KEY for the zen upstream
    scripts/campaign_sweep.sh <task-id> \
      --models "opencode/deepseek-v4-flash-free" \
      --max-rounds 30 --timeout 600s --retries 2

The point of this run is a fair number.
An earlier pass over the five free zen models left deepseek-v4-flash-free looking unmeasurable: its runs kept dying on free-tier infrastructure, a 429 quota bounce or a gateway 400, and those deaths sat in the board next to real task failures with no way to tell them apart.
The abort-aware harness fixes exactly that.
A run that ends with a recorded transport error and did not pass is treated as an infra abort and retried up to a bound, so a flaky free endpoint costs a retry, not a false failure.
With that in place the model gets a clean fifteen-task board.

## The board

Fifteen tasks, one attempt each after aborts are retried out, graded by each task's hidden check.sh.

    task                              verdict  rounds  actions  in_tok   out_tok  cost$    secs
    beeware__briefcase-2085           fail       4      72      70793    21640    0.0171   246.6
    conan-io__conan-17123             PASS      18      17     109452    14644    0.0118   147.8
    aws-cloudformation__cfn-lint-3798 fail       2       0       2665      333    0.0004     5.8
    cyclotruc__gitingest-94           PASS       3      31      26898    11111    0.0087    95.2
    dynaconf__dynaconf-1225           fail       2       0       2585      747    0.0006     8.4
    fonttools__fonttools-3682         PASS      11      12      62875     8239    0.0071    89.9
    huggingface__smolagents-285       fail       1       0       1235      394    0.0003     5.2
    instructlab__instructlab-2540     fail      12      10      50289     1810    0.0039    39.0
    joke2k__faker-2142                fail       1       0       1183      278    0.0003     4.7
    kubernetes-client__python-2303    fail       3       5      14161     5587    0.0042    59.6
    projectmesa__mesa-2394            fail       3       4      18865     8673    0.0063    85.7
    python-control__python-control-1064 fail    16      14      74240     6292    0.0072   147.8
    reata__sqllineage-661             fail       6       8      26319     4486    0.0041    71.6
    sphinx-doc__sphinx-12975          fail       9      11      25866     7282    0.0051    77.9
    stanfordnlp__dspy-1651            fail       4       6      25750    22629    0.0122   171.8

Solved 3 of 15.
Total tokens 624,323, total cost about $0.089 for the whole board.
The most expensive single task, briefcase, cost under two cents and was a failure; the three passes together cost under three cents.

## The three passes

All three passes share one property that is worth stating plainly: the model edited exactly the file the gold patch edits, and nothing else.

    task              edited file (== gold)                               rounds  actions
    conan-17123       conan/internal/api/config/config_installer.py         18      17
    gitingest-94      src/gitingest/parse_query.py                           3      31
    fonttools-3682    Lib/fontTools/ttLib/reorderGlyphs.py                   11      12

These are the well-localized tasks.
gitingest-94 and fonttools-3682 are the two the issue report all but hands over: the report names the file and the symbols, and for fonttools it even pastes a working patch, so the job is find-the-spot-and-transcribe.
conan-17123 is a shade harder, a `.conanignore` inverse-matching rule, and it took the model eighteen rounds of reading and editing config_installer.py to get there, but it stayed in the right file the whole time and ended on end_turn with the hidden test green.
gitingest is the cheapest and quickest engaged pass, three rounds of driving but thirty-one actions, because the code-as-action engine lets it batch reads and a grep into single turns.

## The near-miss: right file, wrong fix

briefcase-2085 is the interesting failure, because it is not a failure of localization.

The model edited `src/briefcase/commands/base.py`, which is the gold file.
It spent seventy-two actions across four rounds working that file, more actions than any pass on the board.
And it still failed: the hidden run ends `4 failed, 11 passed`.

So it knew where the bug lived and could not write the fix that satisfies the tests.
This is the honest edge of the model's ability on this suite.
The tasks it clears are the ones where the fix is a small contiguous edit with a clear anchor; briefcase asks for a behavioral change that the tests pin down precisely, and seventy-two actions of trying variants on the right file did not land it.
hy3-free does solve this task, so it is reachable for a strong free model; deepseek-v4-flash-free gets to the doorstep and not through it.

## The other eleven, in two shapes

The remaining ten failures (briefcase aside) fall into two clean buckets.

Explored but never wrote an edit.
instructlab-2540 (ten actions, twelve rounds), python-control-1064 (fourteen actions, sixteen rounds), sphinx-12975 (eleven actions), sqllineage-661 (eight actions), dspy-1651 (six actions), and the shorter kubernetes-2303 and mesa-2394 all ran real read and grep actions and then stopped without ever editing a source file.
Their summaries show `edited_files: null`.
The model looked, did not form a fix it was willing to commit, and ended the turn.
python-control is the sharpest example: sixteen rounds and fourteen actions of genuine exploration, `6 failed, 382 passed` at the end because nothing was changed.
These are capability failures, not harness failures: the loop offered the model an edit action every turn and it declined.

No engagement at all.
cfn-lint-3798, dynaconf-1225, smolagents-285, and faker-2142 each ran one or two rounds, zero actions, a few hundred output tokens, and quit.
dynaconf ends `4 passed`, smolagents `passed, 27 warnings`, faker `74 passed`, all with the fail_to_pass test still red because nothing was touched.
On these the model read the prompt and produced prose instead of a first action, which the engine scores as a non-start.
cfn-lint is the one that had aborted outright on the first board with no summary written; on the retry it resolved to this same zero-action non-start, so it is a real failure, not an infra death.

## What the fair number says

The headline correction is the one the abort-aware harness was built to make.
deepseek-v4-flash-free is not unmeasurable and it is not zero.
It is a real 3 of 15 at nine cents for the board, and its three passes are the well-localized transcription tasks that the free tier as a whole tends to clear.

It hits the same wall the flagships hit.
The six tasks the wider campaign treats as the hard ceiling (cfn-lint-3798, dynaconf-1225, sqllineage-661, instructlab-2540, sphinx-12975, python-control-1064) are all failures here too, and a gpt-5.6 codex sweep fails the same six.
Where deepseek-v4-flash-free differs from a flagship is the middle band: briefcase, where it finds the file and misses the fix, and the explored-but-unedited cluster, where a stronger model would commit an edit and a weaker-tier one does not.

The engineering takeaway for the free tier is that the retry policy is load-bearing.
Without it this model reads as noise, a scatter of aborts and a pass or two.
With it the signal is clean: three localized solves, one near-miss on the right file, and a set of failures that split neatly into did-not-commit and did-not-start.

Metrics: solved 3/15, total 624,323 tokens, total cost about $0.089, engine oi (code-as-action, graded by hidden check.sh), aborts retried up to two times per task, all three passes edited the exact gold file.
