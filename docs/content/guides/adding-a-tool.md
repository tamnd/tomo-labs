---
title: "Adding a tool"
description: "The two files a new coding agent needs to join the comparison: a Dockerfile on top of the shared base, and an adapter script the harness never looks past."
weight: 40
---

A tool is two files under `tools/<name>/`:

- `Dockerfile`, based on `tomolab-base` so the toolchain matches every other tool, that installs the agent and sets `adapter.sh` as the entrypoint.
- `adapter.sh`, the entrypoint, which the harness runs with these mounts and variables:

  - `/work`: the scenario's working tree and the agent's cwd, writable.
  - `/scenario`: the scenario definition, read-only. `prompt.txt` is the task. An optional `approvals` file holds a number for tools with an interactive gate to answer headlessly.
  - `/trace`: where stdout and the GNU time report go.
  - `LAB_BASE_URL`: the proxy. Point the agent's OpenAI-compatible base here.
  - `LAB_MODEL`, `OPENCODE_API_KEY`, `LAB_MAX_TURNS`.

The adapter runs the task non-interactively, lets the agent act (the container is the sandbox), and wraps the run in `/usr/bin/time -v -o /trace/time.txt` so peak memory comes back. `tools/tomo/adapter.sh` is the worked example.

The harness never reads a tool's own code, only these two files, so every tool is on the same footing.

## Wire dialects

A tool never talks to the real model directly. The trace proxy translates whatever dialect the agent's SDK speaks (OpenAI chat completions, Anthropic Messages, OpenAI Responses, or Gemini's API) into one chat-completions call upstream, using [`tamnd/tomo/pkg/wire`](https://github.com/tamnd/tomo/tree/main/pkg/wire). If a new tool speaks a dialect that translator doesn't cover yet, that's the one place to extend.

## After wiring it in

```bash
go run ./cmd/lab build
go run ./cmd/lab run <name>
go run ./cmd/lab meta
```

`lab meta` captures the tool's version and release date into `tool.json`, checked against its own npm or module registry rather than a version pinned by hand, so the [results](/guides/results/) table never drifts from what actually ran.
