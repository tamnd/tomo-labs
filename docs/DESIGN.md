# Design

tomo-labs answers one question honestly: given a real task and a real model,
did the agent get it done, and what did it cost. Honesty here means two things.
The grade comes from the work left on disk, not from the model's own account of
what it did. And every request, response, and resource number is captured, so a
surprising result can be opened up instead of taken on faith.

The whole thing is built so the agent under test is the only thing that changes
between runs. Same container base, same toolchain, same network path to the
model, same scoring. Swap tomo for another agent and the only new code is how to
launch that agent.

## The pieces

    cmd/lab               the harness CLI, and the thing you run
    cmd/proxy             the trace proxy binary
    pkg/lab               the harness as a library: build, run, report
    pkg/proxy             the trace tap: a reverse proxy that records traffic
    pkg/container         a typed wrapper over the docker or podman CLI
    tools/base/           the shared runtime image every tool builds on
    tools/<tool>/         one tool: a Dockerfile and an adapter.sh
    scenarios/<id>/       one task: prompt.txt, setup.sh, check.sh
    webroot/              static pages served to scenarios that fetch

The harness is one Go module. `pkg/lab` holds all the orchestration logic and
`cmd/lab` is a thin front end over it, so a sweep can be driven from Go directly
as well as from the command line. `pkg/container` shells out to whichever
runtime is present rather than pulling in a container SDK, which keeps the module
dependency-free and every runtime call bounded by a context.

### A run, start to finish

For one (tool, scenario) pair, `RunOne` does this:

1. Make `$HOME/data/<tool>/<scenario>/<timestamp>/`. Each try gets its own
   `attempt-N/` with `work/` and `trace/` under it.
2. Run the scenario's `setup.sh` on the host to lay fixtures into `work/`.
3. Start the trace proxy with `trace/` mounted in, and wait for it to answer.
4. If the scenario needs a web page, start the static sidecar too.
5. Run the tool container: `work/` as its cwd, `scenario/` read-only, `trace/`
   for output, and `LAB_BASE_URL` pointed at the proxy. The tool's `adapter.sh`
   runs the task under `/usr/bin/time -v`.
6. Stop the sidecars.
7. Run the scenario's `check.sh` on the host against `work/`, which passes or
   fails by exit code.
8. If it failed and tries remain, go back to step 1's next `attempt-N/`. If it
   passed, or the last try is spent, stop.
9. Read the numbers back out of the last try's `trace/` and write `result.json`
   at the run root, recording how many tries the pass took.

Sidecars are per run, so one run's trace never bleeds into another's, and a
crashed container never blocks the next run.

### Keeping a run stable

A benchmark is only fair if a rerun means the same thing, and a hosted model is
not naturally repeatable. Two general levers, neither one tuned to a scenario,
close most of the gap. The proxy forces greedy decoding onto every completion
request (temperature 0, top_p 1, a fixed seed), so client-side sampling variance
is gone and every tool is judged under one decoding regime. On top of that the
harness runs up to `LAB_ATTEMPTS` tries and stops at the first pass, which soaks
up the residual nondeterminism a model still shows even at temperature 0. A pass
that needed more than one try is recorded as such, so the flakiness is measured
rather than papered over.

### Why a proxy

Token usage and the exact bytes sent and received are the same measurement for
every agent, but no two agents expose them the same way, and some do not expose
them at all. Putting a proxy on the network path makes the measurement the
lab's job instead of the tool's. The tool points its OpenAI-compatible base URL
at the proxy, the proxy forwards to the real upstream, and it tees a copy of
each request and response into the trace as it goes. Streaming replies keep
streaming, because the proxy flushes as it copies rather than buffering. The
`Authorization` header is never written, so a key a tool carried never lands in
a trace.

Token counts come from the `usage` block in the reply, read from the last one
seen in a streamed response. If a tool does not ask its endpoint for usage, the
count may be zero, but the raw request and response are still in the trace, so
the number can be recomputed later. Nothing is lost.

