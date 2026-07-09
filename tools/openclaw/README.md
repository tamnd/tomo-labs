# openclaw adapter

This folder plugs openclaw into the lab. Like every tool, it is defined by
exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-openclaw`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  It installs openclaw from npm (`npm install -g openclaw`). The base carries
  Node 22, which openclaw needs.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

What the adapter does, in order:

1. `openclaw setup --non-interactive --accept-risk` lays down the baseline
   config and workspace.
2. It registers an OpenAI-compatible provider named `lab` whose `baseUrl` is
   `$LAB_BASE_URL`. That URL is the trace proxy, so openclaw's request/response
   and token usage get captured with no cooperation from openclaw itself. The
   proxy forwards to the real upstream with the real key.
3. `openclaw exec-policy preset yolo` grants full exec with no approval prompts.
   The container is the sandbox, so the agent acts autonomously. This is
   openclaw's equivalent of tomo's all-allow policy, and it lets the shell
   scenarios (build a Go program, run node, drive make) run headless.
4. `openclaw agent --local --json` runs the task in `/scenario/prompt.txt` with
   `/work` as the working directory, wrapped in `/usr/bin/time -v` so the
   harness reads peak memory back.

To build and run:

    ./lab.sh build openclaw
    ./lab.sh run openclaw
