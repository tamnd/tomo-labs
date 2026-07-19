---
title: "The easiest task on one 4090: a 4-bit Qwen3 MoE solves gitingest-94 locally"
linkTitle: "gitingest-94, local MoE"
description: "The local-4090 counterpart to the five-free-models study on the same task. This run drives tomo with qwen3-30b-a3b, a 4-bit Qwen3-30B-A3B MoE served by Ollama on a single RTX 4090 behind the llmgw gateway, on cyclotruc gitingest-94, the easiest task in the swebench-live set. It passes at pass@1: seven model calls, five tool calls, 35.6k total tokens, 214 wall seconds, hidden FAIL_TO_PASS green. No hosted API is involved, so the whole loop runs on one consumer GPU. The interesting part is the contrast: the free-hosted study on this task was dominated by harness friction around code-as-action fences, and this native-tool-calling run has none of that, it just over-thinks in the Qwen3 reasoning channel and lands the one-line fix."
date: 2026-07-19T22:34:00+07:00
---

This is the local-hardware counterpart to an earlier study on the same task, [gitingest-94 with five free hosted models](/experiments/2026/07/16/22-30-gitingest-94-free-models-harness-not-capability/).
That session ran the free zen models through tomo-oi, the code-as-action loop, and found the failures were the harness, not the models.
This one asks a narrower question: can a 4-bit MoE running on a single consumer GPU, with no hosted API in the loop at all, solve the same task?
It can, and it does it clean at pass@1.

Reproducibility: tool=tomo, model=qwen3-30b-a3b (Qwen3-30B-A3B GGUF Q4_K_M served by Ollama on an RTX 4090, 24 GB, behind the llmgw gateway), suite=swebench-live, task=cyclotruc__gitingest-94.
Reproduce command:

    LABS_DIR=~/github/tamnd/tomo-labs TOKEN=<gateway token> \
      MODELS=qwen3-30b-a3b TASKS=cyclotruc__gitingest-94 \
      scripts/solve-swebench.sh

## Setup

The tool is tomo, driven through its native function-calling engine, not the code-as-action engine the free-models study used.
That distinction matters for the contrast below, so it is worth stating plainly.
The free-models session parsed actions out of Markdown code fences, and most of its failures were fences the parser dropped or mis-ran.
This run gives the model a real tool schema, `grep`, `read`, `edit`, and `bash`, and the model returns structured `tool_calls` in the API response, so there is no fence to drop.

The model is qwen3-30b-a3b, the Qwen3-30B-A3B mixture-of-experts, quantized to GGUF Q4_K_M, served by Ollama on one RTX 4090 with 24 GB.
It is a 30B-parameter model with about 3B active per token, so the 4-bit weights fit the card and the active path stays cheap.
The gateway in front is llmgw, which is why the wire looks like a normal OpenAI-style streaming endpoint.
The task checkout has its future git history stripped, so a passing test cannot be fetched from upstream, and the grade comes from the task's own `check.sh`.

## What the task asked

The bug report is short and it names its own suspect.

    Git Ingest request fails on web if the input repo url starts with
    "http://" instead of "https://"
    I think it might be related to `_parse_url` in `parse_query.py`
    explicitly checking for `"https://"`. Was this intended?

In plain terms, `_parse_url` normalizes a bare URL by gluing on a scheme when one is missing.
The check it uses is wrong.

    url = url.split(" ")[0]
    url = unquote(url)  # Decode URL-encoded characters

    if not url.startswith("https://"):
        url = "https://" + url

A URL that already starts with `http://` does not start with `https://`, so the branch fires and prepends the scheme anyway.
The result is `https://http://github.com/user/repo`.
A few lines down the code splits on `/` and reads `url_parts[2]` as the domain, and on the mangled string that slot is `http:`, not `github.com`, so the parse comes out garbage and the request fails.
The hidden grader adds one test, `test_parse_url_valid_http`, that feeds `http://github.com/user/repo` and friends through `_parse_url` and asserts the user, repo, and slug come out right.
That test is the single FAIL_TO_PASS.

## What tomo did

Seven model calls, five of them ending in a tool call, then two rounds of prose to close out.
The path is short and it never wanders.

Round one, the model greps for the function it was told about.

    grep {"glob":"*.py","pattern":"_parse_url","path":"."}

The result points it at `src/gitingest/parse_query.py:77`.
Round two it reads that region, `read` with offset 70 and limit 20, but the window lands on the docstring and shows no logic.
Round three it stops reading blind and greps the file for the literal string from the bug report.

    grep {"pattern":"https://","path":"src/gitingest/parse_query.py"}

That returns the four lines that mention `https://`, including the two that matter:

    parse_query.py:103:     if not url.startswith("https://"):
    parse_query.py:104:         url = "https://" + url

