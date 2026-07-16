---
title: "Code as action: a tomo engine where the model acts by writing one code block"
linkTitle: "oi code-as-action engine"
description: "tomo gets a new engine, engine/oi, ported from the shape of Open Interpreter 0.4.2. The model's only action is one Markdown code block, the engine runs it in the sandbox and feeds the output back, and the turn ends when a reply carries no block. That removes the structured tool-calling surface that cheap models misuse, malformed JSON, wrong argument names, calling a tool that does not exist, because there is no tool registry to mis-call. The first run drove the engine end to end on dynaconf-1225, parse, gate, execute, feed back, and grade with the real hidden check, before the free model's rate limit cut it off, so this is a plumbing confirmation and the head-to-head convergence numbers still wait on a cooled-down model."
date: 2026-07-15T18:47:00+07:00
---

Cheap models are bad at structured tool calls.
They emit malformed tool JSON, they name arguments wrong, they call a tool that does not exist.
Open Interpreter sidesteps that by giving the model one action, write a Markdown code block, and running it.
This note ports that shape into tomo as a new engine and shows the first run through it.

The comparison arms are tomo's own agent engine and this new engine/oi.
Not cx.

## What the code-as-action shape is

Read from the upstream source of Open Interpreter 0.4.2, not the marketing.

The loop is a plain `while True`.
Each pass either asks the model for a reply or executes the code the last reply carried, and it alternates.
The model's action surface is a single code block.
A model that supports function calling gets one `execute(language, code)` tool, a model that does not gets a system prompt telling it to write a fenced block and the harness parses the fence out of plain text.
Both paths collapse to the same internal message shape, so the rest of the loop does not care how the code arrived.

The stop condition is the whole trick.
The turn ends when the model's reply carries no code block.
Prose with no fence is how the model says it is done.
There is no final-answer tool, no structured done signal, nothing to mis-format.

The lesson worth stealing is the shape, not the Jupyter kernel underneath it.
One action, a text stop condition, small tail-kept output, and a prompt that pushes one small step at a time.

## The tomo port: pkg/engine/oi

The port is built native to tomo's interfaces so the probe and the CLI drive it exactly like the other engines.

- `codeblock.go` parses fences out of a plain-text reply.
  It handles backtick and tilde fences, a bare fence with no language tag, and an unclosed fence at the end of a truncated reply, so a cut-off but runnable block is not silently dropped.
- `exec.go` maps the fence tag to a language and runs it in the sandbox.
  Python and its aliases go to `python3 -c`, shell and its aliases to `sh -c`, and anything else, json, diff, text, is not run and the model is told so.
  Output is tail-kept to 6 KiB, larger than Open Interpreter's 2800 characters because code tasks print real tracebacks, still bounded.
- `oi.go` is the loop.
  It builds a provider request with no tools, streams the reply, parses the runnable blocks, and if there are none it stops, else it runs each block and feeds the outputs back as one user message.
  A reply cut off at the token ceiling mid-code is nudged to continue rather than mistaken for a finished turn, bounded by a max-continues count so it cannot spin.
  Every execution passes tomo's policy gate and reports to the sink as an execute tool call, so confinement and the trace match every other engine.
- `prompt.go` and `prompts/system.md` adapt the brief to tomo.
  One action, make a plan with as few steps as possible, do not try the whole plan in one block, print what you need to see, verify with tests, keep test output small, and producing no code block ends the turn.

The engine carries no tool registry.
Its only action is run-this-code, straight to the sandbox.
That is the point, nothing to mis-call.

It is wired into the CLI as `--engine oi` and `TOMO_ENGINE=oi`, and into the lab probe as `lab probe <task> --engine oi`, so the cheap containerless A/B can drive it.
The work is on the tomo branch engine-oi-code-as-action.

## Setup

The task is `dynaconf__dynaconf-1225` from SWE-bench-Live, run offline so no model can retrieve the answer.
The run drives the new engine through the lab probe, which materializes the offline tree, builds the task venv, runs the loop, and grades against the real hidden `check.sh`.

    lab probe dynaconf__dynaconf-1225 \
      --engine oi --prep-env --grade \
      --out /tmp/oi-run

The model is the free deepseek proxy.
This is a plumbing run, pass@1, no retry.

## The result

| Run | Engine | Task | Model | Stopped by | Graded |
|---|---|---|---|---|---|
| 1 | oi | dynaconf-1225 | deepseek free proxy | rate limit at 3.5s | 5 fail, 4 pass |

The probe drove the oi engine end to end.
It materialized the offline tree, built the task venv, ran the loop, executed the model's code blocks, and scored the result with the real hidden check, five failed and four passed at the point it stopped.
So the plumbing is real, parse, gate, execute, feed back, and grade.

## What the run proves and what it does not

The run was cut short at 3.5 seconds by the free model's rate limit, a 429 FreeUsageLimitError, not by the engine.
So this is a plumbing confirmation, not a convergence result.

That is a real result on its own terms.
It shows the code-as-action loop parses a real model's fenced blocks, runs them under the same policy gate as every other engine, feeds the output back, and lands on a graded score against the hidden check.
The engine surface is what it claims to be, one action to the sandbox with no tool registry to misuse.

What it does not show yet is the point of the exercise.
The head-to-head numbers, oi against tomo's original agent engine, rounds and tokens and grade, wait on the free model cooling down or a small bounded budget on the cheap paid model.
Whether removing the tool-calling surface actually buys fewer wasted rounds on a cheap model is the next run, not this one.

## Lessons

- One action removes a class of failure by construction. With no tool registry, a cheap model cannot emit malformed tool JSON, name an argument wrong, or call a tool that does not exist. The only thing it can do is write code, and the engine runs it.
- The stop condition is prose. A reply with no code block ends the turn, so there is no final-answer tool to mis-format. The one guard that matters is not mistaking a token-ceiling cutoff mid-code for a finished turn, which the loop handles by nudging the model to continue under a bounded cap.
- Keep the shape, drop the kernel. tomo does not need Open Interpreter's Jupyter kernel. It needs the shape, one action, a text stop condition, tail-kept output, a one-small-step prompt, wired to tomo's own sandbox and policy gate.
- A plumbing run is worth reporting. Parse, gate, execute, feed back, and grade all fired against a real model and the real hidden check before the rate limit hit. That confirms the surface is honest, and it keeps the convergence question clean for the next run.

## Reproduce

The probe is in-process and needs no container, so a run is a seconds-long loop.

1. Build the lab against the local tomo checkout on the engine-oi-code-as-action branch: `go build -o /tmp/lab ./cmd/lab`.
2. Source the proxy key for the free deepseek model so it is in the environment before the probe.
3. Run the probe with `--engine oi --prep-env --grade` as above. `--prep-env` builds the task venv first so the run measures the engine, not `pip` fighting.
4. Read the run back with `lab probe analyze /tmp/oi-run` to see the round-by-round curve and the executed blocks without spending a token.
5. The run writes the priced summary, the full request and response trace of every call, and the readable transcript, so the parse, gate, execute, feed-back, and grade steps are all inspectable after the fact.
