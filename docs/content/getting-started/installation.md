---
title: "Installation"
description: "Install tomo-labs from source and what it needs to run: Go, a container runtime, and a key for an OpenAI-compatible endpoint."
weight: 20
---

tomo-labs is a Go module with no published binary yet; build it from source or run it straight with `go run`.

## From source

```bash
git clone https://github.com/tamnd/tomo-labs
cd tomo-labs
go build -o bin/lab ./cmd/lab
```

Or skip the build and run it directly, which every example in these docs uses:

```bash
go run ./cmd/lab ...
```

## What it needs to run

- **Go 1.26.5** or newer.
- **podman or docker.** The harness detects whichever is present; set `LAB_RUNTIME` to force one.
- **A key for an OpenAI-compatible endpoint.** The default targets the OpenCode Zen free tier, whose deepseek model does tool calling:

  ```bash
  export OPENCODE_API_KEY=...
  ```

Nothing else is required. tomo-labs never talks to the real model directly; every agent under test points at the trace proxy, and the proxy is the only thing that reaches the upstream API.

Next: [the quick start](/getting-started/quick-start/).
