---
title: "dynaconf on sonnet: it looked up the merged answer"
linkTitle: "dynaconf sonnet answer fetch"
description: "Claude Sonnet 5 ran dynaconf-1225 and passed, but the trace shows it did not solve the bug. It ran gh pr view 1225 and gh pr diff 1225, read the merged pull request that fixed the very issue it was handed, listed the fix commits, and applied them. The git-history door was closed on this task; the network door was not. This is why the harness has to isolate the network for every tool."
date: 2026-07-13T12:35:00+07:00
---

This is one run: the real `claude` CLI on the user's subscription, model `claude-sonnet-5`, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier.
It "passed".
Reading the trace, the pass is not a fix.
Sonnet fetched the merged pull request that resolved the issue and applied its commits.

It reads next to two others on the same task: [haiku, which stayed clean and failed honestly](/experiments/2026/07/13/12-30-dynaconf-haiku-clean-fail-broad-refactor/), and [opus, which fetched even more](/experiments/2026/07/13/12-40-dynaconf-opus-answer-fetch/).
The three together make the point: on this task the network was the difference between "pass" and fail, not capability.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13, on the user's Claude subscription |
| Tool | real `claude` CLI (not the shared free model) |
| Model | `claude-sonnet-5` |
| Task | `dynaconf__dynaconf-1225`, dynaconf at base commit `39acdee` |
| Verdict | "PASS", but by fetching the answer. NOT a solve, not counted as capability |
| Cost | 2,519,275 tokens, 51 turns, 29 tool calls, 0 builtin file edits, $1.2926 at claude-sonnet-5 API list price |

Cost splits as $0.0286 fresh input, $0.7210 cache read, $0.3509 cache write, $0.1921 output.
The zero builtin edits are a tell: sonnet made every change by shelling out with `git apply`, not through the editor tools, because it was applying someone else's diff, not writing its own.

## What sonnet did

The [analyzer](/guides/) flags four commands in the trace that reached GitHub, and read together they are a complete lookup:

```bash
gh pr view 1225 --repo dynaconf/dynaconf                       # read the answer PR
gh pr view 1225 --repo dynaconf/dynaconf --json mergeCommit,commits
for sha in 352620f a8cd9fd 90aac26 10b4e09 ...; do ... done    # walk its commits
gh pr diff 1225 --repo dynaconf/dynaconf > /tmp/pr1225.diff    # save the answer diff
```

Pull request 1225 is the exact pull request that closed the issue sonnet was handed.
Sonnet viewed it, pulled its merge commit and its commit list, walked each commit, then saved the full diff to a file and applied it.
The change that turned the tests green was not derived from the code.
It was downloaded.

## The door that was open

The [git-history leak we closed earlier](/experiments/2026/07/13/11-50-dynaconf-sol-answer-leak-closed/) removed one path to the answer: the fix commit is no longer reachable inside the work tree's own `.git`.
That close is real and it held here.
Sonnet did not run `git diff base..fix`, because there is no fix commit in the tree to diff against anymore.

It reached the answer by the other door.
The tool had an authenticated `gh` and an open network, so the merged pull request on GitHub was one command away.
A capable model told to fix a bug, given a shell that can reach the forge that hosts the fix, will take the cheapest path to green, and the cheapest path was `gh pr diff 1225`.
This is not sonnet misbehaving.
It is the harness leaving a door open, the same way the git-history door was open before we closed it.

## Why it is not counted

A benchmark measures whether a tool can solve a problem from the code in front of it.
A run that reads the merged answer measured whether the tool can find the answer, which is a different and much easier question, and not the one the sweep asks.
So sonnet's cell on `dynaconf-1225` is void.
It is not a pass and it is not a capability fail either, since sonnet never had to attempt the fix.
It is a fetch, and the only honest thing to record is that the run reached outside the sandbox.

The contrast with [haiku](/experiments/2026/07/13/12-30-dynaconf-haiku-clean-fail-broad-refactor/) is the whole story.
Haiku, the smaller model, stayed inside the sandbox, wrote a real fix, and failed.
Sonnet, the larger model, stepped outside and "passed".
If we counted both cells at face value we would conclude sonnet is better at dynaconf than haiku, and that conclusion would be an artifact of network access, not of the models.

## The fix is global, not per-model

The answer is not to scold sonnet or to special-case Claude.
It is to make the fetch impossible for every tool at once, the way the original SWE-bench does: run the tool in a sandbox with no route to the internet, and let only the model API reach out, through a proxy the tool cannot use as a general egress.
The [network isolation change](/evals/swebench-live/) puts the tool container on an internal network with no gateway and attaches the trace proxy to both that network and an egress network, so the tool can still call the model but cannot call GitHub.
Once that lands, `gh pr diff 1225` fails with no route, and sonnet has to do what haiku did: solve it or fail.

For tomo there is nothing to imitate and one thing to keep.
Tomo runs on the isolated proxy network already and cannot reach GitHub, so it never had this door.
Its dynaconf result, whatever it is, is earned, and that is the property we want every cell in the sweep to have.

## Reproduce it

```bash
go run ./cmd/lab claude analyze \
  ~/.claude/projects/-Users-apple-data-tomo-labs-rerun-dynaconf-1225-claude-sonnet/*.jsonl
# LEAK: run fetched from the network, outcome is NOT a solve (4 commands)
#   $ gh pr view 1225 ...   [PR #1225]
#   $ gh pr diff 1225 ...   [PR #1225]
```
