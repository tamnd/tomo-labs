---
title: "A 4-bit Qwen3-30B MoE on one RTX 4090 fixed briefcase-2085 in 47 seconds"
linkTitle: "briefcase-2085, local MoE"
description: "beeware__briefcase-2085 is a real SWE-bench-Live task: Briefcase fails to roll out templates when a user's Git config rewrites HTTPS to SSH with insteadOf, because it calls remote.set_url with an old_url that no longer exists. A local Qwen3-30B-A3B, quantized to 4-bit GGUF Q4_K_M and served by Ollama on a single 24 GB RTX 4090 behind the llmgw gateway, solved it through the tomo agent in one attempt. The fix is one line: drop the old_url argument so Git updates the origin URL unconditionally. The run took 47 wall seconds, seven requests, and 28,945 tokens, and the model diagnosed the bug from the problem statement before it opened a single file. The lesson: a consumer GPU running a quantized 32B-class MoE is now enough to close a genuine upstream bug end to end."
date: 2026-07-19T22:32:00+07:00
---

Reproduction facts up front.
Tool was tomo, the coding agent.
Model was qwen3-30b-a3b, a Qwen3-30B-A3B mixture-of-experts served as GGUF Q4_K_M by Ollama on a single RTX 4090 with 24 GB, fronted by the llmgw gateway.
Suite was swebench-live, task was beeware__briefcase-2085 from the beeware/briefcase repo.
Result was a clean pass: the hidden FAIL_TO_PASS tests went green and the in-file PASS_TO_PASS tests stayed stable.

Reproduce it with:

    LABS_DIR=~/github/tamnd/tomo-labs TOKEN=<gateway token> \
      MODELS=qwen3-30b-a3b TASKS=beeware__briefcase-2085 \
      scripts/solve-swebench.sh

That script lives in the local-llm repo and drives tomo against the task inside the podman sandbox, then grades with the task's own check.sh.

## Setup

The task is a real reported bug, checked out at the commit where it was filed, with future git history stripped so a passing test cannot be fetched or read off disk.
tomo runs in the sandbox with the network off and edits the source in place.
The problem statement tells the agent not to touch tests, and a hidden test suite grades the change.
The grader builds a fresh venv, installs the project, restores any test file the agent touched, applies the hidden test patch, and runs the bug's test files, requiring every FAIL_TO_PASS id green and no in-file PASS_TO_PASS id regressed.

The model is the interesting part.
Qwen3-30B-A3B is a mixture-of-experts with about 30B total parameters and roughly 3B active per token.
Here it is quantized to 4-bit (Q4_K_M) and served by Ollama, so the whole thing fits in the 4090's 24 GB.
The gateway in front of it speaks the OpenAI streaming API, and tomo drives it with native function calling.

## What the task asked

Briefcase caches template repos and, before reusing one, points the cached repo's `origin` at the URL being requested.
A user reported that this fails for anyone whose `~/.gitconfig` rewrites URLs:

    [url "ssh://git@github.com/"]
        insteadOf = https://github.com/

With that rule in place, `briefcase new` dies with:

    GitCommandError: Cmd('git') failed due to: exit code(128)
      cmdline: git remote set-url -- origin https://github.com/beeware/briefcase-template https://github.com/beeware/briefcase-template
      stderr: 'fatal: No such URL found: https://github.com/beeware/briefcase-template'

The command passes two URLs.
The second one is the "old URL" to match against the remote's current URLs, and `insteadOf` has rewritten the stored URL to the `ssh://` form, so the `https://` old URL matches nothing and Git aborts.
The issue itself spells out the fix: the old URL is optional, and since Briefcase owns these cloned repos it can just set the origin URL blindly.

## What tomo did, turn by turn

Seven requests, six model calls, five tool calls, start to finish in 47 seconds.
The first request came back as `404 page not found`, a gateway blip, and tomo retried without drama.
From there the model worked the problem in a tight loop, and its entire chain of thought streamed on the response's `reasoning` channel while the actions came through as function calls.

Round one, before opening a single file, the model read the traceback and diagnosed the bug from the problem statement alone:

    So it's setting the origin to the new URL, but providing an old URL
    (the same as the new URL) which doesn't exist. The correct usage
    should be `git remote set-url origin <new-url>`, without the old URL.

