# opencode adapter

This folder plugs opencode into the lab. Like every tool, it is defined by
exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-opencode`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  It installs opencode from npm (package `opencode-ai`, binary `opencode`). The
  base carries Node 22, which opencode needs.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

What the adapter does, in order:

1. It writes `~/.config/opencode/opencode.json` registering a custom
   OpenAI-compatible provider named `lab` whose `baseURL` is `$LAB_BASE_URL`, via
   the `@ai-sdk/openai-compatible` package opencode fetches at first run. That
   URL is the trace proxy, so opencode's request/response and token usage get
   captured with no cooperation from opencode. The proxy forwards to the real
   upstream with the real key.
2. `opencode run --model lab/$LAB_MODEL --dir /work --auto "$prompt"` runs the
   task headless in one shot. `--auto` approves every permission the run does not
   explicitly deny, opencode's equivalent of tomo's all-allow policy, so the
   shell scenarios run without a prompt. `--dir /work` pins the working tree to
   the exact tree the checker inspects. The whole run is wrapped in
   `/usr/bin/time -v` so the harness reads peak memory back.

To build and run:

    go run ./cmd/lab build opencode
    go run ./cmd/lab run opencode
