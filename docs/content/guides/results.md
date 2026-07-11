---
title: "Results"
description: "The full comparison across all eight wired tools on the same free deepseek model, with every column the harness records, the 00-hello baseline, and tomo's per-scenario breakdown."
weight: 20
---

Eight tools run the same fourteen scenarios against the same free deepseek model through the same trace proxy, so the only thing that differs below is the tool: how many turns it takes, how many tokens it burns getting there, how much memory it holds, and how big its install is. The model, the decoding settings, and the grading are held fixed for everyone.

This page is the full table with every column the harness records, not the trimmed version in the README. The numbers come straight from `lab report`, which keeps only the latest run of each scenario, so a tool's row is its current state over the fourteen scenarios, not a history that still counts runs it failed before an adapter bug got fixed. Run `lab report` yourself to reproduce any of it.

<!-- lab:results-snapshot:begin -->
Snapshot taken 2026-07-11. All 8 wired tools were rerun on 2026-07-11, each at the version shown, against the same OpenCode Zen deepseek endpoint with the same deterministic settings.
<!-- lab:results-snapshot:end -->

## How to read the table

- `pass` is how many of the fourteen scenarios the tool got a passing grade on, graded from the files left on disk.
- `1st` is how many of those it passed on the first attempt, before the best-of-three retry kicked in. `retried` is the rest.
- `plans` is how many scenarios the tool chose to lay out a plan or spawn a subagent on. It is a per-scenario choice, not a fixed capability, which is why even the planners plan on only a few of the fourteen.
- `tokens` is the total across all fourteen, so a bigger number means more work spent, not more runs recorded. `avg` is per scenario. `cache` is the share of prompt tokens served from the provider's cache.
- `cost` prices those tokens at DeepSeek's published paid rates. The runs themselves were free; this is the dollar figure the token gap becomes once you leave the free tier.
- `rss` is peak resident memory, `ttfb` is average time to first byte, `wall` is average wall-clock seconds per scenario, and `install` is the tool's own bytes on top of the shared base image.

The report splits the tools by how they work: the ones that ever lay out a plan or spawn a subagent, and the ones that run a single flat loop. Both tables are ordered by total tokens, cheapest first.

## Tools that plan

<!-- lab:results-plan:begin -->
| tool | version | pass | 1st | plans | tokens | avg | cache | cost | rss | ttfb | wall | install |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| tomo | v0.2.4 | 14/14 | 13 | 4/14 | 187,404 | 13,386 | 86% | $0.0267 | 23MB | 708ms | 10s | 21MB |
| opencode | 1.17.18 | 12/14 | 9 | 2/14 | 457,807 | 32,700 | 94% | $0.0515 | 620MB | 1854ms | 56s | 420MB |
| codex | 0.144.1 | 14/14 | 14 | 3/14 | 732,370 | 52,312 | 96% | $0.0663 | 95MB | 2041ms | 27s | 423MB |
| openclaw | 2026.6.11 | 14/14 | 14 | 1/14 | 1,095,701 | 78,264 | 87% | $0.1143 | 453MB | 2490ms | 61s | 407MB |
| hermes | 0.18.2 | 14/14 | 14 | 3/14 | 1,168,925 | 83,494 | 94% | $0.1064 | 132MB | 3140ms | 41s | 221MB |
| claude-code | 2.1.207 | 14/14 | 14 | 3/14 | 1,793,716 | 128,122 | 97% | $0.1498 | 290MB | 2946ms | 33s | 322MB |
<!-- lab:results-plan:end -->

## Tools that run flat

<!-- lab:results-flat:begin -->
| tool | version | pass | 1st | plans | tokens | avg | cache | cost | rss | ttfb | wall | install |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gemini-cli | 0.50.0 | 5/14 | 5 | 0/14 | 112,988 | 8,070 | 93% | $0.0114 | 260MB | 2181ms | 10s | 181MB |
| pi | 0.80.6 | 14/14 | 14 | 0/14 | 244,455 | 17,461 | 89% | $0.0329 | 159MB | 3163ms | 37s | 156MB |
<!-- lab:results-flat:end -->

Every version above is that tool's latest published release as of its run, checked against its npm or module registry directly, not a stale pin. `lab meta` captures the version and its release date after every build, so the labels never drift from what ran.

## What the numbers say

Token use is the headline, and cost is the same story in dollars. Among the tools that plan, tomo does all fourteen tasks in a fraction of the tokens: 187k total against 732k for codex, 1.10M for openclaw, 1.17M for hermes, and 1.79M for claude-code, which on the paid tier is under 3 cents against 7, 11, 11, and 15. It plans in context, updating one checklist in the same turn, rather than re-reading its own state in a fresh context per step. pi spends more than tomo but still runs lean. gemini-cli spends the fewest tokens of all, but it does not plan and drops nine of the fourteen scenarios, so its cheapness is mostly work it never finished.

