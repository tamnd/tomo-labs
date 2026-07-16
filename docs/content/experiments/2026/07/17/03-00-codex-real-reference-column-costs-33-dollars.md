---
title: "Real codex on all fifteen tasks: four passes, thirty-three dollars, and one runaway that is half the bill"
linkTitle: "codex-real reference column"
description: "This slice steps away from the tomo-oi campaign to pin a reference column: real codex, the Rust CLI on a ChatGPT subscription, run against all fifteen swebench-live tasks on gpt-5.6 at medium effort, one graded pass each, in the same isolated harness. It solves four of fifteen and costs $33.32 in list-price tokens. One task, dynaconf, is $17.32 of that, more than the other fourteen combined, and it still fails. That is the finding in one number: real codex has no budget-aware stopping rule, so on a task it cannot close it does not stop, it spends five million tokens and fails anyway. Getting codex to run at all took a fix in the bridge, because codex sends its request with no tools and relies on the backend to inject them, so the old translation was stripping the tools it never sent. Three of codex's four passes are tasks a free model already solves for nothing, which sets up the real comparison: the same three tools on the same free model, next."
date: 2026-07-17T03:00:00+07:00
---

The campaign so far has been about one tool, tomo-oi, on the free roster.
This slice pins a fixed point to measure it against: what a strong rival costs at full strength, and where it breaks.

The rival is real codex, the Rust CLI, on a ChatGPT subscription.
Not a reimplementation of its loop, and not its prompt ported into another engine, but the actual binary, driven exactly as it drives itself.
It ran against all fifteen swebench-live tasks on gpt-5.6-sol at medium effort, one graded pass each, in the same isolated container harness every other tool uses, with the network off and the task's own hidden tests as the grader.

## Getting codex to run honestly

The first attempt produced nonsense: codex answered every task with a paragraph of prose and never touched a file.
The reason turned out to be structural, and worth writing down.

Codex speaks the Responses wire, and it sends its request with no tools of its own.
It relies on the ChatGPT backend to recognise it and inject its shell and apply_patch tools server-side.
The lab bridge, which lets any tool reach the model through the trace proxy, was translating codex's request into the chat format and back, and that round trip dropped the tools codex never sent, so the model arrived at the task with no way to run a command.
A model with no shell writes an essay.

The fix is to stop translating codex and forward its request verbatim.
The bridge now has a passthrough route that hands codex's own request straight to the backend, changing only the model name and the effort, so the backend still injects the tools and codex behaves precisely as it would on a developer's laptop.
The proxy still records every request and response, and it now reads the token usage in the Responses shape, so the input, output, and cache breakdown lands in the trace for each of the fifteen tasks.
Every run is saved so the column never has to be paid for a second time, which mattered more than expected.

## The bill

Here is the whole column, biggest bill first, priced at gpt-5.6-sol list rates.

| task | result | total tokens | cost |
|---|---|---|---|
| dynaconf-1225 | fail | 5,043,338 | $17.32 |
| dspy-1651 | pass | 661,542 | $2.55 |
| cfn-lint-3798 | fail | 503,576 | $2.11 |
| instructlab-2540 | fail | 561,684 | $2.01 |
| sphinx-12975 | fail | 429,116 | $1.70 |
| python-2303 | pass | 347,362 | $1.28 |
| python-control-1064 | fail | 345,730 | $1.22 |
| mesa-2394 | fail | 293,938 | $1.11 |
| sqllineage-661 | fail | 228,725 | $0.85 |
| conan-17123 | fail | 210,136 | $0.77 |
| smolagents-285 | fail | 206,657 | $0.68 |
| briefcase-2085 | pass | 169,154 | $0.50 |
| gitingest-94 | pass | 141,993 | $0.49 |
| fonttools-3682 | fail | 118,737 | $0.40 |
| faker-2142 | fail | 91,196 | $0.33 |

Four passes out of fifteen, and $33.32 in total.

Now look at the top row again.
dynaconf-1225 is $17.32, which is more than the other fourteen tasks put together, and it is a failure.
Codex spent five million tokens on one task it could not close, because nothing in it says stop.
It kept reading, kept editing, kept re-reading, and ran itself out on a problem it was never going to finish.

That is the single clearest argument for a budget-aware governor, which is the piece tomo-oi's loop has and codex's does not.
An earlier slice already showed dynaconf is budget-bound rather than a capability wall, a task a model can pass if it converges and quits instead of wandering.
Codex, at full strength on a strong model, wanders, and the meter runs the whole time.

## What the failures actually are

Nine of the eleven failures grade as "hidden tests not satisfied", which is the wall this campaign keeps hitting.
The test that decides the task is hidden, so a model that edits plausibly but not exactly gets no red signal to steer by, and it either settles on a near-miss or, on dynaconf, never settles at all.

The other two, smolagents-285 and python-control-1064, grade as "could not install project".
That verdict comes from the grader, which resets the repo, applies codex's patch, and installs the project before running any test.
It is not a flaky abort.
A free model running tomo-oi installs and passes smolagents-285, so the environment is sound, and the install broke on codex's own patch.

## The comparison this sets up

Three of codex's four passes are tasks a free model already solves, and solves for nothing.
gitingest-94 and briefcase-2085 are both solved by a free model at zero paid dollars, where codex spends about half a dollar each on a premium model.
python-2303 is solved by both, and an earlier slice showed the free-harness win there is real rather than a leaked test.

So the reference column does its job.
It says a strong rival at full strength clears four of fifteen, pays real money to do it, and has no brake when a task turns into a hole.
The next slice makes that a fair fight on the ground the product cares about: tomo-oi against pi and against opencode, all three on the same free model, scored on what they solve and what they spend.
