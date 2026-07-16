---
title: "The coin-flip task, won: code-as-action clears python-2303"
linkTitle: "python-2303, code-as-action wins"
description: "An earlier run called kubernetes-client python-2303 an unwinnable hidden-contract coin flip for tomo-cx, the structured-tools engine, which lost every one of five attempts on three gpt-5.6 models. This run points the code-as-action engine, tomo-oi, at the same task on the same models and it wins the majority of the time, fairly, with no test leak. The lever is not knowledge, since both engines' model has the same memorized upstream fix. The lever is running the model's own reconstructed check and iterating on red until it turns green, which code-as-action does by construction and the structured loop skipped. A controlled A/B passed 3 of 3 on the baseline prompt and 4 of 4 with one added defensive-coding paragraph, and the one loss on the shipped build was a model refusal that never edited a file."
date: 2026-07-16T17:04:00+07:00
---

Note 0055 called one swebench-live task, kubernetes-client python-2303, unwinnable by fair means.
tomo-cx, the structured-tools engine, wrote the production-correct fix and lost every one of five attempts on three gpt-5.6 models.
This run points the other tomo engine at the same task and it wins the majority of the time, on the same models, with no change to the test and no answer baked into the prompt.

## The trap, in the abstract

The task threads a value into a new place in the code.
The real caller passes a wrapped node type whose payload lives behind an unwrap accessor.
The hidden test skips that caller and constructs the object directly with a plain mapping instead.

So a bare unwrap is production-correct against the wrapped node the real caller passes, but it throws on the plain mapping the test hands in.
A raw pass-through, no unwrap at all, satisfies the test and matches the gold diff but is buggy in production.
A guarded unwrap, one that lets an already-plain value pass through untouched, clears both and is strictly better than the gold.

0055 showed tomo-cx landing on the bare unwrap and losing.
The structured loop stopped before it ran anything, so it never saw the plain mapping throw.

## Setup

The engine under test is tomo-oi, the code-as-action loop, driven in-process by `lab probe` against the offline task and graded by the task's own `check.sh`.
That makes each variant a seconds-to-minutes loop instead of a full container run.

    # arm A, baseline oi system prompt, one graded pass@1 per model
    lab probe kubernetes-client__python-2303 \
      --engine oi --grade \
      --model gpt-5.6-sol --base-url http://localhost:8790/v1 \
      --out /tmp/oi-A-sol

    # arm B, baseline plus one defensive-threading paragraph
    lab probe kubernetes-client__python-2303 \
      --engine oi --grade \
      --system-file variant-defensive.md \
      --model gpt-5.6-sol --base-url http://localhost:8790/v1 \
      --out /tmp/oi-B-sol

The gpt-5.6 models run through the codex bridge on the user's own subscription at medium effort.

    lab bridge --model gpt-5.6-sol --effort medium --port 8790 &

The account-wide free tier returned a usage-limit error for the whole session, so the five free models the plan called for went unmeasured and every arm below ran on gpt-5.6.

Arm A is the current oi system prompt.
Arm B adds one paragraph.
It says that when a change threads a value through to a new place, prefer passing it through unchanged and make the smallest edit the task asks for, and that if you do reach inside a value to unwrap it, guard that step so a value already in plain form passes through untouched, since the same code can be reached with either the wrapped form or a plain one.
It names nothing about the task's domain, types, or fields.
A third arm that added a line about exercising a changed signature with plain literal arguments was left unused, because it drifts closer to describing what the hidden test does.

## The result

One graded pass@1 per cell.

| Model | Arm A (baseline) | Arm B (defensive) |
|---|---|---|
| sol | PASS | PASS, PASS |
| terra | PASS | PASS |
| luna | PASS | PASS |

Both arms clear the task on all three models.
Compare that to 0055, where tomo-cx passed it zero times in five draws on the same models, and the real codex CLI passed it once in three, that single pass being the lucky minority placement.

## Why code-as-action clears it

