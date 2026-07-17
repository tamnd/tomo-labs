---
title: "M0 slice zero: the experiment estate audited against git"
linkTitle: "M0 slice zero estate audit"
description: "Spec 2105's M0 starts by reconciling what the June experiment journal says shipped against what the tomo tree actually carries. The audit checked one identifiable symbol per patch set across the committed tree and git log -S. Five of the six patch sets landed whole through the #71 squash, including three the journal still flagged as not committed. Two real gaps remained: the anthropic provider never got the gateway-worded 400 retry its openai twin ships, and the default engine still cut long tool results head-only at the 48KB backstop. Both landed as tomo #75 and #76, the latter as one shared head-plus-tail clamp now used by all three engines. A fresh core-14 baseline on deepseek-v4-flash-free pins the pre-M0 state with concrete pass and token numbers."
date: 2026-07-17T18:05:00+07:00
---

M0 of spec 2105 is called land the estate: before any new mechanism, make the tree actually contain what the experiment campaign proved.
The journal is honest but it trails the tree, because several experiment notes were written while a patch sat on a branch that later squash-merged.
So the slice-zero audit trusts neither the journal nor memory; it checks one identifiable symbol per patch set against the committed tree and `git log -S`.

## Method

Each June patch set has a symbol that only exists if the patch landed.
The audit greps the tree at main for the symbol, then asks `git log -S` which commit introduced it.
The tomo tree audited is main at 69ebbaf; the labs halves of each mechanism were checked in tomo-labs the same way.

## Verdicts

| set | mechanism | journal claim | git verdict | landed in |
|---|---|---|---|---|
| P1 | oi glued-fence split (`reopenAfterClose`) + oi head-and-tail output clamp | shipped (exp 0052) | landed | tomo #71 (e07694d) |
| P2 | gateway-worded 400 retried as transient (`retryableStatus`) | shipped, openai path (exp 0053) | half landed; anthropic twin missing | openai in #71, anthropic in #75 (557be53) |
| P3 | dialect zoo costume salvage (`parseExecuteXML` et al) + prose-hallucination guard (`actingMarkers`) | shipped (exp 0052, 0054) | landed | tomo #71, #72, #73 |
| P4 | dropped-file-block guard (`droppedBlockNudge`) | NOT committed (exp 0056) | landed; the journal flag went stale when #71 squashed the branch | tomo #71 |
| P5 | defensive value-threading paragraph in the oi system prompt | NOT committed (exp 0057) | landed inside the #71 squash (branch commit f5c15e5 absorbed) | tomo #71 |
| P6 | `prompt_cache_key` routing pin on the openai path | NOT committed (exp 0050) | landed | tomo #71 |
| PR 8 | default-engine 48KB tool-result backstop cut head-only | gap claimed by spec 2105 doc 01 | real: pkg/agent/agent.go kept the head and ate the verdict | tomo #76 (69ebbaf) |

Three of the journal's NOT-committed flags were stale in the same direction: the work rode the oi engine branch and landed when #71 squashed it.
The lesson for the journal habit is that a shipped-or-not flag written mid-campaign has to be re-checked once the branch merges, which is exactly why M0 makes this audit a slice.

## What actually needed landing

Two mechanisms were genuinely missing, and both are now on main.

Tomo #75 gives the anthropic provider the same rule the openai path has carried since exp 0053: a 4xx whose body names a gateway or upstream failure is the proxy hiccuping, not the request being malformed, so it retries instead of sinking the turn.
The error path now reads the response body and feeds it through the shared `retryableStatus`, with two unit tests: the zen-style 400 with an Upstream request failed body must come back transient, and a malformed 400 must stay permanent.

Tomo #76 replaces the default engine's head-only cut at the 48KB backstop with one shared `tool.Clamp(s, max, advice)` in pkg/tool.
A long tool result front-loads its frame and back-loads its verdict, so a head-only cut throws away exactly the line a verify-to-green loop needs.
The clamp keeps three quarters head and one quarter tail, backs both cut points off to line boundaries, and names the elided byte count in the middle.
cx's clampResult and oi's clampOutput now delegate to it, so the three engines bound results with the same shape, and oi keeps its re-run-narrower advice string through the advice parameter.

## Pre-M0 baseline: core-14 on deepseek-v4-flash-free

The baseline anchors the exit gate: post-M0 smoke must not pass fewer core-14 scenarios than this.
Image pins at baseline: tomo@92b5e82, tomo-cx@71b6e30, tomo-oi@402e0dd, all pre-#75.
Runs are pass at one, greedy decoding through the proxy, free zen tier.

| tool | pass | failed | tokens total | tokens fresh | requests | wall |
|---|---|---|---|---|---|---|
| tomo | 14/14 | none | 263,764 | 20,436 | 90 | 165s |
| tomo-cx | 14/14 | none | 275,008 | 19,904 | 83 | 160s |
| tomo-oi | 13/14 | 07-refactor-dedupe | 125,184 | 47,616 | 68 | 1000s |

Tokens total is the billed sum including cache reads; fresh is prompt minus cached plus completion, the number that actually costs money on a caching provider.
oi's shape is the known one: half the total tokens of the structured-tool engines, more of them fresh, and a much longer wall on the two long scenarios.
The single oi failure is 07-refactor-dedupe at pass at one on the free tier; the same scenario passed on the same model in the two prior recorded runs (Jul 16 and early Jul 17), so it reads as variance, and the exit-gate comparison is on pass count, not on which scenario wobbles.

Reproduce: `for t in tomo tomo-cx tomo-oi; do go run ./cmd/lab run $t; done` at tomo-labs 12f848c with the pins above.

## What this closes and what is next

This is M0 PR 1 (the audit note), PR 2 (anthropic twin, tomo #75) and PR 8 (shared clamp, tomo #76) of spec 2105 doc 10.
Tracking lives in tomo issue #77.
Next: bump the three tool image pins to tomo 69ebbaf, rebuild, run the post-M0 core-14 smoke against this baseline, then the three journal flips at the new pin (smolagents-285 glued fence, fonttools prose-hallucination guard, gitingest-94 costume salvage at 6 of 8 or better).
