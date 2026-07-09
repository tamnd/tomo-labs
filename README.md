# tomo-labs

A small harness for running an AI agent through real tasks and measuring how it
did, with every trace captured so a run can be inspected later, not just scored.

It runs each agent in a throwaway container, routes its model traffic through a
proxy that records request and response bodies and token usage, and grades the
work the agent left behind with a deterministic checker. tomo is the first tool
wired up. Others (openclaw, nanoclaw, whatever comes next) drop in as their own
folder under `tools/` and reuse everything else unchanged.

## What you need

- podman or docker. The harness detects which is present and uses it; set
  `LAB_RUNTIME` to force one. On this machine podman runs through the
  Apple-native `applehv` machine.
- A key for an OpenAI-compatible endpoint. The default targets the OpenCode Zen
  free tier, whose deepseek model does tool calling:

      export OPENCODE_API_KEY=...

## Use it

    ./lab.sh build            # build the base, proxy, and every wired tool image
    ./lab.sh run tomo         # run tomo through all ten scenarios
    ./lab.sh run tomo 03-bugfix-fizzbuzz   # or one scenario
    ./lab.sh report           # summarize pass rate, tokens, memory across runs

    ./lab.sh tools            # list wired tools
    ./lab.sh scenarios        # list scenarios

Every run writes under `$HOME/data/<tool>/<scenario>/<timestamp>/`:

    work/            the tree the agent worked in, exactly as it left it
    trace/
      config.yaml    the config the tool ran with
      requests.jsonl one line per model request, body included, key redacted
      resp-N.txt     the raw response for request N, streamed or not
      usage.jsonl    token usage per response
      stdout.log     what the tool printed
      time.txt       GNU time report, including peak memory
    result.json      the scored summary: passed, tokens, rss, wall, disk

## The ten scenarios

Ordinary tasks a capable agent should handle, each with a checker that grades
the result on disk rather than on what the model said:

| id | task |
| --- | --- |
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
points the tool at `$LAB_BASE_URL` and runs the task in `/work`.
