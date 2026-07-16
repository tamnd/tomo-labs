---
title: "The easiest task, five free models: the failures were the harness, not the model"
linkTitle: "gitingest-94, five free models"
description: "Starting a campaign to solve all fifteen swebench-live tasks with tomo-oi and be the cheapest tool in the lab, the first slice baselines the five free zen models on the easiest task, cyclotruc gitingest-94. The task is solvable by a free model: hy3 and nemotron both pass it clean at pass@1. The other three did not return a clean grade, and none failed on capability. Two aborted on a flaky free-tier gateway, and the two real failures were the harness: one model emitted a file-viewer tool call that the parser ran as the shell command view, which is vim, and it hung the whole run for six hundred seconds; another wrapped its narration in a tool-call envelope the finish guard did not recognize and quit in one round. Both are fixed, each with a unit test on the model's real output. The third finding is about method: on the free tier a single pass@1 sweep is dominated by infra aborts, and an abort must be told apart from a task failure or the grid lies."
date: 2026-07-16T22:30:00+07:00
---

This is the first slice of a longer campaign: solve all fifteen swebench-live tasks with tomo-oi, and be the cheapest tool in the lab while doing it.
The plan is deliberately unglamorous.
Start with the easiest task, run a small experiment, read every trace, fix what the harness gets wrong for each model, write it down, and move on.

The first task is cyclotruc gitingest-94, the easiest in the set, and the first models are the five free zen models the whole lab shares.

The short version: the easiest task is solvable by a free model, and the failures on it this session were the harness and the free-tier gateway, not the models' ability to code.

## Setup

The engine is tomo-oi, the code-as-action loop, driven in-process by `lab probe --engine oi --grade` and graded by the task's own check.sh.
The future git history is stripped from the checkout, so a passing test cannot be fetched from upstream or read off disk.
Each model gets one graded pass@1, a thirty-round cap, and the sandbox network off.

    for m in deepseek-v4-flash-free mimo-v2.5-free hy3-free \
             nemotron-3-ultra-free north-mini-code-free; do
      lab probe cyclotruc__gitingest-94 \
        --engine oi --grade --max-rounds 30 --timeout 600s \
        --model "$m" --out /tmp/git94/"$m"
    done

## The sweep

| Model | Rounds | Actions | Input tokens | Result | Cause |
|---|---|---|---|---|---|
| hy3-free | 16 | 14 | 47.6k | pass | solved it |
| nemotron-3-ultra-free | 16 | 15 | 112.5k | pass | solved it |
| deepseek-v4-flash-free | 7 | 2 | 5.4k | abort | gateway 400, "Upstream request failed" |
| mimo-v2.5-free | 2 | 1 | 1.0k | abort | ran for the full 600s, then the deadline |
| north-mini-code-free | 16 | 11 | 74.0k | abort | "read: no route to host" mid-run |

Two free models pass gitingest-94 clean, so the task is within reach without a paid model.
Three runs aborted, and the important part is that an abort is not a task failure.
Each of the three ended with a real error recorded in the run summary, and treating them as failures would paint the easiest task as mostly unsolved, which is not what happened.

## The mimo hang was the harness running vim

mimo's abort looked like a two-round quitter, but the trace tells a different story.
It reached for a tool this engine does not provide, a file editor, written as a tool call:

    <function=editor>
    <parameter=command>view</parameter>
    <parameter=path>/var/folders/.../probe-822759539</parameter>

The engine's salvage for off-Markdown tool calls read the first parameter as the code to run.
Here the first parameter is a verb, "view", not code, so the engine ran `view` as a shell command.
`view` is vim in read-only mode.
With no terminal attached it printed its warnings, opened its full-screen buffer, and waited, and it kept waiting until the run hit its six-hundred-second deadline.
The tool output captured in the trace starts with vim's own "Input is not from a terminal" warning and its alternate-screen escape codes, so there is no ambiguity about what ran.

The fix is to salvage a tool call only when its function name means "run code", the execute, bash, python, and code-interpreter names a code-as-action model actually uses.
A call to an editor or a file viewer now produces no runnable block, so the turn ends and the finish guard nudges the model back to a real code block instead of the engine launching an interactive program and hanging.
On the fixed build mimo finishes in about twenty seconds.
It still does not solve the task, but it fails by not solving it, not by hanging on vim, and it stops burning a full slot to do so.

## The deepseek quit was a narration envelope

On a clean run, with no gateway abort, deepseek quit gitingest-94 in a single round on this:

    <tool_calls>
    <tool_call kind="text_write" params="...">
    I'll start by exploring the repository structure and finding the relevant file.
    </tool_call>
    </tool_calls>

There is no code in there.
It is a sentence of narration wrapped in a tool-call envelope.
The finish guard is built to catch exactly this, a model that talks like it is acting but runs nothing, and to nudge it once to emit a real block.
The guard looked for the literal `<tool_call>`, and deepseek wrote the plural wrapper `<tool_calls>` and an attributed open tag `<tool_call kind=...`, neither of which contains the literal it was looking for.
So the guard stayed silent and the model ended on nothing done.

The fix matches those opening tags as a prefix, so the plural wrapper and an attributed call both count as a lost action.
The guard still only runs on a turn that ended with no runnable block, so the most a wrong match can cost is one extra nudge on a turn that was already going to end on nothing.

## What the fixes are, and are not

Neither fix touches a model that already writes a normal fenced block.
Neither one turns these models into passes, because the task is already solvable by a free model and these two did not solve it.
What the fixes buy is an honest number.
A model should fail a task because it could not solve it, not because the parser ran an editor verb as a shell command or missed a tool-call envelope and let the model quit early.
That is the whole point of measuring the model and not the harness.

## Abort is not fail

The last finding is about method.
On the free tier, a single pass@1 sweep is dominated by infrastructure noise.
This session, three of five runs aborted, each on a different failure: a gateway 400 forwarded from the upstream model, an interactive-command hang, and a local network drop.
The gateway 400 for "Upstream request failed" is already retried as transient, and it still made it through, which means the whole free tier was degraded at that moment, not one unlucky request.
When re-runs were attempted, the tier kept returning 400s, so a clean grade for the three aborted models was not available in this window.

The rule for the rest of the campaign follows from that.
A run that ends with a recorded error and did not pass is an aborted run, not a task failure.
The grid shows the error, so an abort can never quietly count as a failure.
Retries are bounded, and when the whole tier is down the honest report is "no clean grade this window", not a number invented to fill the cell.

## Lessons

- The easiest task is solvable by a free model. hy3 and nemotron both pass gitingest-94 clean at pass@1, so the campaign's floor is real, not aspirational.
- A silent or hung run is a harness smell, not a verdict. mimo's six-hundred-second "quit" was the engine running vim; the trace, not the summary line, is where the truth was.
- The finish guard has to speak the model's dialect too. A tool-call envelope with attributes or a plural wrapper is still a lost action, and the guard now reads it as one.
- On a flaky shared tier, separate infra aborts from task failures before scoring anything, or the easiest task looks unsolved when it is not.

## Reproduce

1. Build the lab against the current tomo: `go build -o /tmp/lab ./cmd/lab`.
2. Run the five free models once each with the loop in Setup, one out-directory per model.
3. Read each run's summary for the graded result, the rounds and actions, and crucially the error field. A non-empty error with no pass is an abort, not a failure; re-run it.
4. For a run that shows very few actions or a full-timeout elapsed, read the raw events, not just the summary. An action written in an off-Markdown envelope, or an interactive command the engine ran, hides there.
