---
title: "CLI reference"
description: "Every lab command and flag, and every environment variable it reads."
weight: 10
---

```
lab <command> [args]
```

`lab` is one binary with a small command set. All logic lives in `pkg/lab`; `cmd/lab` is a thin front end, so the same harness can be embedded as a library.

## build

```
lab build [tool]
```

Builds the shared base image, the trace proxy image, and every wired tool image. Pass a tool name to build just that one. Only needs to rerun after a `Dockerfile` changes.

## run

```
lab run [tool] [scenario]
```

Runs scenarios through the worker pool. With no arguments, every tool against every scenario. With a tool, that tool against every scenario. With both, just that pair.

```bash
go run ./cmd/lab run tomo                        # tomo, every scenario
go run ./cmd/lab run tomo 03-bugfix-fizzbuzz      # tomo, one scenario
```

## -p, --prompt, prompt

```
lab -p "<prompt>" [tool...]
```

Runs one ad-hoc prompt through every tool, or through the tools named after it, and prints a comparison. Goes through the same worker pool as a graded sweep, so its timing is representative.

```bash
go run ./cmd/lab -p "explain this repo in one line"
go run ./cmd/lab -p "explain this repo in one line" tomo codex
```

## tools

```
lab tools
```

Lists every wired tool, one per line.

## scenarios

```
lab scenarios
```

Lists every scenario with its one-line description.

## prompts

```
lab prompts <tool> [scenario] [--json] [--brief]
```

Recovers a tool's system prompt from its captured traces. It reads the request tap across every run in scope, unions the distinct system prompts, groups the per-run renderings that differ only in volatile spans like the date or a session id, and ranks the agent's working prompt, the one carrying a tool schema, first. Pass a scenario to scope to one; omit it to union every run. `--json` emits the structured form, the same shape the [prompts](/prompts/) pages are generated from; `--brief` keeps the per-prompt headers and drops the full text.

```bash
go run ./cmd/lab prompts tomo                 # every run, full text
go run ./cmd/lab prompts codex --brief         # headers only
go run ./cmd/lab prompts opencode --json       # structured, for regenerating a docs page
```

## gen

```
lab gen [--limit N] [--all] [--langs a,b] [--no-validate]
```

Materializes a public benchmark into the active suite's `tasks/` dir, chosen with the global `--suite` flag. It fetches the upstream benchmark, renders each problem into the harness task shape, and proves each task against a known-good solution before keeping it, so a task that cannot be validated never lands. `--limit N` takes N problems per track, `--all` takes the whole benchmark, `--langs a,b` selects language tracks for aider or datasets for evalplus, and `--no-validate` skips the reference-solution proof for a quick inspection. See [evals](/guides/evals/).

## meta

```
lab meta
```

Captures each wired tool's version and release date into `tool.json`, checked against the tool's own npm or module registry rather than a version pinned by hand. Run this after building a tool so the [results](/guides/results/) table never drifts from what actually ran.

## report

```
lab report [--json]
```

Reads every run ever captured for every tool and prints a comparison table: pass rate, tokens, latency, memory, install footprint, and more. `--json` prints the same summary as JSON instead of a table.

## reparse

```
lab reparse
```

Recomputes the metrics of every captured run from its stored trace, without rerunning any agent. Use it after a change to how a metric is derived, so old runs are scored the same way as new ones.

## clean

```
lab clean
```

Removes lab containers and dangling images left behind by builds and runs.

## --suite

```
lab <command> --suite <name>
```

Any command that runs, lists, reports, or generates over tasks takes `--suite` to select an eval tier instead of the core `scenarios/`. A suite reads its tasks from `evals/<name>/tasks/` and lands results in a separate tree, so a tier never mixes into the core report. `lab gen --suite <name>` materializes a tier; see [evals](/guides/evals/).

```bash
go run ./cmd/lab run tomo --suite aider
go run ./cmd/lab report --suite evalplus
```

## Environment

Every knob has an environment fallback, so a run reproduces regardless of which front end starts it.

| Variable | Default | Meaning |
|---|---|---|
| `OPENCODE_API_KEY` | | Upstream key, forwarded to the tool under test, never written to a trace. |
| `LAB_MODEL` | `deepseek-v4-flash-free` | Bare upstream model id. |
| `LAB_UPSTREAM` | `https://opencode.ai/zen` | OpenAI-compatible base the proxy forwards to. |
| `LAB_DATA` | `$HOME/data` | Where traces and results land, per tool/scenario/timestamp. |
| `LAB_ROOT` | repo root | Root holding `scenarios/` and `tools/`. |
| `LAB_MAX_TURNS` | `12` | Agent turn budget handed to the tool. |
| `LAB_ATTEMPTS` | `3` | Best-of-N: how many tries before a scenario is called failed. |
| `LAB_PROXY_PORT` | `8899` | Host port the first worker's proxy publishes; later workers take the next ports. |
| `LAB_KEEP_RUNS` | `5` | How many timestamped runs to keep per tool/scenario. `0` keeps all. |
| `LAB_CONCURRENCY` | `3` | How many tool/scenario runs to keep in flight at once. |
| `LAB_RUNTIME` | auto-detected | Force `docker` or `podman` instead of detecting which is present. |
| `LAB_DETERMINISTIC` | `1` | Whether the proxy forces greedy decoding onto every completion request. |

Nothing here is invented. If a flag or variable is not on this page, `lab` does not read it.
