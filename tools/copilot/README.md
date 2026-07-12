# copilot adapter

This folder plugs the GitHub Copilot CLI into the lab. Like every tool, it is
defined by exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-copilot`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  Copilot is installed from npm (package `@github/copilot`, binary `copilot`).
  The base already carries Node 22, which the CLI needs.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

Copilot has a BYOK mode that points the CLI at a custom OpenAI-compatible
provider through environment variables, and when a provider base URL is set the
CLI does not require GitHub authentication. That is the whole of the glue. What
the adapter does, in order:

1. It exports `COPILOT_PROVIDER_BASE_URL=$LAB_BASE_URL`,
   `COPILOT_PROVIDER_API_KEY`, `COPILOT_PROVIDER_TYPE=openai`,
   `COPILOT_PROVIDER_WIRE_API=completions`, and pins `COPILOT_MODEL` to the lab's
   fixed model. The base URL is the trace proxy, so copilot's request/response and
   token usage get captured with no cooperation from copilot, and because a custom
   provider is set the CLI never asks GitHub to log in. The proxy forwards to the
   real upstream with the real key.
2. `copilot -p "$prompt" --allow-all` runs the task headless in one shot.
   `--allow-all` turns on every permission at once (tools, paths, urls), copilot's
   equivalent of tomo's all-allow policy and what `-p` needs to run without
   stopping to confirm. `--no-color` and `--log-level none` keep the trace plain.
   Copilot works in the current directory, so `/work` is the tree the checker
   inspects. The whole run is wrapped in `/usr/bin/time -v` so the harness reads
   peak memory back.

To build and run:

    go run ./cmd/lab build copilot
    go run ./cmd/lab run copilot
