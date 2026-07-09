# codex adapter

This folder plugs the OpenAI Codex CLI into the lab. Like every tool, it is
defined by exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-codex`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  It installs codex from npm (package `@openai/codex`, a prebuilt binary). The
  base carries Node 22, which the npm launcher needs.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

What the adapter does, in order:

1. It writes `~/.codex/config.toml` defining a custom `model_provider` named
   `lab` whose `base_url` is `$LAB_BASE_URL` with `wire_api = "responses"`, and
   reads the key from the `OPENCODE_API_KEY` env var via `env_key`. Recent codex
   only speaks the OpenAI Responses wire, but the free deepseek model speaks chat
   completions, so the proxy at that URL translates the Responses request into a
   chat request and the chat response back into a Responses stream. codex talks
   its native wire and never knows, and its request/response and token usage get
   captured with no cooperation from codex. The proxy forwards to the real
   upstream with the real key.
2. `codex exec --sandbox danger-full-access --skip-git-repo-check "$prompt"` runs
   the task headless in one shot. `exec` never stops for an approval;
   `danger-full-access` drops the sandbox so the agent can act freely, codex's
   equivalent of tomo's all-allow policy, safe here because the container is the
   sandbox; `--skip-git-repo-check` lets it run in `/work`, a plain tree rather
   than a git repo. The run is wrapped in `/usr/bin/time -v` so the harness reads
   peak memory back.

To build and run:

    go run ./cmd/lab build codex
    go run ./cmd/lab run codex
