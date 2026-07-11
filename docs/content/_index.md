---
title: "tomo-labs"
description: "tomo-labs runs coding agents through the same tasks on the same model and measures what actually happened. A trace proxy holds the model fixed across tomo, codex, opencode, claude-code, openclaw, hermes, gemini-cli, and pi, and every result is graded from the files an agent left on disk."
heroTitle: "The model held fixed, the agent left to differ"
heroLead: "Agent benchmarks usually compare one number by changing three things at once: the model, the prompt scaffolding, and the tool's own overhead. tomo-labs holds the model fixed. A trace proxy forwards every request from every agent to the same free model with the same deterministic decoding, whatever wire dialect the agent's SDK speaks, and every result is graded from the files it left on disk, not from what it claims to have done."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

Eight agents run through the same harness today: tomo, codex, opencode, claude-code, openclaw, hermes, gemini-cli, and pi.
Every one runs in its own throwaway container, every request and response it sends is captured verbatim, and adding one more agent is a `Dockerfile` and a small adapter script, not a fork of the harness.

```bash
go run ./cmd/lab build   # base, proxy, and every wired tool image
go run ./cmd/lab run tomo   # run tomo through every scenario
go run ./cmd/lab report   # summarize every captured run as a table
```

## What it measures

- **Correctness**, from a checker that grades the work left on disk, never the model's own account of what it did.
- **Tokens, memory, wall time, and disk**, the same measurement for every agent because the proxy and the harness take it, not the tool.
- **Install footprint**, the tool's own bytes on top of the shared base image, a real cost most benchmarks never show.
- **Time to first byte and latency**, timed by the proxy on the same upstream call for every tool.

## Where to go next

- New here?
  Start with the [overview](/overview/) for the whole feature set in one place, then the [installation](/getting-started/installation/) guide and the [quick start](/getting-started/quick-start/).
- Studying an agent?
  The [tools](/tools/) pages are a deep dive on each one: command surface, architecture, captured system prompt, and a Hi! run traced end to end.
- Want the numbers?
  See [results](/guides/results/) for the full comparison table and the `00-hello` baseline.
- Curious what each agent actually sends?
  The [prompts](/prompts/) pages carry every wired agent's real system prompt, recovered from the trace and versioned so drift shows in the diff.
- Adding an agent to the comparison?
  [Adding a tool](/guides/adding-a-tool/) covers the two files a new agent needs.
- Need the exact command surface?
  The [CLI reference](/reference/cli/) lists every `lab` command and flag.
