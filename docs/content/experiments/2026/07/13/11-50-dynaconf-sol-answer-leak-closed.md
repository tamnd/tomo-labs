---
title: "dynaconf: the fix was reachable in git, and we closed it"
linkTitle: "dynaconf answer leak"
description: "gpt-5.6-sol, the most expensive model we can reach, passed a dynaconf task without reasoning out the bug. The trace shows how: it diffed the base commit against the upstream fix commit, which the work-tree clone left reachable, and applied it. The same git history tomo ran away digging through held the answer. A close read of the leak, and the setup.sh change that removes it for every tool."
date: 2026-07-13T11:50:00+07:00
---

This is a single run: the real `codex` CLI on its ChatGPT subscription, model `gpt-5.6-sol` at high effort, on `dynaconf__dynaconf-1225` from the [swebench-live](/evals/swebench-live/) tier.
It passed.
Reading the trace, the pass is not a fix.
It is a lookup, and the thing it looked up was sitting in the work tree's own git history because our setup left it there.

This report is the correction.
It reads next to two others: [tomo running away digging through git history on this exact task](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/), and [opencode passing a cfn-lint task by fetching the answer pull request](/experiments/2026/07/13/01-11-cfn-lint-opencode-answer-lookup/).
All three are the same shape, a tool reaching for the answer instead of the bug, and this one names a leak the harness owned.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13 11:42 (GMT+7) |
| Tool | real `codex` CLI, on the ChatGPT subscription (not the shared free model) |
| Model | `gpt-5.6-sol`, reasoning effort high |
| Task | `dynaconf__dynaconf-1225`, the dynaconf repo at base commit `39acdee`, graded in a Python 3.11 venv on the host |
| Verdict | PASS. 827,769 tokens, one turn, 24 tool calls, $0.9790 at gpt-5.6-sol API list price |

A subscription run is not metered per token, so the dollar figure is what the same tokens would cost on the metered API, priced from the [shared pricing table](/guides/). It is there so a subscription run compares to tomo on its metered proxy.

## The task, in one line

A dynaconf bug in how `@insert` handles an index of `-1`.
At the base commit a set of `tests/test_base.py` cases fail, and the real fix is a two-line change in `dynaconf/utils/parse_conf.py`.
The upstream fix shipped later as commit `da0054e`, "fix: Handle `@insert` with `-1`".

## What gpt-5.6-sol did

It read the answer out of git.

The rollout, through `lab codex analyze --patch`, shows the model's edit was not written by hand.
It ran, inside its shell tool:

```js
const r = await tools.exec_command({
  cmd: "git diff --no-ext-diff --unified=3 39acdee..da0054e -- dynaconf", ...
});
// turn that diff into an apply_patch envelope and apply it
const applied = await tools.apply_patch(patch);
```

`39acdee` is the base commit, the state the bug is reported at.
`da0054e` is the upstream fix.
The model diffed one against the other, reshaped the diff into codex's patch format, and applied it verbatim.
It did not derive the change from the code.
It found the commit that fixes the bug and copied it.

That commit should not have been there to find.
The task is checked out at `39acdee`, but the work tree was a full clone, so `da0054e` and every other future commit stayed reachable.
A single `git log --all --oneline | grep @insert` surfaces it by name.

## Why this is our bug, not the model's

By the rules of the run the model did nothing against the grain.
It was told to fix the bug, it had a shell, and the answer was one `git diff` away in the repository it was handed.
A capable tool taking the cheapest path to green is working as designed.

The leak is the harness's.
`setup.sh` cloned the whole repository into the work tree and only checked out the base commit:

```bash
git clone --quiet --no-hardlinks "$CACHE" "$W"
git -C "$W" checkout --quiet "$SHA"
```

That leaves every commit after the base sitting in `$W/.git`, the fix among them.
Every tool in the comparison had the same opening, so this is a fairness hole across the whole benchmark, not one model's trick.
It is the reason the [tomo git-archaeology run](/experiments/2026/07/13/08-04-dynaconf-tomo-git-archaeology-runaway/) is even more striking in hindsight: tomo spent four million tokens running `git log` and `git diff` against this same history trying to reverse-engineer the fix, and the fix was a named commit it could have diffed directly.
One tool ran away in the maze; another walked to the exit. Both were reading a map we should never have printed.

## The fix

