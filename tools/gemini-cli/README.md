# gemini-cli adapter

This folder plugs Google's Gemini CLI into the lab. Like every tool, it is
defined by exactly two files, the same two tomo has next door:

- `Dockerfile` builds an image named `tomolab-tool-gemini-cli`, based on
  `tomolab-base` so it shares the same toolchain every other tool runs against.
  It installs gemini-cli from npm (package `@google/gemini-cli`, a Node
  launcher). The base carries Node 22, which the launcher needs.
- `adapter.sh` is the container entrypoint. The harness mounts `/work` (the
  scenario's working tree and the agent's cwd), `/scenario` (read-only, holds
  `prompt.txt`), and `/trace` (where stdout and the time report go), and passes
  `LAB_BASE_URL`, `LAB_MODEL`, `OPENCODE_API_KEY`, and `LAB_MAX_TURNS`.

What the adapter does, in order:

1. It sets `GOOGLE_GEMINI_BASE_URL` to the proxy. gemini-cli speaks the Gemini
   generateContent wire, and the free deepseek model speaks chat completions, so
   the proxy at that URL translates the Gemini request into a chat request and
   the chat response back into a Gemini stream. gemini-cli talks its native wire
   and never knows, and its request/response and token usage get captured with no
   cooperation from it. The SDK builds its URL as
   `{base}/v1beta/models/{model}:generateContent`, so the base drops the `/v1`
   suffix the OpenAI tools use.
2. It sets `GEMINI_API_KEY`, which switches gemini-cli from its default OAuth
   login to API-key auth so it runs headless. The key is our upstream credential;
   the SDK sends it as `x-goog-api-key`, which the proxy folds into the bearer it
   forwards to the real upstream.
3. `gemini -m "$LAB_MODEL" --yolo -p "$prompt"` runs the task headless in one
   shot. `-p` is the non-interactive prompt mode; `--yolo` auto-approves every
   tool call so the agent can act freely, gemini-cli's equivalent of tomo's
   all-allow policy, safe here because the container is the sandbox; `-m` pins the
   shared model, which rides in the request URL and is what the proxy reads to
   translate the call. The run is wrapped in `/usr/bin/time -v` so the harness
   reads peak memory back.

To build and run:

    go run ./cmd/lab build gemini-cli
    go run ./cmd/lab run gemini-cli
