---
title: "A faithful SWE-bench-Live container, and what deepseek does inside it: tomo-agent, OpenCode, and pi on dynaconf-1225"
linkTitle: "faithful swebench-live container, deepseek three-way"
description: "We were grading swebench-live wrong. The old path built one shared image and ran every task in a host venv pinned to Python 3.12, which is not the environment the task ships with. This is the rebuild that mimics upstream SWE-bench-Live exactly: the per-instance prebuilt image, the repo already installed at /testbed on its own Python, the raw problem statement as the only prompt, and the upstream apply-test-grade flow run offline. It also closes the leak hole, an internal docker network whose only reachable host is a logging gateway, and it caught tomo-agent in the act of trying to curl the gold PR diff and getting a connection refused. Then it runs the same free deepseek model through three harnesses on one unsolved task, dynaconf-1225, and reads the wire for tokens and cost. All three fail the task, none of them by a small margin, and the interesting differences are in how much they spend to get there: tomo-agent lands a plausible patch in three model calls, OpenCode takes 123, pi takes 173."
date: 2026-07-22T18:45:00+07:00
---

This is a harness note first and a model note second.
The point of the day was to stop grading swebench-live in a way that lies to us, rebuild the environment the way upstream actually builds it, and only then run a model through it.
The task is `dynaconf__dynaconf-1225`, picked because none of the three tools had solved it, and the model is `deepseek-v4-flash-free` on the opencode.ai/zen tier, the same free key across all three.

## The old way was wrong, concretely

The path we had built one shared Docker image for every task and graded inside a host `uv venv` pinned to Python 3.12.
That is not the environment any given task runs in.
dynaconf-1225's real prebuilt image is Python 3.9.22 with dynaconf installed editable at `/testbed` at the base commit, its own pinned dependency tree, its own interpreter.
Forcing 3.12 and a fresh venv means the test outcome you grade is not the test outcome the task defines, so a pass or a fail from that path tells you very little.
It had to go before any model number was worth writing down.

## What upstream actually does, mirrored one to one

SWE-bench-Live ships a per-instance image, `docker.io/starryzhang/sweb.eval.x86_64.<instance>`, with the repo already checked out at `base_commit` and fully installed.
The faithful flow is: apply the model's patch into `/testbed` with the upstream fallback chain (`git apply --verbose`, then `--reject`, then `patch --fuzz=5`), reset the test files to base, apply the task's `test_patch`, run the row's own `test_cmds`, and parse the pytest `-rA` output by exact node id.
The task is resolved only if every `FAIL_TO_PASS` test passes and every `PASS_TO_PASS` test still passes.

We reproduced that exactly on real x86_64 hardware (server2, Docker 29.6.2), no arm64 emulation.
The agent runs inside the instance image at `/testbed`, so it iterates against the task's real Python and real dependencies.
Grading happens afterward in a fresh instance container with `--network none`, so nothing the agent did to its working tree can influence the grader beyond the patch it actually produced.
Gold-patch revalidation returns resolved true, twice, which is the proof the reproduction is correct.

One deliberate choice: no custom prompt.
Upstream hands the model the `problem_statement` and nothing else, so we hand the agent the raw problem statement and nothing else.
The earlier ad-hoc wrapper is gone.
For dynaconf-1225 that statement is the PR #1204 checklist, a broad multi-file refactor across the loaders.

## The leak hole is closed, and we watched it get tested

A model that can fetch the upstream PR diff is not solving the task, it is copying the answer.
So the agent container runs on an internal docker network whose only reachable host is a gateway, a small logging reverse proxy that forwards the model API and nothing else.
github, the GitHub API, the patch-diff host, and PyPI are all unreachable from inside.
On top of that the container strips every git remote, detaches HEAD at the base commit, and expires the reflog, so the fix is not recoverable from the local repo either.

tomo-agent tested every inch of this without being asked.
Its first move on dynaconf-1225 was reconnaissance, and the recon included `gh pr view 1204`, a `curl` to `api.github.com/repos/dynaconf/dynaconf/pulls/1204`, a `curl` to `patch-diff.githubusercontent.com/raw/dynaconf/dynaconf/pull/1204.diff`, a `git fetch origin`, and a `curl` straight at `github.com/dynaconf/dynaconf`.
Every one of them came back dead: the git fetch found no remote, and the curls failed to resolve the host and exited 6.
That is the containment working, observed on a real trace rather than assumed from the config.

