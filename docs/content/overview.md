---
title: "Overview"
linkTitle: "Overview"
description: "What tomo-labs is, every part of it, and why each part exists: the fixed-model idea, the trace proxy, deterministic reruns, the metric set, the scenario and eval tiers, system-prompt recovery, and reproducible reporting."
weight: 5
featured: true
---

tomo-labs answers one question about coding agents: with the model held constant, how much does the agent itself cost you to get the same work done?
Most agent benchmarks change three things at once when they compare a number, the model, the prompt scaffolding, and the tool's own overhead, so the number tells you nothing clean.
tomo-labs pins the model and the decoding, routes every agent's traffic through one proxy, and grades the files an agent left on disk rather than the prose it wrote about itself.
What is left to differ is the agent: its prompt, its tool design, its planning, its token appetite, and its footprint.

This page is the whole feature set in one place.
Each section is a real capability with a reason it exists, and a pointer to where it is documented in full.

## The fixed-model idea

Every agent under test points its model base URL at the trace proxy.
The proxy forwards to one upstream model, the same model, with the same greedy decoding for all of them.
So when tomo finishes a task in 14,000 tokens and another agent finishes it in 180,000, that gap is the agent, not a stronger or weaker model on one side.
The default upstream is a free hosted deepseek model that does tool calling, so a full comparison sweep costs nothing to run.

## The trace proxy

The proxy is the one thing on the network path, and it does four jobs at once.

- It records every request and response verbatim into the run's trace, so nothing is summarized away before you can look at it.
- It normalizes wire dialects.
  Agents speak different protocols: native chat-completions, the OpenAI Responses API, the Anthropic Messages API, and Google's Gemini wire.
  The proxy translates each one to a single chat-completions call upstream, and tags the origin in the recorded path, so a codex request reads `(from responses)` and a claude-code request reads `(from messages)`.
- It forces determinism.
  Every completion is rewritten to temperature 0, top_p 1, and a fixed seed, so client-side sampling variance is gone and every tool is judged under one decoding regime.
- It times the call and counts the tokens, so time to first byte, total latency, and token usage are the lab's measurement rather than each tool's own reporting, which no two tools expose the same way.

The `Authorization` header is never written to a trace, so a key a tool carried never lands on disk.
The full architecture and trace schema is in [how it works](/guides/how-it-works/).

## Containers and isolation

Each agent runs in its own throwaway container, built on a shared base image, with the working tree mounted as its cwd.
The proxy runs as a per-run sidecar, so one run's trace never bleeds into another's, and a crashed container never blocks the next run.
tomo is containerized too, on the same base, so it is measured on the same footing as every other agent rather than getting a home-field advantage.

## Deterministic reruns

A benchmark is only fair if a rerun means the same thing, and a hosted model is not naturally repeatable.
Two levers close most of the gap, and neither is tuned to a scenario.
The proxy forces greedy decoding, and the harness runs up to a few attempts per task and stops at the first pass, which soaks up the residual nondeterminism a model still shows at temperature 0.
A pass that needed more than one attempt is recorded as such, so flakiness is measured rather than papered over.

## What it measures

Every number below is the same measurement for every agent, because the proxy and the harness take it, not the tool.

- Correctness, from a checker that inspects the files on disk, runs the code, or diffs output, and never reads the model's account of what it did.
- Tokens: prompt, completion, and total, summed across every request in the run.
- Memory: peak resident set size, from GNU time wrapping the agent process.
- Wall time, around the container and around the agent.
- Disk: the working tree size before and after, so a task's footprint is visible.
- Requests: how many model calls the agent made to finish, a rough read on back-and-forth.
- Latency: time to first byte and total per call, averaged over the run.
- Install footprint: the tool's own bytes sitting on top of the shared base image, a real cost most benchmarks never show.
- Orchestration: how many model round-trips a run took, and whether the agent planned, by counting its real plan or todo tool calls and any subagents it spawned.

## Scenarios and the Hi! baseline

The core suite is a set of hand-written scenarios, each exercising one behaviour and small enough to read at a glance, from a baseline greeting through a small project scaffold.
Every scenario grades by exit code from a `check.sh` that runs on the host, so the grade is about the work, never the words.
The `00-hello` scenario sends a single "Hi!" and expects a reply, which isolates each tool's fixed round-trip cost: its system prompt size, the context it injects, and the tokens it burns before doing any work at all.
Every tool page traces its own `00-hello` run end to end in a [Hi! section](/tools/).
The suite is listed in [scenarios](/guides/scenarios/).

## The eval tiers

Beside the hand-written scenarios, tomo-labs can run whole public benchmarks rendered into the same task shape.
A tier is selected with `--suite <name>`, reads its tasks from `evals/<name>/tasks/`, and lands results in a separate tree so it never mixes into the core report.
Two tiers ship today: the Aider polyglot benchmark (Exercism exercises graded by their own tests) and EvalPlus (HumanEval+ and MBPP+ with their expanded hidden tests).
`lab gen --suite <name>` materializes a tier by fetching the upstream benchmark and proving each task against a known-good solution before keeping it, so a task that cannot be validated never lands.
The expected answers are kept in a sibling directory the harness never mounts, so an agent only ever sees the prompt and its starting files.
See [evals](/evals/) for the full tier documentation.

## System-prompt recovery

Because the proxy records every completion after normalizing it to chat-completions, each tool's real system prompt is sitting in the trace, whatever wire the tool speaks.
`lab prompts <tool>` reads that tap across every run, unions the distinct system prompts, groups the per-run renderings that differ only in volatile spans like the date or a session id, and ranks the agent's working prompt first.
That makes the trace the ground truth for what actually reached the model, not a copy from a tool's source that may be out of date.
The recovered prompts are checked into the repo under [prompts](/prompts/), one page per tool, so any change a tool makes between versions shows up in a diff.

## Reproducible reporting

`lab report` reads the captured runs and prints one comparison table.
It keeps only the latest run per tool and scenario, so every row is scored over the same set of tasks rather than drifting as history piles up.
It prices tokens at published reference rates, so the free tier's zero bill still yields a comparable dollar figure.
It splits the agents that planned from the ones that ran flat, because planning is a per-scenario choice even for agents that can plan.
The current sweep across the eight fully-swept tools is in [results](/guides/results/).
The harness wires eleven agents today; kilocode, aider, and copilot are the three newest, validated on the `Hi!` baseline with their full sweep still pending.

## Adding an agent

A new agent joins the comparison with two files, a `Dockerfile` on the shared base and a small `adapter.sh` that drives the tool non-interactively and points its base URL at the proxy.
No fork of the harness, no change to the metrics.
The two files are covered in [adding a tool](/guides/adding-a-tool/).

## Where to go next

- Read [how it works](/guides/how-it-works/) to trust the numbers before you read them.
- Read the [tools](/tools/) pages for a deep dive on each agent, its command surface, its architecture, its captured system prompt, and a Hi! run traced end to end.
- Read [results](/guides/results/) for the current comparison table.
- Follow the [quick start](/getting-started/quick-start/) to run a sweep on your own machine.
