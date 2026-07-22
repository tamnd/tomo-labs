# swebench-live-container

The faithful SWE-bench-Live runner. Where the sibling [`../swebench-live`](../swebench-live)
tier grades on the host in a `uv`-built venv (fast, portable, lower fidelity), this
tier mirrors the upstream harness one to one: it runs and grades inside the
per-instance prebuilt image, so the environment the agent iterates against and the
environment the grade runs in are the ones the task actually ships with.

Use this tier when fidelity is the point (comparing agents or models on a specific
unsolved instance, reading real token and cost numbers off the wire). Use the host
tier when breadth and speed are the point.

## What it mirrors

SWE-bench-Live ships a per-instance image
`docker.io/starryzhang/sweb.eval.x86_64.<instance>` with the repo checked out at
`base_commit` and fully installed on its own interpreter. The upstream flow is: apply
the agent's patch into `/testbed` with the fallback chain (`git apply --verbose`,
then `--reject`, then `patch --fuzz=5`), reset the graded test files to base, apply
the hidden `test_patch`, run the row's own `test_cmds`, and read the pytest `-rA`
outcomes by node id. Resolved means every `FAIL_TO_PASS` passes and every
`PASS_TO_PASS` stays green. `eval_instance.sh` is that flow, run in a fresh
`--network none` container so nothing the agent did to its tree can reach the grader
beyond the patch it produced.

## Layout

```
run_task.sh        one (task, tool) attempt on a metered gateway (deepseek/zen)
run_sub.sh         one (task, tool) attempt on a subscription model via the bridge
run_codex.sh       one (task, codex) attempt on a subscription model via the bridge
eval_instance.sh   offline upstream apply-test-grade, per-instance image
sweep.sh           run several tools on one task, print a result table
overlay/           Dockerfile.overlay + run_agent.sh: tool CLIs baked onto the image
net/               no-leak network: usage proxy, subscription bridge, dockerfiles
```

The agent runs on an internal docker network (`swelive-int`, `--internal`, no egress)
whose only reachable host is the usage proxy. That is the leak fence: `git fetch` of
the gold PR, a GitHub API call, or a pip install all fail because github and pypi do
not resolve. `run_agent.sh` also strips every remote and ref beyond `base_commit` and
expires the reflog, so the fix is unrecoverable from the working tree. Every run in
the reports on this task caught the agent probing that fence (fetch, cherry-pick,
`git fsck --unreachable`) and getting nothing.

## Metered gateway vs subscription bridge

`run_task.sh` points the agent at a usage proxy in front of a plain OpenAI-dialect
gateway (`https://opencode.ai`), for metered keys like the free deepseek tier.

`run_sub.sh` and `run_codex.sh` route through the **subscription bridge** instead, so
a model that is only available on a ChatGPT subscription (no metered key) can run on
the same faithful flow:

```
agent (swelive-int, no egress)
  -> usage proxy  (swelive-int)         captures token usage per call
  -> codex bridge (swelive-int+egress)  injects the subscription token, pins model+effort
  -> chatgpt.com/backend-api/codex/responses
```

The bridge (`cmd/lab bridge`, built into `net/Dockerfile.bridge` as `_labbin`) serves
`/v1/chat/completions` by translating to the Responses wire, and `/v1/responses`
verbatim for tools like codex that speak Responses natively. It holds the
subscription OAuth token, sets the model and effort, and refreshes the token when the
backend says it is stale. It runs with `LAB_RUNTIME=noop` because the `bridge`
subcommand only serves HTTP and must not require a container runtime it never uses.

The usage proxy (`net/proxy.py`) is dialect-aware: it reads token usage from both the
chat streamed `usage` chunk and the Responses `response.completed` event, and
normalises them onto one schema. One measured caveat: the chat-to-Responses path does
not surface `cached_tokens` back to the proxy, so on that path the prompt token total
is real but its cache-hit split is not observable. The Responses-native path (codex)
does expose the split.

## Grading a new-test-file instance

`eval_instance.sh` resets each graded test file individually rather than in one
`git checkout BASE -- <all>`. A single multi-path checkout aborts wholesale when any
path is new in the hidden patch (does not exist at base), which would leave an agent's
edits to the *other* graded test files in place and make the atomic `git apply` of the
hidden patch fail on them. Per file: restore tracked files to base, delete
agent-created new ones so the hidden patch recreates them. Net effect: the graded test
tree is exactly base plus the hidden test patch, regardless of what the agent did to
test files. This matters for any agent whose patch strays into the graded test files.

## Run it

```sh
# metered gateway (free deepseek)
LAB_MODEL=deepseek-v4-flash-free bash run_task.sh <slug> <image> <tool> "<test_cmd>"

# subscription model, tomo agent or oi engine
LAB_MODEL=gpt-5.6-sol LAB_EFFORT=high TOMO_ENGINE=oi \
  bash run_sub.sh <slug> <image> tomo "<test_cmd>"

# subscription model, real codex CLI (Responses wire)
LAB_MODEL=gpt-5.6-sol LAB_EFFORT=high \
  bash run_codex.sh <slug> <image> "<test_cmd>"
```

Each run drops its trace under `runs/<slug>/<tool>/`: the model patch, per-call token
usage (`trace/usage.jsonl`), stdout/stderr, `/usr/bin/time -v` output, and the grade.

The `_labbin` in `net/` is `go build -o _labbin ./cmd/lab` from the repo root. The
scenario (`scenario/prompt.txt` is the raw problem statement, `scenario/base_commit`)
and the oracle (`dyn/{test.patch,f2p.json,p2p.json,gold.patch}`) are per-instance and
are not committed here; they are rendered from the instance the same way the host tier
renders its tasks. The subscription bridge needs a `codexauth/auth.json` (the ChatGPT
OAuth token, mode 0600), which is never committed.

## Reports

- [faithful container, deepseek three-way](../../docs/content/experiments/2026/07/22/18-45-faithful-swebench-live-container-deepseek.md)
- [codex + gpt-5.6-sol](../../docs/content/experiments/2026/07/22/20-20-swebench-live-codex-gpt56sol-dynaconf-1225.md)
- [tomo-agent + gpt-5.6-sol](../../docs/content/experiments/2026/07/22/20-21-swebench-live-tomo-agent-gpt56sol-dynaconf-1225.md)
- [tomo-oi + gpt-5.6-sol](../../docs/content/experiments/2026/07/22/20-22-swebench-live-tomo-oi-gpt56sol-dynaconf-1225.md)