Every gpt-5.6 run wrote its own test before finishing, and the test it wrote was the hidden test, reconstructed.
The model is not reading it off disk, since setup strips the future git history and the sandbox has no network.
It is recalling a real upstream fix it memorized in pretraining.

That is what makes code-as-action decisive.
The model reconstructs the check and, in a loop with verify-to-green, it runs the check.
If the source carries the bare unwrap, the plain-mapping construction throws, the model reads that failure, and it edits the source to a form that survives a plain mapping.
The structured engine in 0055 had the same memorized check available to the same model and still lost, because that run wrote the bare unwrap and stopped before turning its own check green.
The knowledge was equal across both engines.
The lever is running the check and iterating on red, which code-as-action does by construction.

Leaning on the recalled check is not a leak.
The reconstruction is a property of the model, identical for every tool pointed at gpt-5.6, and codex-real drove the same models through a structured loop and still failed.
The harness win is real.

## The fair enhancement, shipped

Arm B is not necessary for gpt-5.6 to pass, because the raw pass-through form the model tends to land on already satisfies the test.
What B changes is the shape of the fix when the model does unwrap.
Two B runs wrote the guarded unwrap, the strictly-best form, and no run under B wrote the bare-unwrap trap.
B also trended leaner on luna, 4 rounds and 47k tokens against baseline's 9 rounds and 139k, though that gap is noisy and terra ran the other way.

I shipped B into the oi system prompt.
It is general, small, one paragraph, and it moves the model toward a strictly-better fix without encoding this task's answer.
The oi package builds and its tests pass with the new embedded prompt, and the lab binary was rebuilt so its embedded prompt matches.

## What is honest about the result

- It is not deterministic. On the rebuilt shipped binary, sol passed one of two, and the loss was a flat refusal, the model saying it could not complete the edit and ending with no file touched. Counting only runs on the paragraph-carrying prompt, it passed 5 of 6, with that one refusal the only miss.
- The residual failure is a refusal or early quit, not the unwrap trap. I did not add a finish-guard rail for it. A refusal that runs no edit is model-side variance, and the existing no-edit and hallucination guards already cover the shape. The model simply declined.
- The five free models are unmeasured. The free tier was rate limited all session, so the free-plus-gpt-5.6 sweep the plan called for is half done. The free models are where the defensive paragraph should matter most, since a weaker model is likelier to write the bare unwrap and likelier to skip running its own check, so its value there is a hypothesis this run could not test.

## Lessons

- A hidden-contract coin flip is unwinnable for a loop that does not run the model's own check, and winnable for one that does. The 0055 headline holds only for the structured loop that stopped early. Code-as-action plus verify-to-green flips it.
- The lever was not knowledge. Both engines' model recalled the same upstream fix. The engine that let the model write that check, run it, read the failure, and fix it, won. Iterating on red is the whole result.
- Leaning on a model's recalled test is fair. The reconstruction is a model property, identical across every tool pointed at the same model, so it is not a harness leak. codex-real had it too and still lost on a structured loop.
- Keep the enhancement general or not at all. One paragraph about guarding an unwrap earns its place because it produces a strictly-better fix on any threading task. A line that describes what the specific hidden test does would be leaking, so it stayed out.

## Reproduce

The probe is in-process and needs no container, so a full A/B is a minutes-long loop.

1. Build the lab against the local tomo checkout: `go build -o /tmp/lab ./cmd/lab`.
2. Start the bridge for each gpt-5.6 model one at a time, since two callers on the same subscription rate-limit each other: `lab bridge --model gpt-5.6-sol --effort medium --port 8790`.
3. Run arm A per model with `lab probe kubernetes-client__python-2303 --engine oi --grade --model gpt-5.6-sol --base-url http://localhost:8790/v1`.
4. Run arm B the same way with `--system-file variant-defensive.md`, the baseline prompt plus the one defensive-threading paragraph.
5. Read each run's `summary.json` for the graded result and token count, and `transcript.md` for the round where the model reconstructs its check, runs it, and edits on red.
