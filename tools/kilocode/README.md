# kilocode adapter

This folder plugs Kilo Code into the lab. Like every tool, it is defined by
exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-kilocode`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  It installs Kilo Code from npm (package `@kilocode/cli`, binary `kilo`). The
  npm package is a thin launcher whose postinstall pulls the prebuilt binary for
  the image's platform out of an optional dependency. The base carries Node 22,
  which the installer needs.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

Kilo Code is an opencode fork, so the glue matches opencode's next door. What the
adapter does, in order:

1. It writes `~/.config/kilo/kilo.json` registering a custom OpenAI-compatible
   provider named `lab` whose `baseURL` is `$LAB_BASE_URL`, via the
   `@ai-sdk/openai-compatible` package kilo fetches at first run. That URL is the
   trace proxy, so kilo's request/response and token usage get captured with no
   cooperation from kilo. The proxy forwards to the real upstream with the real
   key.
2. `kilo run --model lab/$LAB_MODEL --dir /work --auto "$prompt"` runs the task
   headless in one shot. `--auto` approves every permission the run does not
   explicitly deny, kilo's equivalent of tomo's all-allow policy, so the shell
   scenarios run without a prompt. `--dir /work` pins the working tree to the
   exact tree the checker inspects. The whole run is wrapped in `/usr/bin/time -v`
   so the harness reads peak memory back.

To build and run:

    go run ./cmd/lab build kilocode
    go run ./cmd/lab run kilocode
