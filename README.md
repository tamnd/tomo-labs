# tomo-labs

A small harness for running an AI agent through real tasks and measuring how it
did, with every trace captured so a run can be inspected later, not just scored.

It runs each agent in a throwaway container, routes its model traffic through a
proxy that records request and response bodies and token usage, and grades the
work the agent left behind with a deterministic checker. Six tools are wired up
today: tomo, codex, opencode, claude-code, openclaw, hermes, and gemini-cli.
Each is its own folder under `tools/` and reuses everything else unchanged, so
adding one more is a Dockerfile and an adapter script, not a fork of the
harness.

The proxy speaks whatever wire each tool's SDK expects (OpenAI chat, the
Anthropic Messages API, the Responses API, or Gemini's API) and translates it
to one deterministic chat-completions call upstream, so every tool hits the
same free model through the same path and the comparison is fair.

The harness is a Go program. The `lab` command builds the images, runs the
scenarios, and reports; the same code is importable as a library under
`pkg/lab` if you want to drive a sweep from your own Go. The trace proxy is the
second binary, `cmd/proxy`, sharing the module.

## What you need

- Go 1.26.5 to build and run the harness.
- podman or docker. The harness detects which is present and uses it; set
  `LAB_RUNTIME` to force one. On this machine podman runs through the
  Apple-native `applehv` machine.
- A key for an OpenAI-compatible endpoint. The default targets the OpenCode Zen
  free tier, whose deepseek model does tool calling:

      export OPENCODE_API_KEY=...

## Use it

    go run ./cmd/lab build            # base, proxy, and every wired tool image
    go run ./cmd/lab run tomo         # run tomo through every scenario
    go run ./cmd/lab run tomo 03-bugfix-fizzbuzz   # or one scenario
    go run ./cmd/lab -p "explain this repo in one line"  # one ad-hoc prompt,
                                       # every tool, in parallel
    go run ./cmd/lab meta             # capture each tool's version and release date
    go run ./cmd/lab report           # summarize the captured runs as a table
    go run ./cmd/lab report --json    # the same summary as JSON

    go run ./cmd/lab tools            # list wired tools
    go run ./cmd/lab scenarios        # list scenarios

Install it as a binary with `go install ./cmd/lab` and call `lab` directly.

Two things keep a run from swinging on the model's luck, and both are general,
not tuned to any scenario. The proxy forces greedy decoding (temperature 0,
top_p 1, a fixed seed) onto every completion request, so a repeat run sees the
same sampling. On top of that the harness gives each scenario up to
`LAB_ATTEMPTS` tries (default 3) and stops at the first pass, which absorbs the
run-to-run nondeterminism a hosted model still shows even under greedy decoding.
`result.json` records how many tries a pass took, so flakiness stays visible
instead of hidden.

Every run writes under `$HOME/data/<tool>/<scenario>/<timestamp>/`:

    attempt-N/
      work/          the tree the agent worked in, exactly as it left it
      trace/
        config.yaml    the config the tool ran with
        requests.jsonl one line per model request, body included, key redacted
        resp-N.txt     the raw response for request N, streamed or not
        usage.jsonl    token usage per response
        latency.jsonl  per-call time to first byte and total
        stdout.log     what the tool printed
        time.txt       GNU time report, including peak memory
    result.json      the scored summary for the run: passed, attempts, tokens,
                     rss, latency, wall, disk, and install footprint

## Results so far

Six tools against the same free deepseek model through the same trace proxy,
so the differences below are the tools, not the model.
`lab report` reads every captured run, so a tool's row reflects its full
history, including scenarios it failed before an adapter fix, not just a
single clean sweep.

| tool | version | released | pass | avg tokens | avg ttfb | install |
| --- | --- | --- | --- | --- | --- | --- |
| tomo | v0.2.2-...c1a34b365454 | 2026-07-09 | 11/11 | 5,379 | 751ms | 21MB |
| codex | 0.143.0 | 2026-07-08 | 12/12 | 15,105 | 902ms | 423MB |
| opencode | 1.17.16 | 2026-07-09 | 11/11 | 26,227 | 820ms | 420MB |
| claude-code | 2.1.205 | 2026-07-08 | 12/13 | 63,747 | 1205ms | 322MB |
| openclaw | 2026.6.11 | 2026-06-30 | 11/11 | 56,234 | 1165ms | 407MB |
| hermes | 0.18.2 | 2026-07-08 | 14/24 | 25,834 | 1015ms | 221MB |
| gemini-cli | 0.50.0 | 2026-07-08 | 8/14 | 6,885 | 897ms | 181MB |

Run `lab report` for the full table (cache hit rate, cost, RSS, wall time), or
`lab report --json` for the raw numbers.

A few of these deserve a note.

Token use is the headline: tomo does the same tasks in a fraction of the
tokens of every other tool here, because it takes fewer, cleaner turns rather
than re-reading its own context on every step.

Install footprint, not image size, is the honest size axis.
Image size is dominated by the shared base every tool sits on (Python, Node, a
Go toolchain), so it says more about the base than the tool.
The install layer is the tool's own bytes on top of that base: 21MB for
tomo's single static binary against hundreds of megabytes for a Node
dependency tree.

Time to first byte is bounded by the hosted model, which is the same upstream
for every tool, so the gap you might hope for is not on the table here.
tomo is still fastest because its prompts are shorter, so the model spends
less time reading before it starts answering.

hermes and gemini-cli's pass counts include the runs recorded while their
adapters were still broken: hermes shipped a custom provider that dropped the
API key until its adapter learned to set it explicitly, and gemini-cli needs
`~/.gemini/settings.json` written with an explicit auth type or its headless
mode falls back to an interactive prompt that never resolves. Both wire
translators work end to end now (hermes passes its scenarios cleanly;
gemini-cli's remaining failures are the model missing a step, not a wiring
bug: it makes only 2-3 requests per scenario against 20-30 for tomo or
hermes, so it rarely retries the way the others do).

## The scenarios

Ordinary tasks a capable agent should handle, each with a checker that grades
the result on disk rather than on what the model said, plus a baseline
scenario that measures the fixed cost of one trivial round trip:

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

## Adding a tool

See `docs/DESIGN.md` for the architecture and the trace schema, and
`tools/openclaw/README.md` for the two files a new tool needs. The short
version: a `Dockerfile` on top of `tomolab-base`, and an `adapter.sh` that
points the tool at `$LAB_BASE_URL` and runs the task in `/work`. The harness
never reads a tool's code, only these two files, so every tool is on the same
footing.

## Layout

    cmd/lab      the harness CLI
    cmd/proxy    the trace proxy binary
    pkg/lab      the harness as a library: build, run, report
    pkg/proxy    the trace proxy as a library
    pkg/container a typed wrapper over the docker or podman CLI
    scenarios    one directory per task: prompt, fixtures, checker
    tools        one directory per tool: Dockerfile and adapter.sh
