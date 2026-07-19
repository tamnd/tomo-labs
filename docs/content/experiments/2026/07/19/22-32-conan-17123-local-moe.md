---
title: "A local 4-bit Qwen3-30B MoE taught .conanignore inverse matching in 196 seconds of mostly thinking"
linkTitle: "conan-17123, local MoE"
description: "conan-io__conan-17123 is a real SWE-bench-Live feature request: .conanignore should support inverse matching with a leading !, the way .gitignore and .dockerignore do, so a config repo can ignore everything and then re-include a few files. A local Qwen3-30B-A3B, quantized to 4-bit GGUF Q4_K_M and served by Ollama on one 24 GB RTX 4090 behind the llmgw gateway, solved it through the tomo agent in a single attempt. The fix splits the matcher into an ignore set and an un-ignore set and checks the negations first. The run passed clean but took 196 wall seconds and 28,634 tokens, four times slower than the same model on the briefcase task, and almost all of that time was the model deliberating on its own reasoning channel. The lesson: a consumer-GPU 4-bit MoE can close a real feature-shaped bug, but on a local box the thinking is the clock."
date: 2026-07-19T22:32:00+07:00
---

Reproduction facts up front.
Tool was tomo, the coding agent.
Model was qwen3-30b-a3b, a Qwen3-30B-A3B mixture-of-experts served as GGUF Q4_K_M by Ollama on a single RTX 4090 with 24 GB, fronted by the llmgw gateway.
Suite was swebench-live, task was conan-io__conan-17123 from the conan-io/conan repo.
Result was a clean pass: the hidden FAIL_TO_PASS tests went green and the in-file PASS_TO_PASS tests stayed stable.

Reproduce it with:

    LABS_DIR=~/github/tamnd/tomo-labs TOKEN=<gateway token> \
      MODELS=qwen3-30b-a3b TASKS=conan-io__conan-17123 \
      scripts/solve-swebench.sh

That script drives tomo against the task inside the podman sandbox, then grades with the task's own check.sh.

## Setup

The task is a real reported issue, checked out at the commit where it was filed, with future git history stripped so a passing test cannot be fetched or read off disk.
tomo runs in the sandbox with the network off and edits the source in place.
The problem statement tells the agent not to touch tests, and a hidden test suite grades the change.
The grader builds a fresh venv, installs the project, restores any test file the agent touched, applies the hidden test patch, and runs the bug's test files, requiring every FAIL_TO_PASS id green and no in-file PASS_TO_PASS id regressed.

The model is the same local box as the briefcase run.
Qwen3-30B-A3B is a mixture-of-experts with about 30B total parameters and roughly 3B active per token.
Here it is quantized to 4-bit (Q4_K_M) and served by Ollama, so the whole thing fits in the 4090's 24 GB.
The gateway in front of it speaks the OpenAI streaming API, and tomo drives it with native function calling.

## What the task asked

`.conanignore` lets `conan config install` filter which files get copied into the Conan home.
The reporter keeps their Conan config in a shared Git repo full of unrelated automation dotfiles, and they wanted the common pattern of ignore-all-then-allow-a-few:

    *
    !profiles/
    !global.conf
    !remotes.json

A leading `!` should mean "do not ignore this after all", exactly like `.gitignore` and `.dockerignore`.
The existing matcher had no notion of negation.
It read every non-comment line into one set and ignored a path if it matched any entry, so a line like `!profiles/` was itself treated as a literal pattern to ignore, which is the opposite of what the user wanted.

## What tomo did, turn by turn

Seven requests, six model calls, five tool calls, start to finish in 196 seconds.
The first request was a `GET /` health probe that came back `404 page not found`, a gateway blip that cost nothing.
From there the model worked the problem in a tidy loop, and its entire chain of thought streamed on the response's `reasoning` channel while the actions came through as function calls.
The `content` channel stayed empty until the final summary.

Round one, the model planned before touching anything and reached for `grep`:

    I'll start by using grep to find where .conanignore is processed.
    ... So I'll call grep with pattern "conanignore".

The grep landed the class straight away, matching both the implementation and the test file:

    conan/internal/api/config/config_installer.py:16:  def __init__(self, conanignore_path, ignore=None):
    conan/internal/api/config/config_installer.py:18:      self._ignored_entries = {".conanignore"}
    conan/internal/api/config/config_installer.py:129:      if not conanignore.matches(rel_path):
    test/integration/command/config_test.py:67: def test_config_install_conanignore():

Round two it read the class with `read` at offset 16, and saw the whole of the matcher:

        self._ignored_entries = {".conanignore"}
        ...
                    if line_content:
                        self._ignored_entries.add(line_content)
        ...
        def matches(self, path):
            for ignore_entry in self._ignored_entries:
                if fnmatch.fnmatch(path, ignore_entry):
                    return True
            return False

Round three is where the local model spent itself.
Its reasoning for this single turn ran 24,297 characters, working out the negation semantics, the default `.conanignore` entry it had to preserve, and the exact old and new strings for both edits before it committed to anything:

    So the plan is:
    1. When parsing the conanignore file, separate lines that start with '!'
       into a different list (e.g., _unignored_patterns).
    2. In the matches method, first check if any _unignored_patterns matches.
       If so, return False (not ignored).
    3. Otherwise, check if any _ignored_entries matches.

