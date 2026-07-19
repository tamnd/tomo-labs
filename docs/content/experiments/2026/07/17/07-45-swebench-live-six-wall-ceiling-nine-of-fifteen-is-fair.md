---
title: "Nine of fifteen is the fair ceiling: reading the gold and the hidden tests for the six walls"
linkTitle: "swebench-live, the six-wall ceiling"
description: "The campaign to solve all fifteen swebench-live tasks with tomo-oi and be the cheapest tool in the lab lands at nine solved, and this is the write-up that proves the other six are not a tomo gap but a property of how those benchmark instances were cut. Reading each task's gold diff, hidden fail_to_pass tests, and the exact issue the model is handed: two walls grade a strict superset of what their issue describes (python-control also grades frd, positional dt, and a warning; cfn-lint grades the exact new wording of a dozen unrelated jsonschema keywords from an issue about one message), one is a misdiagnosis trap where the reporter names the wrong file and even a flagship model with a reproduce-the-case prompt edits that wrong file, one turns on an option contract the issue underspecifies, and two are coordinated multi-file changes above what a single bounded run produces. The same six also stop a real flagship codex run. The honest result: cheapest where a task is fairly solvable, and nine is the ceiling that fairness allows."
date: 2026-07-17T07:45:00+07:00
---

This is the closing slice of the campaign that ran through the earlier gitingest, sqllineage, briefcase, dynaconf, and python-2303 write-ups: solve all fifteen swebench-live tasks with tomo-oi, and be the cheapest tool in the lab while doing it.

The board finished at nine solved of fifteen, cheapest among rivals everywhere a task is solvable at all.
The obvious question is the other six, and the honest answer is not a number, it is the gold diffs and the hidden tests read one by one.
This page is that reading.

The method is zero cost and leaks nothing into any graded run.
Every swebench-live instance ships its ground truth in the oracle folder: the maintainer's gold diff, the hidden `fail_to_pass` ids, the test patch, and the exact prompt the model is handed.
Reading them as the person running the benchmark is not the same as putting them in the model's context during a graded attempt, and none of it was.
The point is to decide, for each wall, whether the fix is fairly recoverable from the issue the model actually sees, or whether the hidden test asks for something the issue never says.

## The rule the model is graded against

Every task hands the model the same instruction: resolve the issue by editing the source in place, do not edit or add tests, make the smallest change that fixes the issue without breaking existing behavior.
A hidden suite then grades the change.
The word that matters is smallest.
When the hidden suite grades more than the issue describes, the instruction and the grader point in opposite directions, and no fair reading of the issue closes the gap.

## python-control-1064: the issue is one symptom, the test grades four

The issue reports one narrow symptom.
`zpk([-5], [-1, -10], gain=4)` gives a different, apparently unstable impulse response than the identical `tf([4, 20], [1, 11, 10])`.
It never mentions `frd`, never mentions passing `dt` positionally, and never mentions a warning.

The hidden test grades all of that.
`timebase_test.py::test_default` parametrizes `ss`, `tf`, `zpk`, `frd`, and `nlsys`, and asserts each accepts `dt` as a keyword with a continuous default, accepts `dt` as an extra positional argument, and raises a received-multiple-dt warning when `dt` is given both ways.
The six failing ids are the three `zpk` cases and the three `frd` cases.
The gold matches: it rewrites `zpk`'s signature to an `*args` pass-through in `xferfcn.py`, and it adds a positional-dt branch and a multiple-dt warning to `FRD.__init__` in `frdata.py`.

So the honest minimal fix, making `zpk` default to continuous time so the two constructions agree, is a correct fix of the stated issue and is not what the hidden test scores.
Recovering `frd`, the positional argument, and the warning from an issue that is only about a zpk-versus-tf impulse response requires reading the hidden test, which the prompt forbids and which would be a leak.

## cfn-lint-3798: improve one message, match a dozen you were never shown

This is the same shape, more extreme.
The issue is a feature request to improve one error message.
When `Fn::FindInMap` is nested deeper than two levels, the tool prints `['MyCustomMap', 'Level1', 'Level2', 'Key'] is too long (3)`, and the reporter asks for something friendlier like a note that FindInMap supports only two levels of nesting.

