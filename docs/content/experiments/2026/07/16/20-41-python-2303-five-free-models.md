---
title: "Five free models on python-2303: the recall wall, and a dropped dialect"
linkTitle: "python-2303, five free models"
description: "The earlier python-2303 run left the five free zen models unmeasured, since the free tier was rate limited all session, and it guessed the defensive-coding paragraph would matter most for a weak model. This run measures them. All five fail the task at pass@1, and not because the paragraph fell short: the models never reach the fix. Two bail in two rounds, three engage and two of those edit the exact files the gold diff touches, but none writes a change that turns the hidden test green. The task needs a memorized upstream fix that gpt-5.6 recalls and these models do not. One model, mimo-v2.5-free, surfaced a real harness bug on the way: it emitted every action inside a code_interpreter tool-call fence the oi parser did not read, so it ran zero of them. Salvaging that costume took its executed actions from zero to eleven."
date: 2026-07-16T20:41:00+07:00
---

The earlier run on kubernetes-client python-2303 ended with an honest gap.
The account-wide free tier returned a usage-limit error for the whole session, so the five free zen models the plan called for went unmeasured, and every arm ran on gpt-5.6.
That run guessed the free models were where the defensive-coding paragraph should matter most, since a weaker model is likelier to write the buggy fix and likelier to skip running its own check.
This run measures them.

The short version: the guess was wrong about where the paragraph helps, because the paragraph never gets a chance.
All five free models fail python-2303 at pass@1, and they fail before the question the paragraph addresses ever comes up.
None of them reaches a fix the hidden test accepts.

## Setup

The engine under test is tomo-oi, the code-as-action loop, driven in-process by `lab probe` against the offline task and graded by the task's own `check.sh`.
Each model gets one graded pass@1, no retry, a forty-round cap, and a twenty-minute inner deadline.

    for m in deepseek-v4-flash-free mimo-v2.5-free hy3-free \
             nemotron-3-ultra-free north-mini-code-free; do
      lab probe kubernetes-client__python-2303 \
        --engine oi --grade --max-rounds 40 --timeout 1200s \
        --model "$m" --out /tmp/oi_free_2303/"$m"
    done

The five are the account-wide free tier of the zen catalog, the same proxy every tool in the lab shares.
The grade is the real hidden test, run offline with the future git history stripped, so a green cannot be fetched and cannot be read off disk.

## The result

One graded pass@1 per model.

| Model | Rounds | Actions run | Input tokens | Reached the gold files | Grade |
|---|---|---|---|---|---|
| deepseek-v4-flash-free | 7 | 333 | 213k | yes, both | 2 errors |
| mimo-v2.5-free | 2 | 0 | 3k | no | baseline red |
| hy3-free | 40 (cap) | 40 | 476k | yes, both | baseline red |
| nemotron-3-ultra-free | 27 | 26 | 276k | no | baseline red |
| north-mini-code-free | 2 | 1 | 1k | no | baseline red |

Baseline red is the task's starting state, two failing and sixteen passing.
Three models end exactly there, having changed nothing that matters.
deepseek ends worse, on two errors, a fix that broke the import path it was editing.
hy3 edits both files the gold diff touches and still lands on baseline red, an edit that is not the right edit.
No model passes.

## What each model actually did

The spread is not one failure mode, it is three.

Two models quit almost at once.
north-mini-code-free ran a single action and stopped after two rounds.
mimo-v2.5-free stopped after two rounds having run nothing at all, for a reason that turned out to be the harness, not the model, and that is the next section.

Two models engaged hard and still missed.
deepseek-v4-flash-free ran three hundred and thirty-three actions across seven rounds, spent the most tokens of any model, and edited both files the gold diff touches, then left the tree throwing errors.
hy3-free ran to the forty-round cap, edited both gold files, and never turned them into a passing change.
Reaching the right files is the part code-as-action makes easy, and both of these did it.
Writing the change the hidden test wants is the part that needs the recalled upstream fix, and neither had it.

