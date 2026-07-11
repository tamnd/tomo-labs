---
title: "How it works"
description: "A run end to end: the trace proxy, the worker pool that runs a sweep in parallel, and the two levers that keep a rerun meaning the same thing."
weight: 10
---

```
scenario prompt ─▶ tool container ─▶ trace proxy ─▶ upstream model
   (/scenario)      (runs in /work)   (records +      (deterministic,
                                        translates       same for
                                        the wire)         every tool)
                          │
                          ▼
                     work left in /work ─▶ checker ─▶ result.json
```

## A run, start to finish

For one (tool, scenario) pair, `RunOne` does this:

1. Make `$HOME/data/<tool>/<scenario>/<timestamp>/`. Each try gets its own `attempt-N/` with `work/` and `trace/` under it.
2. Run the scenario's `setup.sh` on the host to lay fixtures into `work/`.
3. Start the trace proxy with `trace/` mounted in, and wait for it to answer.
4. If the scenario needs a web page, start the static sidecar too.
5. Run the tool container: `work/` as its cwd, `scenario/` read-only, `trace/` for output, and `LAB_BASE_URL` pointed at the proxy. The tool's `adapter.sh` runs the task under `/usr/bin/time -v`.
6. Stop the sidecars.
7. Run the scenario's `check.sh` on the host against `work/`, which passes or fails by exit code.
8. If it failed and tries remain, go back to step 1's next `attempt-N/`. If it passed, or the last try is spent, stop.
9. Read the numbers back out of the last try's `trace/` and write `result.json` at the run root, recording how many tries the pass took.

Sidecars are per run, so one run's trace never bleeds into another's, and a crashed container never blocks the next run.

## Running a sweep in parallel

A full sweep fans out across a worker pool, `LAB_CONCURRENCY` deep (default 3). Each worker gets its own proxy container name and port, so two runs in flight never share a sidecar or bleed into each other's trace. `lab -p` (an ad-hoc prompt against every tool) goes through the same pool, so its timing is representative of a real sweep.

Concurrency inside one `lab run` process is fine. Two separate `lab run` processes racing each other is not: each owns the shared web sidecar container by a fixed name, and a second process can stop the first one's sidecar out from under it. Run sweeps one process at a time and let `LAB_CONCURRENCY` do the parallelizing.

## Why a proxy

Token usage and the exact bytes sent and received are the same measurement for every agent, but no two agents expose them the same way, and some do not expose them at all, and not every agent speaks the same wire dialect. Putting a proxy on the network path makes the measurement the lab's job instead of the tool's. The tool points its base URL at the proxy; the proxy tees a copy of each request and response into the trace, translates whatever dialect the tool's SDK speaks into a chat-completions call, and forwards it upstream. Streaming replies keep streaming, because the proxy flushes as it copies rather than buffering. The `Authorization` header is never written, so a key a tool carried never lands in a trace.

## Keeping a run stable

A benchmark is only fair if a rerun means the same thing, and a hosted model is not naturally repeatable. Two general levers, neither tuned to a scenario, close most of the gap. The proxy forces greedy decoding onto every completion request (temperature 0, top_p 1, a fixed seed), so client-side sampling variance is gone and every tool is judged under one decoding regime. On top of that the harness runs up to `LAB_ATTEMPTS` tries and stops at the first pass, which soaks up the residual nondeterminism a model still shows even at temperature 0. A pass that needed more than one try is recorded as such, so the flakiness is measured rather than papered over.

## What gets measured

- **Correctness**: the scenario checker's exit code. It inspects files, runs the code the agent wrote, or diffs output against an expected value. It never reads the model's prose.
- **Tokens**: prompt, completion, and total, summed across every request in the run.
- **Memory**: peak resident set size, from GNU time wrapping the agent process.
- **Wall time**: measured by the orchestrator around the container, and by GNU time around the agent.
- **Disk**: the size of the working tree before and after, so a task's footprint on disk is visible.
- **Requests**: how many model calls the agent made to finish, a rough read on how much back-and-forth the task took.
- **Latency**: per call, the time to first byte and the total, averaged over the run's completions. The proxy times these, so the number is the same measurement for every tool. A 429 from the free tier's rate limit also carries a `Retry-After` header; the proxy reads it into `retry_after_s` on that call's latency row, so a rate-limited attempt shows up in the trace as a wait time instead of a bare failure.
- **Install footprint**: the tool image's on-disk size and the slice that is the tool itself sitting on the shared base, captured at build time.

See the full architecture and trace schema in [`docs/DESIGN.md`](https://github.com/tamnd/tomo-labs/blob/main/docs/DESIGN.md).
