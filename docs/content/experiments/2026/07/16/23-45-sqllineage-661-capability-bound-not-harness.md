---
title: "The second task, five free models: this one the harness got right and the models got wrong"
linkTitle: "sqllineage-661, capability not harness"
description: "The campaign's second slice baselines the five free zen models on sqllineage-661, the next-easiest swebench-live task. Where the first task was harness-bound, this one is the opposite. No free model solves it, and not one of the clean failures is the harness. Two models patch the public entry point, runner.py, when the real bug is a subtle join-clause traversal buried deep in the sqlfluff parser; a third investigates for sixteen rounds and never attempts an edit. The engine ran every action correctly, so there is no fair harness fix to ship: the only steer that would flip these runs is the location of the answer, which stays out of the harness. Two more aborted on an exhausted free-tier quota and are not scored. The finding of record is a clean negative result and a diagnostic that generalizes: task one was the harness's fault, task two is the model's."
date: 2026-07-16T23:45:00+07:00
---

The first slice of this campaign, cyclotruc gitingest-94, was the harness's fault.
Free models could solve it, and the failures were the engine running vim and a narration envelope the finish guard missed.
Both got fixed.

This slice is the mirror image.
The task is reata sqllineage-661, the next-easiest by gold-patch size, and the same five free zen models run it under the same protocol: one graded pass@1 each, a thirty-round cap, the network off, graded by the task's own check.sh with the future git history stripped.

The short version: no free model solves it, and this time the harness is not to blame.
The models mis-diagnosed a real bug, and there is no fair way for the harness to fix that.

## The task

sqllineage-661 is a wildcard-column-lineage bug.
The gold fix is thirteen lines in `sqllineage/core/parser/sqlfluff/utils.py`, inside a helper called `list_join_clause`: when a select reads from a subquery that has no join at the top level, the helper should not recurse down into the subquery hunting for join clauses.
The failing test is `test_output_consistency` in `tests/sql/column/test_metadata_wildcard.py`.
The important thing about that fix is where it lives: deep in the sqlfluff parser layer, several calls below the public entry point a user actually touches.

## The sweep

| Model | Rounds | Actions | Input tokens | Result | Edited | Cause |
|---|---|---|---|---|---|---|
| hy3-free | 9 | 8 | 34.8k | fail | runner.py | fixed the wrong file |
| nemotron-3-ultra-free | 30 | 30 | 405.6k | fail | runner.py | wrong file, ran to the cap |
| north-mini-code-free | 16 | 10 | 51.5k | fail | nothing | never converged to an edit |
| deepseek-v4-flash-free | 4 | 0 | 0 | abort | | quota exhausted, 429 |
| mimo-v2.5-free | 4 | 0 | 0 | abort | | quota exhausted, 429 |

The two aborts are honest infra, not task failures, and they are on me: health probes earlier in the same window spent deepseek's free quota, so the tier returned its usage-limit error for the smaller models.
By the campaign's abort-is-not-fail rule they are not scored, and a healthy-window re-run is queued.
The three models that ran are the real signal, and all three failed on the same thing: diagnosis.

## Two models fixed the wrong file

hy3 and nemotron both edited `sqllineage/runner.py`, the public entry point where a caller asks for column lineage.
The bug is not there.
It is in the parser helper `list_join_clause`, well below the surface, and the surface is exactly where a model lands first because the failing test calls through it.
Both models anchored on that first file and never followed the wildcard expansion down into the parser where the real defect sits.

nemotron is the instructive one.
It ran to the thirty-round cap still working: it read the actual failing test, wrote a small reproduction, ran it several times, and at the last round was still reading the entry point looking for the lineage call.
It was productive to the end, but productive in the wrong file.
A higher round cap would buy it more of the same, not the parser.

north never edited anything at all.
Its ten actions were all grep and sed into the entry point, the test helpers, and the holders module, and then it stopped without an attempt.
Every action ran exactly as written, so there is nothing for the harness to salvage.
It investigated and gave up.

## Why there is no fair harness fix

gitingest gave us two general repairs because the harness was genuinely wrong: it ran an editor verb as a shell command, and it missed a narration envelope.
sqllineage gives us none, because the harness did its job.
The parser parsed, the shell ran, the edits landed where the models aimed them.
The only lever that would turn these runs into passes is to tell the model the fix is in the parser and not in the entry point, and that is the answer to the task.
Putting it in the engine, even softly, would be teaching the harness the location of a fix, which is a leak.
So the harness stays as it is, and the honest report is that the three free models we could test were not able to trace this bug to its source.

## One thing worth noting, not fixing

When hy3 ran the broad test suite to check its work, it hit `PermissionError: Operation not permitted` on unrelated tests that open sockets or spawn a process pool, because the sandbox keeps the network off.
That did not cause the failure, which was already the wrong-file edit, and the grade is taken outside the sandbox so the score is untouched.
But a model that self-verifies with the whole suite gets a noisy signal from it.
Cleaning that up means loosening the same isolation that stops a model from fetching an answer over the network, so it is its own careful question, not a quick patch, and it is filed as one.

## Lessons

- The harness-versus-capability split generalizes. The first task was the harness's fault and got a fix and a free pass; the second is the model's and gets a documented wall. Both measure the model honestly, and that is the whole point.
- A wrong-file edit is a diagnosis failure, not a harness failure. When two models patch the public entry point and the bug is three layers down in the parser, there is nothing for the engine to repair without handing over the answer.
- A negative result is a result. sqllineage-661 is capability-bound for the free models tested here, and saying so plainly is more useful than inventing a fix that only works because it points at the solution.

## Reproduce

1. Build the lab against the current tomo and run the committed sweep: `scripts/campaign_sweep.sh reata__sqllineage-661`.
2. Read each run's summary for the graded result and, for a failing run, the edited files. A run that edits the public entry point and leaves the parser untouched is a diagnosis miss, not a harness miss.
3. A run whose summary shows a non-empty error and no pass is an abort on the free tier's quota, not a task failure; re-run it in a healthy window.