One model explored the wrong places.
nemotron-3-ultra-free ran twenty-six actions across twenty-seven rounds, read around the config loader, and never opened the two files the fix lives in.

The earlier run explained why gpt-5.6 clears this task: it recalls the real upstream fix from pretraining, reconstructs the hidden test, runs it, and iterates on red until it is green.
These five do not recall that fix.
Code-as-action gives them the mechanics, a shell, a loop, and a check they can run, but the mechanics are not the answer.
The lever on this task is knowledge the strong model has and the free models do not, so the defensive paragraph, which only shapes the fix once a model is writing one, never comes into play.

## The dropped costume, and the fix

mimo-v2.5-free is the interesting failure, because its first two rounds ran zero actions while its text was full of real ones.
It wrote its grep commands like this:

    <tool_call>
    <function=code_interpreter>
    <parameter=code>
    import subprocess
    result = subprocess.run(['grep', '-rn', 'exec', '--include=*.py', '.'], ...)
    print(result.stdout)
    </parameter>
    </function>
    </tool_call>

That is a Hermes-shaped tool call, and the oi engine has a dialect for it, but mimo is not routed to that dialect, so its replies go through the Markdown parser.
The Markdown parser has a no-fence salvage that recovers several off-Markdown shapes a model reaches for instead of a fence, an execute tool call, an HTML pre block, a language-named tag.
None of those read a `<function=NAME><parameter=code>` shape, because the code sits in a `<parameter=code>`, not the bare `<code>` the salvage matches.
So every action mimo emitted was dropped, the turn ended on nothing done, and after two empty rounds the model gave up.

That is a harness bug, not a model failure, so it got fixed.
The Markdown salvage now reads the `<function=NAME><parameter=...>` shape too, mapping a code_interpreter or python function name to python and everything else to shell.
Rerunning mimo on the fixed build took its executed actions from zero to eleven, over twelve rounds instead of two.

    # before: 2 rounds, 0 actions run, gave up
    # after:  12 rounds, 11 actions run, engaged

The fix does not flip mimo to a pass.
It still does not recall the upstream fix, so it still fails the grade.
What the fix buys is honesty: mimo now fails because it cannot solve the task, not because the harness threw its work away.
That is the whole point of the per-model dialect work, to measure the model and not the parser.

## Lessons

- The free models fail python-2303 at pass@1, all five, and the earlier run's guess about the defensive paragraph is moot. The paragraph shapes a fix once the model is writing one. These models never get that far, so the task's real gate for them is recall, not fix shape.
- Reaching the right files is not solving the task. Two models edited the exact files the gold diff touches and still failed, one of them by breaking the tree. Code-as-action makes the mechanics cheap and leaves the knowledge gap fully exposed.
- A silent zero-action round is a harness smell worth chasing. mimo looked like a two-round quitter and was really a model whose every action was being dropped. The bug hid as a weak-model result until the trace was read.
- Meet the model where it is. mimo speaks the code_interpreter costume, and the parser now reads it. The salvage does not change any model that writes a fence, and it does not rescue a model that cannot solve the task, it just stops throwing away work the model actually did.

## Reproduce

The probe is in-process and needs no container, so the whole sweep is a single loop.

1. Build the lab against the local tomo checkout: `go build -o /tmp/lab ./cmd/lab`.
2. Run each free model once, graded, with the loop in Setup, pointing `--out` at a per-model directory.
3. Read each run's `summary.json` for the graded result, the round and action counts, and the token cost, and `transcript.md` for the shape of the failure.
4. For a model that reports zero actions run, read the raw `events.jsonl` and look at what it emitted: an action written in an off-Markdown fence is the model working and the parser dropping it, not the model idling.
5. To see the dialect fix, rerun mimo-v2.5-free and confirm its action count moves off zero.
