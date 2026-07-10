# tomo-labs

[![Go Reference](https://pkg.go.dev/badge/github.com/tamnd/tomo-labs.svg)](https://pkg.go.dev/github.com/tamnd/tomo-labs)
[![Go Report Card](https://goreportcard.com/badge/github.com/tamnd/tomo-labs)](https://goreportcard.com/report/github.com/tamnd/tomo-labs)

**tomo-labs** puts coding agents through the same tasks on the same model and measures what actually happened, not what a leaderboard says happened. Every agent runs in its own throwaway container, every request and response it sends is captured verbatim, and every result is graded from the files it left on disk, not from what it claims to have done.

[Install](#install) • [Quick start](#quick-start) • [Results](#results) • [Scenarios](#the-scenarios) • [Adding a tool](#adding-a-tool) • [Docs](https://tomo-labs.tamnd.com/)

Agent benchmarks usually compare one thing everybody actually cares about (did it get the task done) by changing three things at once: the model, the prompt scaffolding, and the tool's own overhead. That is not a comparison, it is three experiments wearing one number. tomo-labs holds the model fixed. A trace proxy sits in front of every agent and forwards every request to the same free model with the same deterministic decoding settings, whatever wire dialect the agent's SDK speaks, OpenAI chat, Anthropic Messages, OpenAI Responses, or Gemini's API. What is left to differ is the agent: how many turns it needs, how many tokens it burns getting there, how much memory it holds, how big its install is. Eight agents run through the same harness today: tomo, codex, opencode, claude-code, openclaw, hermes, gemini-cli, and pi. Adding one more is a `Dockerfile` and a small adapter script, not a fork of the harness.

## Install

```sh
git clone https://github.com/tamnd/tomo-labs
cd tomo-labs
go build -o bin/lab ./cmd/lab
```

Or run it straight from source with `go run ./cmd/lab ...`, which every example below uses.

You'll need:

- Go 1.26.5
- podman or docker (the harness detects which is present; set `LAB_RUNTIME` to force one)
- a key for an OpenAI-compatible endpoint. The default targets the OpenCode Zen free tier, whose deepseek model does tool calling:

  ```sh
  export OPENCODE_API_KEY=...
  ```

## Quick start

```sh
go run ./cmd/lab build            # base, proxy, and every wired tool image
go run ./cmd/lab run tomo         # run tomo through every scenario
go run ./cmd/lab report           # summarize every captured run as a table
```

That's the whole loop: build the images once, run whichever agent you want against whichever scenario you want, then read the report. A few more useful shapes:

```sh
go run ./cmd/lab run tomo 03-bugfix-fizzbuzz     # just one scenario
go run ./cmd/lab -p "explain this repo in one line"   # one ad-hoc prompt, every
                                                        # tool, in parallel
go run ./cmd/lab meta                             # capture each tool's version
                                                    # and release date
go run ./cmd/lab report --json                    # the same summary as JSON
go run ./cmd/lab tools                            # list wired tools
go run ./cmd/lab scenarios                        # list scenarios
```

Two things keep a run from swinging on the model's luck, and neither is tuned to any one scenario. The proxy forces greedy decoding (temperature 0, top_p 1, a fixed seed) onto every completion request, so a repeat run sees the same sampling. On top of that the harness gives each scenario up to `LAB_ATTEMPTS` tries (default 3) and stops at the first pass, absorbing the run-to-run nondeterminism a hosted model still shows even under greedy decoding. `result.json` records how many tries a pass took, so flakiness stays visible instead of hidden.

Runs go through a worker pool, `LAB_CONCURRENCY` deep (default 3), each with its own proxy container and port, so a full sweep is bounded by the slowest few runs rather than the sum of all of them.

## What a run leaves behind

Every run writes under `$HOME/data/<tool>/<scenario>/<timestamp>/`:

```
attempt-N/
  work/            the tree the agent worked in, exactly as it left it
  trace/
    config.yaml      the config the tool ran with
    requests.jsonl    one line per model request, body included, key redacted
    resp-N.txt        the raw response for request N, streamed or not
    usage.jsonl       token usage per response
    latency.jsonl     per-call time to first byte and total
    stdout.log        what the tool printed
    time.txt          GNU time report, including peak memory
result.json        the scored summary: passed, attempts, tokens, rss,
                   latency, wall time, disk, install footprint
```

Nothing is summarized away. If a number in the report table looks wrong, the request that produced it is sitting right there in plain text.

## Results

Eight tools against the same free deepseek model through the same trace proxy, so what differs below is the tool, not the model. `lab report` splits them by how they work, the tools that lay out a plan or spawn a subagent apart from the ones that run a single flat loop, and reads every run ever captured, so a tool's row is its full history, including scenarios it failed before an adapter bug got fixed, not just one clean sweep. Both tables are ordered by average tokens.

Tools that plan:

| tool | version | released | pass | avg tokens | avg ttfb | install |
| --- | --- | --- | --- | --- | --- | --- |
| tomo | v0.2.3 | 2026-07-10 | 17/17 | 20,587 | 3347ms | 21MB |
| codex | 0.143.0 | 2026-07-08 | 19/26 | 35,076 | 1734ms | 423MB |
| opencode | 1.17.16 | 2026-07-09 | 18/20 | 47,587 | 1558ms | 420MB |
| hermes | 0.18.2 | 2026-07-08 | 15/15 | 86,260 | 3003ms | 221MB |
| openclaw | 2026.6.11 | 2026-06-30 | 18/18 | 92,153 | 2450ms | 407MB |
| claude-code | 2.1.205 | 2026-07-08 | 26/26 | 104,252 | 2127ms | 322MB |

Tools that run flat:

| tool | version | released | pass | avg tokens | avg ttfb | install |
| --- | --- | --- | --- | --- | --- | --- |
| gemini-cli | 0.50.0 | 2026-07-08 | 6/16 | 7,572 | 2092ms | 181MB |
| pi | — | — | 17/17 | 15,685 | 2994ms | 156MB |

Every version above is that tool's latest published release as of the run, checked against its npm/module registry directly, not a stale pin. `lab meta` captures the version and release date after every build so the table never drifts from what actually ran; run `lab report` yourself for the full columns (cache hit rate, cost, RSS, wall time).

A few of these deserve a note.

Token use is the headline. Among the tools that plan, tomo does the same tasks in a fraction of the tokens: 21k on average against 35k for codex, 92k for openclaw, and 104k for claude-code. It plans in context, updating one checklist in the same turn, rather than re-reading its own state in a fresh context per step. pi and gemini-cli spend fewer tokens still, but they do not plan at all, they run the task in one flat loop, which is cheaper on the easy scenarios and part of why gemini-cli falls apart on the multi-step ones.

Planning is a choice a tool makes per run, not a fixed capability. openclaw carries a plan tool and a whole subagent layer but ran several scenarios flat until the prompt asked, in plain terms, for a live plan it kept current as it worked. The split above is by what a tool did on a run, not by what it could do.

Install footprint, not image size, is the honest size axis. Image size is dominated by the shared base every tool sits on (Python, Node, a Go toolchain), so it says more about the base than the tool. The install layer is the tool's own bytes on top of that base: 21MB for tomo's single static binary against 150 to 420MB for a Node dependency tree.

Time to first byte is bounded by the hosted model, the same upstream for every tool, so it clusters in the same couple of seconds for everyone and is not a real axis of difference here.

gemini-cli's 6/16 is mostly the model missing a step, not a wiring bug: it makes only 2 to 3 requests per scenario, so it rarely retries the way the others do, and it drops the multi-step scenarios where a plan would have kept it on track. Its wire translator works end to end. pi is the opposite kind of flat, a minimal harness that runs the whole task in one loop and passed every scenario cleanly.

The `00-hello` scenario is a baseline, just the prompt `Hi!`, isolating the fixed round-trip cost every tool pays before it does any real work. See the [Hi! baseline results](https://github.com/tamnd/tomo#the-hi-baseline) in tomo's own README for that table; it lives there since it's the number tomo's README leads with.

## The scenarios

Ordinary tasks a capable agent should handle, each with a checker that grades the result on disk rather than on what the model said, plus the `00-hello` baseline above:

| id | task |
| --- | --- |
| 00-hello | say hi, no task beyond completing the round trip |
| 01-file-organize | sort a flat pile of files into folders by extension |
| 02-json-transform | filter and sort a JSON array of users |
| 03-bugfix-fizzbuzz | fix a FizzBuzz that never prints FizzBuzz |
| 04-web-extract | fetch a page and name the cheapest product |
| 05-log-count | count HTTP 500s in an access log |
| 06-codegen-primes | write, build, and run a Go primes program |
| 07-refactor-dedupe | remove a duplicated function, keep the test green |
| 08-data-summary | total a sales CSV and find the top day |
| 09-project-scaffold | scaffold a small project and run its make target |
| 10-reasoning-calc | follow a precise two-step calculation into a file |
| 11-storefront-budget | fetch a page, read a local budget, and write the affordable products in order |
| 12-invoice-join | fetch a catalog, join it with a local orders CSV, and total the invoice |
| 13-release-fix | fetch tax rates, fix a bug, build, run, and get the Go test suite green |

## Adding a tool

See [`docs/DESIGN.md`](docs/DESIGN.md) for the architecture and the trace schema, and [`tools/openclaw/README.md`](tools/openclaw/README.md) for the two files a new tool needs: a `Dockerfile` on top of `tomolab-base`, and an `adapter.sh` that points the tool at `$LAB_BASE_URL` and runs the task in `/work`. The harness never reads a tool's own code, only these two files, so every tool is on the same footing.

## How it works

```
scenario prompt ─▶ tool container ─▶ trace proxy ─▶ upstream model
   (/scenario)      (runs in /work)   (records +      (deterministic,
                                        translates       same for
                                        the wire)         every tool)
                          │
                          ▼
                     work left in /work ─▶ checker ─▶ result.json
```

The proxy is the one piece every tool shares. It records every request and response verbatim, forces deterministic decoding, and translates whatever wire the tool's SDK speaks into one chat-completions call upstream, using the translators in [`tamnd/tomo/pkg/wire`](https://github.com/tamnd/tomo/tree/main/pkg/wire). A tool never talks to the real model directly, and never knows the proxy is anything other than the API it expects.

## Layout

```
cmd/lab         the harness CLI
cmd/proxy       the trace proxy binary
pkg/lab         the harness as a library: build, run, report
pkg/proxy       the trace proxy as a library
pkg/container   a typed wrapper over the docker or podman CLI
scenarios/      one directory per task: prompt, fixtures, checker
tools/          one directory per tool: Dockerfile and adapter.sh
docs/           DESIGN.md, the architecture and trace schema in full
```
