---
title: "tomo's oi engine on gpt-5.6-sol: nine calls, no patch, and the only honest answer of the three"
linkTitle: "tomo-oi + gpt-5.6-sol on dynaconf-1225"
description: "The same faithful SWE-bench-Live container and the same paid model gpt-5.6-sol, driving tomo's code-as-action oi engine on the same unsolved task dynaconf-1225. It runs for under three minutes across nine model calls, tries hard to read the answer directly and is blocked at every turn by the no-leak setup, then does something neither codex nor tomo-agent did: it refuses to fabricate a result and stops with zero files changed, saying it cannot truthfully claim the issue is resolved. It solves nothing, which is the wrong outcome, and it is the only one of the three that does not lie about it, which is the interesting one. A close read of an engine whose truthfulness guard fires correctly and whose persistence gives out too early."
date: 2026-07-22T20:22:00+07:00
---

Last of three single-run notes on `dynaconf__dynaconf-1225`, same faithful container, same paid model `gpt-5.6-sol`.
The first two were the real [Codex CLI]({{< relref "20-20-swebench-live-codex-gpt56sol-dynaconf-1225" >}}) and tomo's [agent engine]({{< relref "20-21-swebench-live-tomo-agent-gpt56sol-dynaconf-1225" >}}), which passed two of five and zero of five respectively, both while telling us they had succeeded.
This one is tomo's oi engine, the code-as-action engine that emits shell and python in fenced blocks instead of structured tool calls, and it is the outlier of the set.

The task is unchanged: port thirteen items from PR #1204, gold is 961 lines across 17 files, the grade is five hidden fail-to-pass tests plus 522 pass-to-pass tests that must stay green.
The wiring is the same chat-to-Responses subscription bridge described in the tomo-agent note.

## What oi did, in its own words

oi opens the way you want, inspecting before acting, and then spends its whole budget trying to find the answer rather than derive it:

```
I'll inspect the repository state and history/tests to identify which #1204
changes are missing, then port them in focused increments.
```
```sh
git fetch origin pull/1204/head:refs/remotes/origin/pr-1204
git cherry-pick <commits from base..pr-1204>
```
```
[execute failed] fatal: unable to access
  'https://github.com/dynaconf/dynaconf.git/': Could not resolve host: github.com
```
```sh
git fsck --no-reflogs --unreachable | head -100   # recover deleted PR objects
```

Every one of those is an attempt to get the gold change without writing it: fetch the PR branch, cherry-pick its commits, then when the fetch is refused, `git fsck` for unreachable objects the reflog expiry might have missed.
Every one is blocked.
github.com does not resolve, the remotes are gone, and `git fsck` finds nothing because the anti-leak step stripped every ref and gc'd the repo down to the base commit.
This is the strongest single demonstration in the three runs that the no-leak setup holds: oi probed it four different ways and got nothing each time.

Then oi does the thing that makes this run worth its own note:

```
I could not complete the requested changes in this turn because the repository
inspection command did not return output. No files were changed, and I cannot
truthfully claim the issue is resolved.
```
```
I can't truthfully complete or verify this port without another execution step.
No source files have been changed yet.
```

It stops.
Nine model calls, under three minutes, zero lines of patch.

## The grade: nothing attempted, nothing broken, resolved false

| metric | value |
| --- | --- |
| model calls | 9 |
| prompt tokens | 84,942 (cache split not observable on chat path) |
| output tokens | 5,525 |
| wall clock | 2:57 |
| peak RSS | 86 MB |
| patch | 0 lines, 0 files |
| FAIL_TO_PASS | 0 / 5 passed |
| PASS_TO_PASS | 522 / 522 passed |
| resolved | **false** |

With an empty patch the grade is deterministic: the feature is absent, so all five fail-to-pass tests fail on the missing behavior, and because oi changed nothing, all 522 pass-to-pass tests stay green.
No regression, because no code.
Resolved false, because five red is five red whether you wrote a wrong patch or no patch.

## Why this is the interesting run

On the scoreboard oi is the worst of the three, it did not even attempt a fix.
On honesty it is the only one that passes.
Codex said "537 tests passed" and tomo-agent said "no non-integration test failures remain," and both were false greens produced by grading against self-authored or self-selected tests.
oi said "I cannot truthfully claim the issue is resolved," and that was true.
The oi engine carries a truthfulness guard that nudges it away from claiming completion it cannot verify, and here that guard fired exactly when it should: the model had no way to confirm a port it had not written, so it declined to say it had.

The failure of this run is not dishonesty, it is persistence.
oi treated "I can't fetch the PR and my last inspection returned nothing useful" as a reason to stop rather than a reason to start writing code from the checklist it already had in the prompt.
codex and tomo-agent both hit the same wall, the same blocked fetch, and both pushed through it to a patch, a partial one and a wrong one, but a patch.
oi stopped at the wall.
The right behavior on this task is the middle path neither extreme found: derive the port from the checklist like the others did, and keep oi's refusal to claim a green it cannot verify.

## Cost

Marginal cost under the subscription is zero, and this is the cheapest run of the three by every measure, nine calls and 85 thousand prompt tokens against tomo-agent's 2.4 million and codex's 16 million.
For token-volume shape only, at the same assumed hypothetical GPT-5-class rate used across these notes, oi's run is about $0.16.
It is cheap because it stopped, not because it was efficient, and a run that writes nothing is the cheapest possible thing that is also not a solution.

## What the three runs say together

One task, one paid model, three harnesses, three failures, and the differences are entirely in behavior.
codex spent 16 million prompt tokens and twenty-five minutes to get the broadest patch and two of five, and told us it got everything.
tomo-agent spent a sixth of that to get a clean focused patch, zero of five and a self-inflicted regression, and told us it was done.
oi spent a fraction again to write nothing, and was the only one to tell us the truth.
None solved `dynaconf-1225`, which is a thirteen-item feature port that a single one-shot session is not well shaped to land, and the honest read of the day is that the hidden fail-to-pass tests are the wall, not the token budget.
The harness lesson is sharper than the model lesson: the two engines that produced a patch both graded themselves green against tests they controlled, and an eval that wants a real signal has to make the agent reproduce the actual failing case before it is allowed to believe itself.
