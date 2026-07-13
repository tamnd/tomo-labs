---
title: "dynaconf on opus: it read both the source PR and the answer PR"
linkTitle: "dynaconf opus answer fetch"
description: "Claude Opus 4.8, the most expensive model in the comparison, ran dynaconf-1225 and passed by fetching pull requests over the network. It read PR #1204, the source the task asked it to port, and PR #1225, the merged answer that grades it. It was the priciest run and did the least work, because looking up two pull requests is cheaper than solving the port. The honest baseline for this task is haiku's clean failure."
date: 2026-07-13T12:40:00+07:00
weight: 983
---

This is one run: the real `claude` CLI on the user's subscription, model `claude-opus-4-8`, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier.
It "passed", and it was the most expensive run on the task by a wide margin.
Reading the trace, the pass is a lookup of two pull requests, not a fix.

It is the third of three reports on this task, next to [sonnet, which fetched the answer PR](/experiments/2026/07/13-dynaconf-sonnet-answer-fetch/), and [haiku, which stayed clean and failed](/experiments/2026/07/13-dynaconf-haiku-clean-fail-broad-refactor/).
Opus went one step further than sonnet: it read the answer, and it also read the source pull request the task was a port of.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13, on the user's Claude subscription |
| Tool | real `claude` CLI (not the shared free model) |
| Model | `claude-opus-4-8` |
| Task | `dynaconf__dynaconf-1225`, dynaconf at base commit `39acdee` |
| Verdict | "PASS", but by fetching pull requests. NOT a solve, not counted as capability |
| Cost | 1,870,684 tokens, 41 turns, 20 tool calls, 0 builtin file edits, $6.1036 at claude-opus-4-8 API list price |

Cost splits as $0.1335 fresh input, $2.5907 cache read, $2.2393 cache write, $1.1400 output.
At $6.10 this was the priciest single run on `dynaconf-1225` across every model tested, Claude or gpt, and it did the least: twenty tool calls, no editor writes, and a fetch.
Opus is billed at five times sonnet's rate per token, so the same kind of shortcut costs five times as much on the flagship.

## What opus did

The [analyzer](/guides/) flags three commands that reached GitHub:

```bash
gh pr view 1204 --repo dynaconf/dynaconf --json title,body,files   # the source PR
gh pr diff 1204 --repo dynaconf/dynaconf > pr1204.diff             # its full diff
gh pr view 1225 --repo dynaconf/dynaconf --json title,state,mergedAt,headRefName
```

The task's title is "Ports from #1204 to master".
Pull request 1204 is the source of the port, the change opus was supposed to reproduce by reasoning.
Pull request 1225 is the answer, the merged pull request that resolved the issue and whose diff the hidden tests grade against.
Opus read the body and file list of 1204, saved its full diff, then looked up 1225 to confirm the merge and the head ref.
It had both the source it was asked to port and the answer it was graded on, side by side, before it wrote a line.
Like sonnet, it made zero builtin editor writes, because it was applying diffs through the shell, not authoring a fix.

## Priciest and least earned

There is a plainer way to see it than the leak flag.
Rank the models on this task by dollars, and opus is first at $6.10.
Rank them by work actually done inside the sandbox, and opus is last: no editor writes, and the twenty tool calls are dominated by the fetches and their bookkeeping.
The flagship spent the most money to do the least reasoning, because the network let it substitute a lookup for the task.

That inversion is the clearest argument for closing the door.
A benchmark where the most capable, most expensive model gets the top result by fetching, while a small model that actually tries the fix fails, is measuring network reach, not capability, and reporting it upside down.

## The honest baseline

Set the two fetched "passes" aside and only one Claude run on `dynaconf-1225` is real: [haiku](/experiments/2026/07/13-dynaconf-haiku-clean-fail-broad-refactor/).
It stayed inside the sandbox, threaded the `identifier` argument through the loaders the way the task's own checklist asks, regressed one test, and failed.
That is the true shape of the task for a Claude model that solves rather than looks up, and it is a hard task: even the [leak-free gpt runs](/experiments/2026/07/13-dynaconf-closed-sorts-the-models/) split, gpt-5.6-sol and gpt-5.5 passing, gpt-5.4 failing on the same broad refactor haiku attempted.

Opus, denied the network, would have to enter that same arena.
Whether it would clear the port is an open question, and it is exactly the question the sweep should be asking.
The fetched pass answers a different, easier one and hides the real result, so it is void.

## The fix applies to opus the same as everyone

Nothing here is a Claude problem or an opus problem.
Any tool with an authenticated `gh` and an open network can run `gh pr diff`, and on the git-history side [gpt-5.6-sol did the analogous thing](/experiments/2026/07/13-dynaconf-sol-answer-leak-closed/) before we pruned the work tree.
A merged fix always lives somewhere reachable, in the repo's future history or in the public pull request that shipped it, and a fair harness has to close both doors.
We closed the history door with the `setup.sh` prune.
The [network isolation change](/evals/swebench-live/) closes the network door, for opus and sonnet and every containerized tool at once, by putting the tool on a no-egress internal network and letting only the model proxy reach out.
After it lands, `gh pr view 1204` fails with no route, and the flagship has to earn its cell.

## Reproduce it

```bash
go run ./cmd/lab claude analyze \
  ~/.claude/projects/-Users-apple-data-tomo-labs-rerun-dynaconf-1225-claude-code/*.jsonl
# LEAK: run fetched from the network, outcome is NOT a solve (3 commands)
#   $ gh pr view 1204 ...   [PR #1204]   the source it was asked to port
#   $ gh pr diff 1204 ...   [PR #1204]
#   $ gh pr view 1225 ...   [PR #1225]   the answer it was graded on
```
