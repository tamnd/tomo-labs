---
title: "Results"
description: "The current comparison table across all seven wired tools, and the 00-hello baseline that isolates each tool's fixed round-trip cost."
weight: 10
---

Seven tools against the same free deepseek model through the same trace proxy, so what differs below is the tool, not the model. `lab report` reads every run ever captured, so a tool's row is its full history, including scenarios it failed before an adapter bug got fixed, not just one clean sweep.

| tool | version | released | pass | avg tokens | avg ttfb | install |
| --- | --- | --- | --- | --- | --- | --- |
| tomo | v0.2.2-0.20260709142456-c1a34b365454 | 2026-07-09 | 11/11 | 5,379 | 751ms | 21MB |
| codex | 0.143.0 | 2026-07-08 | 12/12 | 15,105 | 902ms | 423MB |
| opencode | 1.17.16 | 2026-07-09 | 11/11 | 26,227 | 820ms | 420MB |
| claude-code | 2.1.205 | 2026-07-08 | 12/13 | 63,747 | 1205ms | 322MB |
| openclaw | 2026.6.11 | 2026-06-30 | 11/11 | 56,234 | 1165ms | 407MB |
| hermes | 0.18.2 | 2026-07-08 | 14/24 | 25,834 | 1015ms | 221MB |
| gemini-cli | 0.50.0 | 2026-07-08 | 8/14 | 6,885 | 897ms | 181MB |

Every version above is that tool's latest published release as of the run, checked against its npm or module registry directly, not a stale pin. `lab meta` captures the version and release date after every build so the table never drifts from what actually ran; run `lab report` yourself for the full columns (cache hit rate, cost, RSS, wall time).

A few of these deserve a note.

Token use is the headline: tomo does the same tasks in a fraction of the tokens of every other tool here, because it takes fewer, cleaner turns rather than re-reading its own context on every step.

Install footprint, not image size, is the honest size axis. Image size is dominated by the shared base every tool sits on (Python, Node, a Go toolchain), so it says more about the base than the tool. The install layer is the tool's own bytes on top of that base: 21MB for tomo's single static binary against hundreds of megabytes for a Node dependency tree.

Time to first byte is bounded by the hosted model, which is the same upstream for every tool, so the gap you might hope for is not on the table here. tomo is still fastest because its prompts are shorter, so the model spends less time reading before it starts answering.

hermes and gemini-cli's pass counts include runs recorded while their adapters were still broken. hermes shipped a custom provider that silently dropped the API key until its adapter learned to set it explicitly. gemini-cli needs `~/.gemini/settings.json` written with an explicit auth type, or its headless mode falls back to an interactive prompt that never resolves. Both wire translators work end to end now: hermes passes its scenarios cleanly, and gemini-cli's remaining failures are the model missing a step, not a wiring bug, it makes only 2 to 3 requests per scenario against 20 to 30 for tomo or hermes, so it rarely retries the way the others do.

## The Hi! baseline

The `00-hello` scenario is a baseline, just the prompt `Hi!`, isolating the fixed round-trip cost every tool pays before it does any real work. See the [Hi! baseline results](https://github.com/tamnd/tomo#the-hi-baseline) in tomo's own README for that table; it lives there since it's the number tomo's README leads with.