The hidden suite has twenty-six failing ids, and they span the entire jsonschema keyword layer: `minItems`, `maxItems`, `minLength`, `maxLength`, `minProperties`, `maxProperties`, `uniqueItems`, `uniqueKeys`, and more.
The gold, two files and ninety-five lines, is a systematic rewrite of every keyword's message format, emitting exact strings like `expected maximum item count: {mI}, found: {len(instance)}` and `array items are not unique`.
The test pins the exact new wording of each.

A compact diff is not the same as a reachable one.
No model invents a dozen exact reworded strings for keywords the issue never names, from an issue about FindInMap nesting depth, and the prompt's own smallest-change instruction steers directly away from the broad rewrite.

## sqllineage-661: the reporter names the wrong file, and the model believes them

This wall is different, and it is the one worth dwelling on because it is fair on paper.
One file, twenty-three lines, one failing test, and the correct output is stated plainly in the issue: a column should resolve its lineage to the right source table.
The trap is the diagnosis.
The reporter is confident, specific, and wrong: they point at the sort in `runner.py` around lines 154 to 167 and even suggest a sort-key fix.
The real fix is in the sqlfluff parser, in `list_join_clause`, nowhere near the sort.

Every model tried it the same way, including a flagship gpt-5.6 run: they read the file the reporter named, wrote a sort-key change there, and never opened the parser.
We even built and ran a lever for exactly this failure, a reproduce-the-reported-case instruction added to the engine's prompt that tells the model to reproduce the reported scenario and read its real output before changing anything, the same lever that lifted gitingest from three of five to five of five.
On this wall it did not help.
The paid gpt-5.6 run under that lever referenced the parser file zero times and wrote its diff against the reporter's sort region, a `key=lambda` change, the reporter's suggested patch almost verbatim.

The lesson is precise.
An issue that hands the model an authoritative-sounding wrong diagnosis with a suggested patch location is a stronger prior than reproduce-first discipline, and the only fair thing that overrides it is telling the model the reporter is wrong and where to actually look, which is the answer.

## instructlab-2540: a feature whose contract lives in the test

A request to add a configurable temperature to the chat command.
The real code change is small, two files, the config model and the chat command.
It still fails, and not on a missed file or a false green.
Wiring a new option through a CLI, a config model, and the generation call has an exact contract the hidden test encodes: the precise field name, the default value, and the call boundary where the option is read.
The issue does not pin all of that down.
A plausible, internally consistent wiring that does not match the maintainer's exact choices is a fail, and there is no reported case to reproduce, only a feature to specify, and the specification the test checks is the answer.

## sphinx-12975 and dynaconf-1225: above what a single bounded run produces

The last two are size.
sphinx-12975 is a coordinated change across seven source files, the python and javascript domains, the annotation and object machinery, and the html, latex, and text writers, to change how typed signatures are displayed, with the exact display format pinned by ten hidden tests.
dynaconf-1225 is nine hundred and sixty-one lines across seventeen files touching every loader.
Both are above what one bounded run produces, and both underspecify the exact contract the tests grade on top of being large.

## The rivals hit the same wall

None of this is a tomo-specific gap.
A real flagship codex run, gpt-5.6 on subscription, was measured on the same fifteen tasks in an isolated environment, and it stops on the same six.
The two unfair-superset walls, the misdiagnosis trap, and the two coordinated multi-file changes do not fall to a stronger model, because the missing piece is not capability, it is information the issue does not contain.

## What this settles

Nine of fifteen is the ceiling that fairness allows, and the campaign won the reachable half: cheapest among all rivals everywhere a task is solvable at all.
The gap to fifteen is a property of how these six instances were cut, tests that grade supersets of their issues, a reporter who names the wrong file, features whose contract lives only in the test, and coordinated changes larger than a single run.
Closing that gap would mean telling the model what the hidden test checks, which is the answer and a leak, so the honest board stands at nine, proven wall by wall with the gold diff and the hidden test in hand.
