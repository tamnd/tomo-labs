# aider adapter

This folder plugs aider into the lab. Like every tool, it is defined by exactly
two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-aider`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  aider is a Python program, installed from PyPI (package `aider-chat`, binary
  `aider`). The base already carries Python 3 and pip. Debian marks its Python as
  externally managed, so the install passes `--break-system-packages` to write
  into the image's own site-packages, which is what we want in a throwaway
  container.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

aider talks to any OpenAI-compatible endpoint through litellm, so the glue is
just an env pair and a model name. What the adapter does, in order:

1. It exports `OPENAI_API_BASE=$LAB_BASE_URL` and `OPENAI_API_KEY`, and names the
   model `openai/$LAB_MODEL`. litellm routes an `openai/`-prefixed model to its
   OpenAI handler, which reads those two variables, so every request/response and
   token count flows through the trace proxy and gets captured with no
   cooperation from aider. The proxy forwards to the real upstream with the real
   key.
2. `aider --message "$prompt"` runs the task headless in one shot. `--yes-always`
   answers every confirmation with yes, aider's equivalent of tomo's all-allow
   policy, so it creates and edits files without stopping to ask. `--no-git`
   keeps aider out of version control, since `/work` is a plain tree rather than a
   repo. `--no-show-model-warnings`, `--no-check-update`, and `--no-analytics`
   stop it prompting about the unknown model or phoning home, and its own chat and
   input history go to `/trace` rather than the tree the checker inspects. The
   whole run is wrapped in `/usr/bin/time -v` so the harness reads peak memory
   back.

To build and run:

    go run ./cmd/lab build aider
    go run ./cmd/lab run aider