It then edited only the `__init__` method, because tomo runs one tool call per turn:

    edited conan/internal/api/config/config_installer.py: replaced 1 occurrence(s)

Round four the model caught its own half-finished job.
It had rewritten the parser but not the matcher, and its reasoning says so plainly:

    In my previous tool call, I only edited the __init__, but didn't fix matches.
    That would be a problem. So I need to make another edit to the matches method.

It issued the second edit, adding the negation check to `matches`.
Round five it ran the existing suite with `bash` to check it had not broken anything:

    ============================== 7 passed in 6.34s ===============================
    test/integration/command/config_test.py::test_config_install_conanignore PASSED

Round six it wrote a one-line summary and stopped.
The model never edited a test file.

## The fix

Two edits to `conan/internal/api/config/config_installer.py`, both in the `_ConanIgnoreMatcher` class.

The parser now sends `!` lines to a separate set instead of the ignore set:

    -        self._ignored_entries = {".conanignore"}
    +        self._ignored_entries = set()
    +        self._unignored_patterns = set()
    +        self._ignored_entries.add(".conanignore")
             if os.path.exists(conanignore_path):
                 with open(conanignore_path, 'r') as conanignore:
                     for line in conanignore:
                         line_content = line.split("#", maxsplit=1)[0].strip()
    -                    if line_content:
    +                    if not line_content:
    +                        continue
    +                    if line_content.startswith('!'):
    +                        pattern = line_content[1:].strip()
    +                        if pattern:
    +                            self._unignored_patterns.add(pattern)
    +                    else:
                             self._ignored_entries.add(line_content)

And the matcher checks the negations first, returning "not ignored" on a hit before it ever looks at the ignore set:

     def matches(self, path):
    +    for unignored in self._unignored_patterns:
    +        if fnmatch.fnmatch(path, unignored):
    +            return False
         for ignore_entry in self._ignored_entries:
             if fnmatch.fnmatch(path, ignore_entry):
                 return True
         return False

The default `.conanignore` self-ignore is preserved, and a file with no negation match behaves exactly as before, so existing configs are untouched.

The test-file changes you would see in the work tree after grading are not the model's.
The grader restores any test the agent touched, then applies the hidden test patch, which adds `!b/c/important_file` style cases and a new ignore-all-then-allow workflow test.
Those edits are the oracle patch landing on disk during grading, not the agent leaning on the tests.

## Why it passed

The hidden test extends `test_config_install_conanignore` with negated lines and adds a second test that ignores everything with `*` and re-includes `!important_folder/*`, `!important_file`, and even `!.conanignore`.
Before the fix, a `!` line was just another literal pattern in the single ignore set, so `matches` could only ever return "ignore" and never "keep", and every re-included file stayed filtered out.
After the fix, `matches` consults `_unignored_patterns` first and returns `False` the moment a path matches a negation, so `b/c/important_file` survives even though `b/c/helmet` next to it is still dropped, and the `*`-then-allow config keeps exactly the named files.
That is what the new asserts check, so FAIL_TO_PASS goes green, and because a config without any `!` line still routes every entry into `_ignored_entries` and matches the same way, the in-file PASS_TO_PASS cases hold.

## Metrics

| Metric | Value |
|---|---|
| Result | PASS (fail_to_pass green, in-file pass_to_pass stable) |
| Wall clock | 196 s (3:15.78) |
| Requests | 7 (one 404 health probe) |
| Model calls | 6 |
| Tool calls | 5 (grep, read, edit, edit, bash) |
| Prompt tokens | 17,640 |
| Completion tokens | 10,994 |
| Total tokens | 28,634 |
| Avg time to first byte | 20,553 ms |
| Avg call latency | 30,273 ms |
| Sum of call latency | 181,643 ms |
| Peak RSS (agent) | 55 MB |

## The lesson

Same box, same model, same clean single pass as briefcase-2085, but four times the wall clock: 196 seconds against 47.
The token counts are close, 28,634 here versus 28,945 there, so the gap is not more work.
It is that on this task the model put far more of its budget into thinking.
The completion count is 10,994 tokens against briefcase's 7,100, almost all of it reasoning, and of the 196 wall seconds, 181.6 were spent inside model calls.

The trace shows where it went.
The two edit rounds were the expensive ones: the request gaps around them were 56 and 74 seconds, and the `__init__` round alone carried a 24,297-character chain of thought that re-derived the same negation plan several times over.
On a cloud frontier model that deliberation would be cheap tokens at high throughput.
On a 4-bit MoE generating at a local 4090's token rate, with a 20-second average time to first byte, every extra thousand reasoning tokens is real seconds on the clock.

The encouraging half is that the thinking bought a correct answer.
The model reached the right two-set design, remembered to keep the default `.conanignore` self-ignore, and, most tellingly, noticed on its own that its first edit left `matches` unfixed and issued the second one instead of declaring victory.
It also verified against the existing suite before it stopped, so the pass was earned, not hoped for.

The honest caveat is the same as before, plus one.
This is one task, a well-specified feature with a hidden test that checks exactly the described behavior, and it does not say a local 4-bit model matches a frontier model on hard multi-file work.
What it adds is the local-box tax: the model that can solve these tasks for free at the margin is slow enough that its own reasoning, not the tool loop, is the thing to watch.
When a run drags, the first place to look is how many tokens the model spent talking to itself, because on a single GPU that is the clock.