That is the correct root cause, reached from the error text.
It then called `grep` for `set-url` across the tree and got no matches, because the code uses the GitPython method, not the shell string.
It caught its own miss and widened the search to `set_url`, which landed the line:

    src/briefcase/commands/base.py:1017:                 remote.set_url(new_url=template, old_url=remote.url)

The same grep also surfaced four `set_url` asserts in the test file, and the model noted them but held the line that it must not edit tests.
It went to edit `base.py` and the first edit failed:

    ERROR: old_string not found in src/briefcase/commands/base.py

The cause is a whitespace trap.
grep prints its match with a leading space after the line number, so the model copied an indentation of seventeen spaces when the real line has sixteen.
It recovered the honest way: it `read` line 1017 exactly, saw the true indentation, and reissued the edit with the correct sixteen-space prefix:

    edited src/briefcase/commands/base.py: replaced 1 occurrence(s)

Then it stopped and wrote a one-line summary of the change.
No test was ever edited by the model; the only source edit was that single line.

## The fix

One argument removed:

    -                remote.set_url(new_url=template, old_url=remote.url)
    +                remote.set_url(new_url=template)

The surrounding code is unchanged.
The call still sits in the `try` that already logs a warning and continues if the later `remote.fetch()` cannot reach the origin, so the cached template is still used when offline.

The test-file changes you would see in the work tree after grading are not the model's.
The grader restores any test the agent touched, then applies the hidden test patch, which drops the same `old_url=` argument from the four `assert_called_once_with` blocks.
Those edits are the oracle patch landing on disk during grading, not the agent leaning on the tests.

## Why it passed

The hidden tests assert that Briefcase now calls `set_url` with only `new_url`.
Before the fix, `remote.set_url(new_url=template, old_url=remote.url)` expands to `git remote set-url origin <new> <old>`, where Git treats the old URL as a filter it must find among the remote's current URLs.
Under an `insteadOf` rewrite the stored URL is the `ssh://` form, the `https://` old URL matches nothing, and Git exits 128.
Dropping `old_url` turns the call into `git remote set-url origin <new>`, which sets the origin URL unconditionally and never consults the existing value.
That is exactly the behavior the issue asked for and exactly what the updated asserts check, so FAIL_TO_PASS goes green while the offline-warning path that the other in-file tests cover is untouched, so PASS_TO_PASS holds.

## Metrics

| Metric | Value |
|---|---|
| Result | PASS (fail_to_pass green, in-file pass_to_pass stable) |
| Wall clock | 47 s (0:46.62) |
| Requests | 7 (one 404 retry) |
| Model calls | 6 |
| Tool calls | 5 (grep, grep, edit-fail, read, edit-ok) |
| Prompt tokens | 21,845 |
| Completion tokens | 7,100 |
| Total tokens | 28,945 |
| Avg time to first byte | 1,298 ms |
| Avg call latency | 7,499 ms |
| Peak RSS (agent) | 19 MB |

The completion tokens are almost all reasoning.
The model narrated its whole diagnosis on the `reasoning` channel and put nearly nothing on the `content` channel until the final summary, and the actions rode a separate function-calling path, so tomo never had to parse a code fence out of prose.

## The lesson

A quantized 32B-class MoE on one consumer GPU is now enough to close a real upstream bug end to end, and it did so cheaply.
The whole run was 47 seconds and under 29k tokens, on a 4-bit model that fits in 24 GB of VRAM with no cloud call in the loop.

Two things carried the run, and neither is raw model size.
First, the reasoning channel earned its keep: the model reached the correct root cause from the traceback before it read any code, so exploration was three quick lookups, not a search of the repo.
Second, the agent's tool discipline covered the model's small slips.
An empty `grep` for the shell string became a `grep` for the Python method name.
A whitespace mismatch from grep's own output was fixed by reading the exact line and retrying, not by flailing.
Even the opening 404 from the gateway cost nothing because the retry is built in.

The honest caveat is that this is one task, and an easy one: a single-line fix the issue itself described, with a hidden test that checks exactly that call.
It does not say a local 4-bit model matches a frontier model on hard multi-file work.
It does say that for the large class of small, well-specified fixes, a coding agent on a local quantized MoE is a real option now, fast and private and free at the margin, and the harness around the model matters as much as the model.
