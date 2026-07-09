# hermes adapter

This folder plugs Hermes Agent into the lab. Like every tool, it is defined by
exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-hermes`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  It installs `hermes-agent` from npm. That npm package is an unofficial bridge
  whose postinstall step pip installs the upstream Python Hermes Agent by Nous
  Research, so the real agent is a Python runtime the bridge launches. The base
  carries Node 22 and Python 3.11, which both need, and the image sets
  `PIP_BREAK_SYSTEM_PACKAGES=1` so the bridge's pip install lands on Debian
  bookworm's externally managed Python.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

What the adapter does, in order:

1. It points Hermes at a custom OpenAI-compatible endpoint with
   `hermes config set model.provider custom`, `model.base_url $LAB_BASE_URL`, and
   `model.default $LAB_MODEL`, and exports the key as `OPENAI_API_KEY`. That URL
   is the trace proxy, so Hermes' request/response and token usage get captured
   with no cooperation from Hermes itself. The proxy forwards to the real
   upstream with the real key.
2. It sets `HERMES_YOLO_MODE` and passes `--yolo` so the agent auto-approves
   every action and runs headless. The container is the sandbox, so the agent
   acts autonomously, which is Hermes' equivalent of tomo's all-allow policy.
3. `hermes chat -q "$prompt"` runs the task in single-query mode with `/work` as
   the working directory, wrapped in `/usr/bin/time -v` so the harness reads peak
   memory back.

To build and run:

    go run ./cmd/lab build hermes
    go run ./cmd/lab run hermes