Round four is where the Qwen3 reasoning channel earns its keep and also shows its cost.
The model works out, in the reasoning field, exactly the failure mode described above, that an `http://` input gets `https://` prepended and becomes `https://http://...`, and it re-derives that conclusion several times before committing.
The reasoning for that single turn is 3632 completion tokens, most of it circular, a run of "Wait, ... Wait, ... Wait," that keeps re-checking an answer it already has.
It lands on the right edit and makes it:

    edit {"old_string":"if not url.startswith(\"https://\"):",
          "new_string":"if not (url.startswith(\"http://\") or url.startswith(\"https://\")):",
          "path":"src/gitingest/parse_query.py"}

Round five runs the tests: `bash pytest src/gitingest/tests`.
The parse_query file goes green, all seventeen of its tests pass, but the full run exits red because `test_clone.py` fails eleven of eleven.

    src/gitingest/tests/test_clone.py FFFFFFFFFFF                  [ 36%]
    src/gitingest/tests/test_ingest.py ..                         [ 43%]
    src/gitingest/tests/test_parse_query.py ................. [100%]

The clone tests need git and network that the sandbox does not have, so they are unrelated to the change.
Round six the model says so and declares the fix done.
That ends the turn on a command that exited non-zero, so tomo's red-check guard fires and nudges once:

    You are ending the turn, but the last test or build command you ran
    exited with an error ... If the failure is from parts of the suite
    that cannot run here ... run only the tests that exercise your change
    and confirm those are green.

Round seven the model holds its ground in prose, argues the `test_clone.py` failures were pre-existing and out of scope, and ends.
It does not actually re-run a scoped `pytest src/gitingest/tests/test_parse_query.py` the way the nudge asked, it just asserts the conclusion.
That would matter if the grader trusted the model's own test run, but it does not.
The grader restores the test files, applies its hidden `test.diff`, and runs only the bug's test file, so the model's shortcut at the very end costs nothing here.

## The fix

One line, in `src/gitingest/parse_query.py`.

    -    if not url.startswith("https://"):
    +    if not (url.startswith("http://") or url.startswith("https://")):
             url = "https://" + url

A URL that already carries either scheme is now treated as already-schemed and left alone.
Only a truly bare URL gets `https://` prepended, which is the original intent.

## Why it passed

With the guard widened, `http://github.com/user/repo` no longer gets a second scheme glued on.
It splits to `['http:', '', 'github.com', 'user', 'repo']`, so `url_parts[2]` is `github.com`, and the user, repo, and slug parse out correctly.
That is exactly what the hidden `test_parse_url_valid_http` asserts, so the single FAIL_TO_PASS goes green.
The existing `https://` cases are untouched, so the PASS_TO_PASS set, including the renamed `test_parse_url_valid_https`, stays stable.
The grade is `PASS: fail_to_pass green, in-file pass_to_pass stable`.

## The local angle

This is the same task the free hosted models were measured on, done by a 4-bit MoE on one 4090, with nothing hosted in the loop.
Two things stand out against the free-hosted session.

The harness friction that dominated that study is simply absent here.
Every failure in the five-free-models run was about code-as-action fences, an editor verb run as a shell command, a narration envelope the finish guard missed.
Native tool calling removes that surface: the model emitted five tool calls, all with well-formed JSON arguments, and the parser never had to guess.
When the model over-produces, it over-produces in the reasoning channel, which the engine ignores, not in the action channel, which the engine parses.

The cost profile is a local one.
Prompt tokens per call stay small, from 2052 up to 3193, because the checkout is small and grows only by tool output.
The spend is on the completion side, 16811 completion tokens against 18791 prompt tokens, and the completion is mostly Qwen3 thinking.
The single edit turn alone is 3632 completion tokens for a one-line change.
On the 4090 that shows up as latency, an average 17.4-second time to first byte and 30 seconds per call, which is where the 214-second wall goes.
The trace files are large for the same reason, 1.4 MB of streamed SSE for round seven, because every reasoning token arrives as its own chunk wrapped in a full JSON envelope.

## The lesson

The floor holds on local hardware, and the shape of the cost changes.
A 4-bit Qwen3-30B MoE on a single 4090 solves the easiest swebench-live task at pass@1, with no hosted API, and it reaches the fix by the same route a person would: grep the named function, grep the named string, read the two lines that matter, widen the guard, run the tests.
The Qwen3 reasoning channel is where this model spends, and it spends a lot, re-deriving a conclusion it already has before it acts.
Native tool calling is what keeps that verbosity harmless, the rumination stays in the reasoning field and the action that reaches the engine is a clean, well-formed call, so none of the fence-parsing failures from the hosted study can happen.
The one soft spot, the model arguing past a red check instead of re-running a scoped test, did not bite only because the grader scopes to the bug's own file, and it is worth watching on tasks where the failing suite and the target test share a file.

Metrics: pass@1, 8 requests (one 404 gateway probe, 7 model calls), 5 tool calls, prompt 18791 tokens, completion 16811 tokens, total 35602, avg TTFB 17.4 s, avg call 30.0 s, wall 214 s, max RSS 40.9 MB, check `PASS: fail_to_pass green, in-file pass_to_pass stable`.
