# pi adapter

This folder plugs pi into the lab. Like every tool, it is defined by exactly two
files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-pi`, based on `tomolab-base`
  so it shares the same toolchain every other tool runs against. It installs pi
  from npm (package `@earendil-works/pi-coding-agent`, binary `pi`) with
  `--ignore-scripts`, the install pi's own docs prescribe. The base carries Node
  22, which pi needs.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

What the adapter does, in order:

1. It writes `~/.pi/agent/models.json` registering a custom provider named `lab`
   whose `baseUrl` is `$LAB_BASE_URL` and whose `api` is `openai-completions`,
   pi's OpenAI chat-completions client. That URL is the trace proxy, so pi's
   request/response and token usage get captured with no cooperation from pi. The
   `apiKey` stays the literal `$OPENCODE_API_KEY`, which pi interpolates from the
   environment at run time, so the real key never lands in the config copy in
   `/trace`. The proxy forwards to the real upstream with it.
2. `pi -p "$prompt" --model lab/$LAB_MODEL -a` runs the task headless in one
   shot and exits. pi has no sandbox of its own and does not pause to approve a
   shell command or a file write, so its built-in tools run freely, pi's
   equivalent of tomo's all-allow policy, the container being the sandbox. `-a`
   trusts the project-local tree for the run so nothing stops on a trust check.
   The whole run is wrapped in `/usr/bin/time -v` so the harness reads peak
   memory back.

To build and run:

    go run ./cmd/lab build pi
    go run ./cmd/lab run pi
