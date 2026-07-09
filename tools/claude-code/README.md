# claude-code adapter

This folder plugs Anthropic's Claude Code CLI into the lab. Like every tool, it
is defined by exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-claude-code`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  It installs Claude Code from npm (package `@anthropic-ai/claude-code`, binary
  `claude`). The base carries Node 22, which the CLI needs.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

What the adapter does, in order:

1. It points Claude Code at the trace proxy as if it were the Anthropic API.
   Claude Code appends `/v1/messages` to `ANTHROPIC_BASE_URL`, so the adapter
   strips the trailing `/v1` off `$LAB_BASE_URL`. The free deepseek model speaks
   chat completions, not the Anthropic Messages wire, so the proxy translates the
   Messages request into a chat request and the chat response back into a Messages
   stream. Claude Code talks its native wire and never knows the difference, and
   its usage and latency get captured with no cooperation from Claude Code.
2. It forces `ANTHROPIC_MODEL` and `ANTHROPIC_SMALL_FAST_MODEL` to `$LAB_MODEL`
   so every request, including the cheap side tasks Claude Code farms out to the
   small fast model, rides the one shared model each tool is graded on.
3. It disables the autoupdater, telemetry, error reporting, and other
   nonessential traffic, and pre-seeds `~/.claude.json` so the first-run
   onboarding and folder-trust prompts a headless run can never answer are
   already dismissed.
4. `claude -p "$prompt" --dangerously-skip-permissions --output-format text` runs
   the task headless in one shot. `-p` prints the result and exits;
   `--dangerously-skip-permissions` drops every approval prompt, Claude Code's
   equivalent of tomo's all-allow policy, safe here because the container is the
   sandbox. The run is wrapped in `/usr/bin/time -v` so the harness reads peak
   memory back.

To build and run:

    go run ./cmd/lab build claude-code
    go run ./cmd/lab run claude-code
