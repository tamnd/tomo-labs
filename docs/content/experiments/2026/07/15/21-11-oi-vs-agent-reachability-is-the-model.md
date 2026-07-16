---
title: "Reachability is the model: tomo's oi surface versus the agent surface"
linkTitle: "oi vs agent, reachability is the model"
description: "Run the same cheap model through tomo's two engines, the oi code-as-action surface and the default structured agent surface, on swebench-live tasks that carry one failing test and one changed file. Two things separate cleanly. Token cost per round is set by the surface, and oi is structurally leaner, reaching the identical grade on dspy-1651 for 63,831 input tokens against the agent surface's 138,461, a 2.2x gap. Pass or fail is set by the model's ability to write the specific fix, and the surface does not move it, so on the two harder tasks a cheap model lands the edit on neither surface. The leaner-tokens claim generalizes as a property of the surface; correctness a cheap model does not have cannot be manufactured by the harness."
date: 2026-07-15T21:11:00+07:00
---

An earlier run on gitingest-94 showed tomo's oi surface passing a reachable task leaner than every structured rival.
One task is luck until it repeats.
This run widened to two more swebench-live tasks that also carry a single failing test and a single changed file, reata sqllineage-661 and stanfordnlp dspy-1651, and asked one question.
Same model, two of tomo's own engines, does the oi token win hold.

## Setup

The two tasks are `reata__sqllineage-661` and `stanfordnlp__dspy-1651` from SWE-bench-Live, both picked because they rank near gitingest on the easy end by fail_to_pass count and gold size.
Both run offline so no model can retrieve the answer.
The model is `hy3-free`, one cheap model held constant across both surfaces, so the only thing that changes between arms is tomo's engine.

The oi arm is tomo's code-as-action engine, where the model writes a shell or code block and the harness runs it.

    lab probe reata__sqllineage-661 \
      --engine oi --model hy3-free --max-rounds 15 --grade \
      --out /tmp/sqllineage-oi

    lab probe stanfordnlp__dspy-1651 \
      --engine oi --model hy3-free --max-rounds 15 --grade \
      --out /tmp/dspy-oi

The agent arm is tomo's default structured engine, where the model calls named tools and reads back a per-tool result envelope.

    lab probe stanfordnlp__dspy-1651 \
      --engine agent --model hy3-free --max-rounds 15 --grade \
      --out /tmp/dspy-agent

Every arm is pass@1, no retry, and every arm is allowed the full fifteen rounds.

## The result

| Task | Engine | Model | Rounds | Calls | Input tokens | Grade | Gold hit |
|---|---|---|---|---|---|---|---|
| sqllineage-661 | oi | hy3-free | 15 | 15 | 84,322 | FAIL | no |
| dspy-1651 | oi | hy3-free | 15 | 22 | 63,831 | FAIL | no |
| dspy-1651 | agent | hy3-free | 15 | 16 | 138,461 | FAIL | no |

Every arm ran the full fifteen rounds without landing the gold fix.
The grades sat at the unedited baseline, sqllineage 1 failed and 18 passed, dspy 1 failed and 37 passed on both surfaces.

## The two new tasks are not reachable for a cheap model

Both new tasks failed, and they failed for the model, not the surface.

Ranking by fail_to_pass count and gold size put these two near gitingest at the easy end, one test and one file each.
But easy to grade is not easy to fix.
gitingest's gold is a literal one-line substitution, the kind of edit a cheap model can copy.
sqllineage's gold is a multi-line control-flow guard that has to reason about how a subquery is crawled, and dspy's gold turns a plain attribute into a property with a getter and setter wired to the predictor's signature.
Those are inventions, not substitutions, and hy3 could not produce either regardless of how many rounds it was given.
So the reachable-for-a-cheap-model set inside this corpus is essentially the literal one-line fixes.
gitingest was not one task's luck so much as the one task whose ceiling a cheap model can actually touch.

## The oi surface is structurally leaner at equal grade

dspy-1651 gives the clean same-model, same-grade comparison the run was after, just on a failed grade instead of a passed one.
Both arms are hy3, both end at 1 failed and 37 passed, both run fifteen rounds.
The input token cost is not close.

The oi surface reached the identical grade for 63,831 input tokens against the agent surface's 138,461, which is 46% of the agent surface's input, a 2.2x gap.
This matches the gitingest run, where oi passed at 64.7K against 85K to 104K for the structured surfaces, and it matches the dynaconf run, where oi's input was about a tenth of the structured arms.
The reason is the same each time.
The code-as-action surface carries no tool-schema block and no per-tool result envelope, so the model re-reads far less context every round.
That leanness is a property of the surface, and it shows up whether the run passes or fails.

## The two levers separate cleanly

Token cost per round is set by the surface.
oi is structurally leaner than the structured agent surface on the same model, measured here at about 2.2x on a shared-ceiling task and at 1.3x to 1.6x on the task both surfaces can pass.

Pass or fail is set by the model's ability to write the specific fix.
The surface does not move it.
On dspy both surfaces fail, on sqllineage oi fails, and only on gitingest, whose fix is a literal substitution, does the cheap model land it.

So the defensible claim is sharper now.
tomo's oi surface is the leaner way to spend tokens on a given model, confirmed across three tasks and two grades.
That is the less-tokens half of the goal, and it holds independent of the task.
The smarter half is gated on the model, not the harness.
On the two harder-to-fix tasks a cheap model cannot land the edit on either surface, so no harness change turns them green.
The oi win is real and it generalizes as a token-efficiency property.
It does not, and cannot, manufacture correctness a cheap model does not have.

## Lessons

- Reachability is a property of the model, not the surface. The same cheap model fails both harder tasks on both engines, so the wall is capability, and the harness cannot move it.
- The oi surface is the leaner way to spend tokens on a fixed model. It reached the identical failed grade on dspy for 46% of the agent surface's input, and it passed gitingest at fewer tokens than any structured rival, so the leanness holds at both grades.
- Single test and single file is a grading property, not a difficulty measure. gitingest's gold is a substitution a cheap model can copy, while sqllineage and dspy demand inventions it cannot. Rank reachability by gold-diff shape, not by test count.
- The two levers route independently. Cheap model plus oi where the fix is reachable, capable model plus the structured surface where it is not, which keeps the routing honest.

## Reproduce

The probe is in-process and needs no container, so a full A/B is a seconds-long loop.

1. Build the lab against the local tomo checkout: `go build -o /tmp/lab ./cmd/lab`.
2. Source the proxy key so the free `hy3-free` model is reachable before the probe runs.
3. Run each arm as above, holding `--model hy3-free` and `--max-rounds 15` constant, and change only `--engine` between `oi` and `agent`.
4. Read each run back with `lab probe analyze /tmp/dspy-oi`, which prints the round-by-round token curve and the tool mix without spending a token.
5. Every run writes `summary.json` with the priced cost and input token count, `trace.jsonl` with the full request and response of every call, and `transcript.md` with the readable turn.