Passing on the first try is its own axis, separate from passing at all. tomo passed thirteen of fourteen on the first attempt and needed one retry, on the invoice-join scenario. The v0.2.4 build is why: the agent now runs the project's tests before it calls a task done, and it is no longer cut off mid-answer by a low token cap, so a wrong first draft gets caught and fixed inside the same run instead of failing the grade. opencode is the other end of that column among the planners, nine first-try passes and five retries, and two scenarios it never got green.

Planning is a choice a tool makes per scenario, not a fixed capability, which is what the `plans` column shows: even the planners lay out a plan on only a few of the fourteen tasks and run the rest flat. openclaw is the clearest case. It carries a plan tool and a whole subagent layer but planned just one of fourteen until a prompt asked, in plain terms, for a live plan it kept current as it worked. The split into two tables is by whether a tool ever plans: tomo and the five others do on at least one scenario, pi and gemini-cli never.

Install footprint, not image size, is the honest size axis. Image size is dominated by the shared base every tool sits on (Python, Node, a Go toolchain), so it says more about the base than the tool. The install layer is the tool's own bytes on top of that base: 21MB for tomo's single static binary against 150 to 420MB for a Node dependency tree.

Time to first byte is bounded by the hosted model, the same upstream for every tool, so it is not a real axis of difference here. It clusters in the same couple of seconds for everyone. tomo's lower figure comes from shorter prompts, so the model spends less time reading before it starts answering, not from a faster server.

gemini-cli's 5/14 is mostly the model missing a step, not a wiring bug. It makes only two to three requests per scenario, so it rarely retries the way the others do, and it drops the multi-step scenarios where a plan would have kept it on track. Its wire translator works end to end. pi is the opposite kind of flat, a minimal harness that runs the whole task in one loop and passed every scenario cleanly. pi does ship a plan mode, but it is a read-only exploration extension gated behind an interactive prompt: it writes a prose plan and asks, in the TUI, whether to execute, rather than exposing a plan tool the model calls mid-run. In a one-shot headless run there is no prompt to answer and no plan tool to record, so pi stays flat here by design.

## The 00-hello baseline

The `00-hello` scenario is a baseline, just the prompt `Hi!`, isolating the fixed round-trip cost every tool pays before it does any real work: the system prompt, the tool schemas, and whatever preamble the harness wraps around a single turn.

<!-- lab:hello-baseline:begin -->
| tool | tokens | ttfb | rss |
| --- | --- | --- | --- |
| tomo | 1,585 | 704ms | 12MB |
| pi | 1,606 | 10859ms | 120MB |
| opencode | 7,260 | 1023ms | 660MB |
| codex | 7,596 | 914ms | 91MB |
| gemini-cli | 7,883 | 5800ms | 245MB |
| hermes | 13,619 | 8136ms | 121MB |
| openclaw | 16,755 | 1163ms | 366MB |
| claude-code | 19,178 | 7389ms | 290MB |
<!-- lab:hello-baseline:end -->

tomo pays the least to say hello: 1,585 tokens against 7,000 to 19,000 for the tools that ship a large standing prompt and a wide tool schema on every turn. That fixed cost is paid on every scenario, so a lean baseline compounds across the suite.

## tomo per scenario

The v0.2.4 sweep, scenario by scenario, so the total above is not a black box. Thirteen passed on the first attempt; invoice-join took a second.

<!-- lab:baseline-scenarios:begin -->
| scenario | tokens | attempt |
| --- | --- | --- |
| 00-hello | 1,585 | 1st |
| 10-reasoning-calc | 3,750 | 1st |
| 06-codegen-primes | 5,708 | 1st |
| 03-bugfix-fizzbuzz | 7,636 | 1st |
| 04-web-extract | 8,047 | 1st |
| 07-refactor-dedupe | 8,200 | 1st |
| 02-json-transform | 8,555 | 1st |
| 01-file-organize | 9,628 | 1st |
| 05-log-count | 10,539 | 1st |
| 08-data-summary | 10,660 | 1st |
| 09-project-scaffold | 16,835 | 1st |
| 11-storefront-budget | 24,627 | 1st |
| 13-release-fix | 33,740 | 1st |
| 12-invoice-join | 37,894 | 2nd |
<!-- lab:baseline-scenarios:end -->

The cost climbs with the shape of the task, not randomly: a bare greeting and a word problem sit at the bottom, and the two that fetch a page, join it with a local file, and get a test suite green sit at the top. Even the most expensive scenario here, at 38k tokens, is a fraction of what several tools spend on an average one.

## Reproduce it

```sh
go run ./cmd/lab build            # base, proxy, and every wired tool image
go run ./cmd/lab run tomo         # one tool through all fourteen scenarios
go run ./cmd/lab report           # the full table above
go run ./cmd/lab report --json    # the same numbers as JSON
go run ./cmd/lab report 00-hello  # one scenario across every tool
```

The scenario definitions are committed and graded by deterministic checkers, so a rerun on the same tool version and the same model lands on the same verdict. The only thing that moves the numbers is the tool.