### What gets measured

- Correctness: the scenario checker's exit code. It inspects files, runs the
  code the agent wrote, or diffs output against an expected value. It never
  reads the model's prose.
- Tokens: prompt, completion, and total, summed across every request in the run.
- Memory: peak resident set size, from GNU time wrapping the agent process.
- Wall time: measured by the orchestrator around the container, and by GNU time
  around the agent.
- Disk: the size of the working tree before and after, so a task's footprint on
  disk is visible.
- Requests: how many model calls the agent made to finish, a rough read on how
  much back-and-forth the task took.
- Latency: per call, the time to first byte and the total, averaged over the
  run's completions. The proxy times these, so the number is the same
  measurement for every tool.
- Install footprint: the tool image's on-disk size and the slice that is the
  tool itself sitting on the shared base, captured at build time. A heavier
  agent is a real cost, so it is an axis of the comparison.

## The trace schema

`result.json` is the summary a reader starts from:

    {
      "tool": "tomo",
      "scenario": "10-reasoning-calc",
      "timestamp": "20260709T081417Z",
      "model": "opencode/deepseek-v4-flash-free",
      "runtime": "podman",
      "passed": true,
      "exit_code": 0,
      "attempts": 1,
      "attempts_max": 3,
      "wall_seconds": 8,
      "elapsed_clock": "0:07.13",
      "max_rss_kb": 12896,
      "requests": 4,
      "tokens": { "prompt": 3363, "completion": 384, "total": 3747 },
      "latency_ms": { "avg_ttfb": 706, "avg_total": 1974, "calls": 4 },
      "disk_before_kb": 0,
      "disk_after_kb": 8,
      "disk_delta_kb": 8,
      "install_kb": 21831,
      "image_kb": 936845,
      "check": "final number is correct"
    }

The full detail sits beside it in `trace/`, keyed by a sequence number so a
request in `requests.jsonl`, its reply in `resp-N.txt`, and its usage in
`usage.jsonl` all line up.

## Adding a tool

A tool is two files under `tools/<name>/`:

- `Dockerfile`, based on `tomolab-base` so the toolchain matches every other
  tool, that installs the agent and sets `adapter.sh` as the entrypoint.
- `adapter.sh`, the entrypoint, which the harness runs with these mounts and
  variables:

  - `/work`: the scenario's working tree and the agent's cwd, writable.
  - `/scenario`: the scenario definition, read-only. `prompt.txt` is the task.
    An optional `approvals` file holds a number for tools with an interactive
    gate to answer headlessly.
  - `/trace`: where stdout and the GNU time report go.
  - `LAB_BASE_URL`: the proxy. Point the agent's OpenAI-compatible base here.
  - `LAB_MODEL`, `OPENCODE_API_KEY`, `LAB_MAX_TURNS`.

The adapter runs the task non-interactively, lets the agent act (the container
is the sandbox), and wraps the run in `/usr/bin/time -v -o /trace/time.txt` so
peak memory comes back. `tools/tomo/adapter.sh` is the worked example.

## Adding a scenario

A scenario is a folder under `scenarios/` with:

- `prompt.txt`: the task, phrased as a user would ask it. The agent's cwd is the
  working tree, so refer to plain filenames.
- `setup.sh <workdir>`: lays fixtures into the working tree on the host.
- `check.sh <workdir>`: grades the result, pass or fail by exit code, with a
  one-line reason on stdout.
- `desc`: a one-line summary for listings.
- optional `web`: presence starts the static web sidecar for this scenario.
- optional `approvals`: a number the tomo adapter uses to answer gate prompts.

Keep the checker deterministic and keep it honest: grade the artifact, not the
chatter. Every checker in this repo passes on a correct solution and fails on
the untouched starting state, and both directions are worth testing when you add
one.