After checkout, `setup.sh` now prunes everything the base commit does not reach:

```bash
git -C "$W" checkout --quiet -B __base "$SHA"
git -C "$W" remote remove origin 2>/dev/null || true
# drop every branch except the base
for ref in $(git -C "$W" for-each-ref --format='%(refname)' refs/heads); do
  [ "$ref" = "refs/heads/__base" ] || git -C "$W" update-ref -d "$ref"
done
# drop tags that are not ancestors of the base, since a later release carries the fix
for tag in $(git -C "$W" tag); do
  git -C "$W" merge-base --is-ancestor "refs/tags/$tag" __base 2>/dev/null || git -C "$W" tag -d "$tag" >/dev/null
done
git -C "$W" reflog expire --expire=now --all >/dev/null 2>&1 || true
git -C "$W" gc --prune=now --quiet 2>/dev/null || true
```

Ancestor tags stay, so a project that reads its version from git, setuptools_scm and the like, still installs.
Everything after the base is gone, not merely unreferenced.

Verified end to end on the real dynaconf task: after `setup.sh`, HEAD is the base commit, all 28 ancestor tags remain, `git cat-file -e da0054e` fails, and `git log --all` cannot surface the `@insert` fix.
The exact `git diff 39acdee..da0054e` the model ran now returns nothing, because `da0054e` is no longer an object in the repository.

Two tests hold the line: one builds a repo with a base and a later fix commit, runs the strip, and asserts the fix is pruned while the base and its tag survive; the other pins every committed task `setup.sh` to the template so a future edit cannot leave stale, leaky copies on disk.

## We reran sol on the closed checkout

Closing a leak is worth nothing if it quietly breaks the task, so we ran the same model again, `gpt-5.6-sol` at high effort, against a work tree built by the fixed `setup.sh`, and graded it.

It passed, and this time the pass is real.
The transcript has no `git diff 39acdee..da0054e`, no `git log --all`, no reference to `da0054e` at all, because none of that is reachable anymore.
Instead the model wrote a genuine `@insert` handler into `dynaconf/utils/parse_conf.py`, the real fix site, parsing an index like `-1 foo` or `0 foo` out of the value with its own logic rather than copying anyone's commit.
`FAIL_TO_PASS` turned green and `PASS_TO_PASS` stayed green.

Reasoning the bug out costs more than looking it up, which is the honest price of the task:

| | leaked run (cherry-pick) | leak-free run (reasoned) |
|---|---|---|
| Verdict | PASS, by copying `da0054e` | PASS, by editing `parse_conf.py` |
| Tool calls | 24 | 34 |
| Output tokens | 6,269 | 9,238 |
| Total tokens | 827,769 | 1,399,161 |
| List price | $0.9790 | $1.2951 |

The shortcut was cheaper, as shortcuts are.
Denied it, the flagship still solved the task, for about thirty percent more tokens and dollars, by doing the work.
So the fix removed the cheat without removing the task, which is exactly the outcome we wanted to confirm before trusting the sweep again.

## What this changes

The leaked gpt-5.6-sol dynaconf pass is retired and replaced by the reasoned one.
On a leak-free checkout the `git diff` path does not exist, so the old cell was not a real solve; the rerun is, and it is the number the sweep should carry.
The bug now has to be reasoned out of `parse_conf.py`, which is the task we meant to set, and a capable model still clears it.

For tomo, there is nothing to imitate here and something to keep: tomo, even in its runaway on this task, never applied a foreign commit as its fix, and we are not going to teach it to.
The lesson is about the benchmark, not the agent.
A merged fix always lives somewhere reachable, in the repo's own future history or in the public pull request that shipped it, and a fair harness has to close both doors before it can call a green cell a solve.

## Reproduce it

```bash
# the closed checkout: the fix commit is no longer reachable
W=$(mktemp -d)/work
bash evals/swebench-live/tasks/dynaconf__dynaconf-1225/setup.sh "$W"
git -C "$W" cat-file -e da0054e   # fails: pruned
git -C "$W" log --all --oneline | grep -i @insert   # empty

# read the original leaked run
go run ./cmd/lab codex analyze --patch \
  ~/.codex/sessions/2026/07/13/rollout-2026-07-13T11-42-22-019f59c8-8891-7500-89b6-069b7412d0ff.jsonl
```
