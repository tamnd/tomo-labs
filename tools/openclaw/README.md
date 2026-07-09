# openclaw adapter (placeholder)

This folder is where openclaw plugs into the lab. Nothing here is wired yet,
because openclaw is not on this machine and there is no source to build from.
When you have it, filling this in is the whole job, and the rest of the lab
(scenarios, trace capture, resource accounting, scoring) works unchanged.

A tool is defined by exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-openclaw`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  Copy openclaw's binary (or `pip install` / `npm i -g` it) and set the
  adapter as the entrypoint.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

The adapter's contract, which the harness relies on:

1. Point openclaw's OpenAI-compatible base URL at `$LAB_BASE_URL`. That URL is
   the trace proxy, so its request/response and token usage get captured with
   no cooperation from openclaw itself.
2. Run the task in `/scenario/prompt.txt` non-interactively, with `/work` as the
   working directory, letting the agent act autonomously (the container is the
   sandbox).
3. Wrap the run in `/usr/bin/time -v -o /trace/time.txt` so the harness can read
   peak memory back, and send stdout to `/trace/stdout.log`.

See `tools/tomo/adapter.sh` for a worked example. Copy it, swap the tomo
invocation for openclaw's, and drop the tomo-specific approval handling if
openclaw has no interactive gate. Then:

    ./lab.sh build openclaw
    ./lab.sh run openclaw