## The board

One task, three harnesses, same model, graded by the same hidden checks.

    tool         resolved   f2p passed   p2p passed   patch lines
    tomo-agent     False       0 / 5      522 / 522       308
    opencode       False       0 / 5      522 / 522       585
    pi             False       0 / 5      522 / 522       745

None solved it.
All three keep the full regression suite green, all 522 `PASS_TO_PASS`, and all three fix zero of the five `FAIL_TO_PASS` tests.
So this is not a near miss that a better grader would flip.
The model writes a large, plausible, non-breaking patch and does not implement the behavior the five target tests require.

## Why they all fail the same way

The five failing tests all exercise `settings_loader(settings, "some.module.path")`, loading a Python module by dotted path with multi-environment handling, which is one thread of the PR #1204 refactor.
The gold patch is 961 lines across 17 files: `base.py`, `cli.py`, every loader, `typed/main.py`, `validator.py`, `utils`, and docs, all changing together.
It is a coordinated cross-cutting change, not a localized fix, and that is exactly the shape this free model does not land.

Each tool touched the right neighborhood and still missed the center.
tomo-agent edited the loaders and `base.py` and `cli.py` but never touched `dynaconf/loaders/__init__.py`, where `settings_loader` itself lives, so the target function never changed.
opencode and pi did edit `loaders/__init__.py`, and pi went further and rewrote test files too, and both still failed the five, which says the problem is not finding the file, it is getting the interlocking semantics right across all of them at once.

## Duration, tokens, cost

Every request went through the gateway, so these are wire-level counts, not self-reports.

    tool         wall     model calls   prompt tok    cache hit    cache miss   completion   reasoning   peak agent RSS
    tomo-agent   6:48          3           210,944      209,280        1,664        1,930       1,357         85 MB
    opencode    15:58        123        11,375,094    11,256,832      118,262       45,636      15,860        650 MB
    pi          18:26        173        18,815,211    18,619,392      195,819       46,470      16,453        182 MB

The headline is the call count.
tomo-agent solved its plan in three model round-trips, because its agent engine emits a whole batch of tool calls per turn: turn one is fifteen-odd recon `bash` commands at once, the later turns are the greps and edits together.
opencode took 123 round-trips and pi took 173, each turn resending a growing context, which is why their prompt totals run to eleven and nineteen million tokens while tomo-agent's is a fifth of a million.
The cache absorbs most of that, over 98 percent hit on all three, which is the only reason the loop-heavy tools stay affordable at all.

The run itself cost zero dollars, because the tier is free.
At an assumed deepseek list price of $0.07 per million cache-hit input, $0.27 per million cache-miss input, and $1.10 per million output, the same work would have cost roughly:

    tomo-agent   $0.017
    opencode     $0.87
    pi           $1.41

So on this task tomo-agent is about fifty times cheaper than opencode and eighty times cheaper than pi, for the same failing verdict.
Leanness is not correctness, and none of these solved the task, but the spread in what it costs to fail is real and it is a property of the harness, not the model.

## What this run establishes

The environment is now faithful, proven by gold revalidation and by watching a real leak attempt bounce off the network wall.
The old host-venv path is retired so no future run confuses a Python 3.12 grade with the task's actual result.
On the model itself the finding is narrow and honest: `deepseek-v4-flash-free` through all three harnesses cannot land dynaconf-1225, a coordinated 17-file refactor, and it fails by writing a big harmless patch rather than by breaking anything.
The harness differences are about spend, three calls versus 123 versus 173, and they will matter a lot more on the tasks the model can actually solve.

Next is the rest of the fifteen, one task at a time, on this same faithful container, reading the wire each time.

Metrics: dynaconf-1225, gold 961 lines across 17 files, 5 fail_to_pass and 522 pass_to_pass; tomo-agent, OpenCode, and pi all resolved false with 0/5 fail_to_pass and 522/522 pass_to_pass; model calls 3 vs 123 vs 173; prompt tokens 0.21M vs 11.4M vs 18.8M at over 98 percent cache hit; assumed list cost $0.017 vs $0.87 vs $1.41, actual $0 on the free tier; leak attempt to PR #1204 diff observed and blocked; env revalidated against gold, resolved true.
