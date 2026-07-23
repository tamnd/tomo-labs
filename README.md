# tomo-labs

[![ci](https://github.com/tamnd/tomo-labs/actions/workflows/ci.yml/badge.svg)](https://github.com/tamnd/tomo-labs/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/tamnd/tomo-labs.svg)](https://pkg.go.dev/github.com/tamnd/tomo-labs)
[![Go Report Card](https://goreportcard.com/badge/github.com/tamnd/tomo-labs)](https://goreportcard.com/report/github.com/tamnd/tomo-labs)

**tomo-labs** puts coding agents through the same tasks on the same model and measures what actually happened, not what a leaderboard says happened. Every agent runs in its own throwaway container, every request and response it sends is captured verbatim, and every result is graded from the files it left on disk, not from what it claims to have done.

[Install](#install) • [Quick start](#quick-start) • [Results](#results) • [Scenarios](#the-scenarios) • [Adding a tool](#adding-a-tool) • [Docs](https://tomo-labs.tamnd.com/)

Agent benchmarks usually compare one thing everybody actually cares about (did it get the task done) by changing three things at once: the model, the prompt scaffolding, and the tool's own overhead. That is not a comparison, it is three experiments wearing one number. tomo-labs holds the model fixed. A trace proxy sits in front of every agent and forwards every request to the same free model with each agent's own sampling settings passed through untouched and recorded, whatever wire dialect the agent's SDK speaks, OpenAI chat, Anthropic Messages, OpenAI Responses, or Gemini's API. What is left to differ is the agent: how many turns it needs, how many tokens it burns getting there, how much memory it holds, how big its install is. Eleven agents run through the same harness today: tomo, codex, opencode, claude-code, openclaw, hermes, gemini-cli, pi, kilocode, aider, and copilot. Adding one more is a `Dockerfile` and a small adapter script, not a fork of the harness.

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

A run is kept from swinging on the model's luck without any per-scenario tuning, and without touching the model's sampling. The proxy passes every tool's decoding knobs through untouched and records them; an earlier version pinned greedy decoding (temperature 0, top_p 1, a fixed seed), but forcing a tool's sampling risks lowering its quality to buy an illusion of determinism, so that lever is gone. Honesty comes from the statistics instead: each scenario is scored on a single first-try attempt (pure pass@1), and claims aggregate over repeats, with pass rates as raw fractions over n and token columns as medians with spreads. An upstream fault (a dropped stream or a rate-limit) is re-issued off the books, so a gateway hiccup is never scored as the model failing; raising `LAB_ATTEMPTS` above 1 turns on opt-in best-of-N for anyone who wants to measure recovery instead. `result.json` records how many tries a pass took, so any retry stays visible.

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

## Publishing

Every run also mirrors itself to the public [open-index/tomo-traces](https://huggingface.co/datasets/open-index/tomo-traces) dataset: it converts the run's trace to the Hub's agent-trace format, regenerates the README and the reports from every result on disk, and commits the lot in one push, so no run's evidence is ever lost.

Publishing is on by default when `HF_TOKEN` is set and off otherwise, and `TOMO_LABS_PUBLISH=0` turns it off for a local experiment that should not touch the public dataset. It is best-effort and always the last step of a run: the run is already graded and recorded locally before the publish speaks to the network, so a publish failure never sinks the run, and the next publish picks the run back up from disk.

Because the dataset is public, every assembled file passes a secret gate before any byte is uploaded: the `HF_TOKEN`, the `OPENCODE_API_KEY` value, and bearer tokens are read from the environment only and never written to a trace, a log, or a commit, and a file that carried one blocks the whole commit.

```
lab publish --dry-run     assemble the commit and run the gate, upload nothing
lab publish --backfill    reconstruct and commit every local trace in one pass
lab publish               regenerate and commit the README and reports
```

## Results

Eight tools against the same free deepseek model through the same trace proxy, so what differs below is the tool, not the model. `lab report` splits them by how they work, the tools that lay out a plan or spawn a subagent apart from the ones that run a single flat loop, and keeps only the latest run of each scenario, so a tool's row is its current state over the same 14 scenarios, not a history that still counts runs it failed before an adapter bug got fixed. That is what makes the columns comparable: pass reads as N of the 14 scenarios, `plans` is how many of those it chose to plan on, and `tokens` is the total across all 14, so a bigger number means more work spent, not more runs recorded. Both tables are ordered by tokens. Cost prices those tokens at DeepSeek's published paid rates (the runs themselves were free), which is the dollar figure the token gap becomes once you leave the free tier.

Tools that plan:

| tool | version | pass | plans | tokens | cost | install |
| --- | --- | --- | --- | --- | --- | --- |
| tomo | v0.2.4 | 14/14 | 4/14 | 187,404 | $0.027 | 21MB |
| opencode | 1.17.18 | 12/14 | 2/14 | 457,807 | $0.052 | 420MB |
| codex | 0.144.1 | 14/14 | 3/14 | 732,370 | $0.066 | 423MB |
| openclaw | 2026.6.11 | 14/14 | 1/14 | 1,095,701 | $0.114 | 407MB |
| hermes | 0.18.2 | 14/14 | 3/14 | 1,168,925 | $0.106 | 221MB |
| claude-code | 2.1.207 | 14/14 | 3/14 | 1,793,716 | $0.150 | 322MB |

Tools that run flat:

| tool | version | pass | plans | tokens | cost | install |
| --- | --- | --- | --- | --- | --- | --- |
| gemini-cli | 0.50.0 | 5/14 | 0/14 | 112,988 | $0.011 | 181MB |
| pi | 0.80.6 | 14/14 | 0/14 | 244,455 | $0.033 | 156MB |

Every version above is that tool's latest published release as of the run, checked against its npm or module registry directly, not a stale pin. `lab meta` captures the version and its release date after every build so the table never drifts from what actually ran; run `lab report` yourself for the full columns (release dates, cache hit rate, average tokens, RSS, ttfb, wall time).

A few of these deserve a note.

Token use is the headline, and cost is the same story in dollars. Among the tools that plan, tomo does all 14 tasks in a fraction of the tokens: 187k total against 732k for codex, 1.10M for openclaw, and 1.79M for claude-code, which on the paid tier is under 3 cents against 7, 11, and 15. It plans in context, updating one checklist in the same turn, rather than re-reading its own state in a fresh context per step. pi spends more than tomo but still runs lean; gemini-cli spends the fewest tokens of all, but it does not plan at all and drops 9 of the 14 scenarios, so its cheapness is mostly work it never finished.

Planning is a choice a tool makes per scenario, not a fixed capability, which is what the `plans` column shows: even the planners lay out a plan on only a few of the 14 tasks and run the rest flat. openclaw is the clearest case, it carries a plan tool and a whole subagent layer but planned just 1 of 14 until the prompt asked, in plain terms, for a live plan it kept current as it worked. The split into two tables is by whether a tool ever plans, tomo and the others do on at least one scenario, pi and gemini-cli never.

Install footprint, not image size, is the honest size axis. Image size is dominated by the shared base every tool sits on (Python, Node, a Go toolchain), so it says more about the base than the tool. The install layer is the tool's own bytes on top of that base: 21MB for tomo's single static binary against 150 to 420MB for a Node dependency tree.

Time to first byte is left out of the tables on purpose. It is bounded by the hosted model, the same upstream for every tool, so it clusters in the same couple of seconds for everyone and is not a real axis of difference here; `lab report` still prints it if you want to see for yourself.

gemini-cli's 5/14 is mostly the model missing a step, not a wiring bug: it makes only 2 to 3 requests per scenario, so it rarely retries the way the others do, and it drops the multi-step scenarios where a plan would have kept it on track. Its wire translator works end to end. pi is the opposite kind of flat, a minimal harness that runs the whole task in one loop and passed every scenario cleanly. pi does ship a plan mode, but it is a read-only exploration extension gated behind an interactive prompt: it writes a prose plan and asks, in the TUI, whether to execute, rather than exposing a plan tool the model calls mid-run. In a one-shot headless run there is no prompt to answer and no plan tool to record, so pi stays flat here by design, not for lack of trying.

The `00-hello` scenario is a baseline, just the prompt `Hi!`, isolating the fixed round-trip cost every tool pays before it does any real work. All eleven wired tools clear it, each booting, authenticating through the proxy, and round-tripping the greeting on the fixed model. See the [Hi! baseline results](https://github.com/tamnd/tomo#the-hi-baseline) in tomo's own README for that table; it lives there since it's the number tomo's README leads with.

The eight tools in the sweep tables above have run the full 14 scenarios. kilocode, aider, and copilot are the three newest adapters, wired and validated on the `Hi!` baseline; their full sweep is pending. Their per-tool pages ([kilocode](https://tomo-labs.tamnd.com/tools/kilocode/), [aider](https://tomo-labs.tamnd.com/tools/aider/), [copilot](https://tomo-labs.tamnd.com/tools/copilot/)) trace the greeting run end to end and recover the system prompt each one actually sent.

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
   (/scenario)      (runs in /work)   (records +      (same model for
                                        translates       every tool)
                                        the wire)
                          │
                          ▼
                     work left in /work ─▶ checker ─▶ result.json
```

The proxy is the one piece every tool shares. It records every request and response verbatim, passes each tool's sampling settings through untouched, and translates whatever wire the tool's SDK speaks into one chat-completions call upstream, using the translators in [`tamnd/tomo/pkg/wire`](https://github.com/tamnd/tomo/tree/main/pkg/wire). A tool never talks to the real model directly, and never knows the proxy is anything other than the API it expects.

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
