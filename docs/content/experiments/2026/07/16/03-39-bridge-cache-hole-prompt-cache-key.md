---
title: "Closing the bridge cache hole with a stable prompt_cache_key"
linkTitle: "bridge cache hole and prompt_cache_key"
description: "The 2026-07-15 dynaconf run named the bridge's zero percent cache-read rate as its most actionable finding, so this run tries to close it. tomo's OpenAI client now sends a stable prompt_cache_key, a short sha256 of the system prompt plus the tool set, so a provider can route a run's repeated prefix to one cache backend, and the codex bridge now forwards that key through to the Responses backend and surfaces the cached-token count back. The lever is a lossless, unit-tested change that cannot alter a single output token. A two-arm A/B on the real codex bridge came back null, 86.7 versus 87.5 percent hit at n=1, because the backend already caches around 87 percent keylessly and has no routing scatter for the key to fix. The change stays as a no-regret no-op, not a token cut, with the multi-machine OpenAI regime left honestly unmeasured on this single-process bridge."
date: 2026-07-16T03:39:00+07:00
---

This is a direct follow-up to the [dynaconf cost and caching run](/experiments/).
That run measured the codex bridge serving zero percent of the resent history from cache and called the zero the most actionable thing in it.
This run closes that hole and then measures whether closing it buys anything.

The short answer is that the hole is now closed and the token win it promised did not show up on tomo's own benchmark path.
The longer answer is why, and it is worth the space.

## Setup

Start from the gap, proven on the live off-arm trace of `dynaconf-1225` through the `gpt-5.6-luna` bridge.
The request is byte-perfect for caching across all 55 rounds: the system prompt is one distinct value, the tools array is one distinct value, and the messages list is strictly append-only, so for every round `k` the messages `0..n-1` are byte-identical to round `k-1` with only new ones appended.
For a strictly append-only prefix the fresh cost telescopes to the final transcript once, a floor of 112,392 tokens, which is a 97.1 percent ideal cache hit.
The run actually billed 581,850 fresh tokens, an 84.3 percent hit.
That is 469,458 excess fresh tokens on a single task, about 4.2 times the append-only floor, on bytes that were cacheable and were not cached.

The prefix is already optimal, so the miss is in how identical bytes get billed, not in what tomo sends.
Automatic prompt caching is per-machine, so without a routing hint a provider scatters a run's rounds across backends and each new backend cold-misses the shared prefix even though the bytes match.
The Anthropic path in `pkg/provider/anthropic.go` already handles this with the two standard `cache_control: ephemeral` breakpoints, but the benchmark and the bridge both speak the OpenAI dialect, which carried no cache hint at all.

The change is in `pkg/provider/openai.go`, which now sends `prompt_cache_key`, the field OpenAI documents for cache routing, derived from the parts of the request that stay byte-identical across a run: a short sha256 of the system prompt plus the tool set.
It is lossless because it is a hash that carries no message content and never reaches the model, stable within a run because system and tools are constant, sensitive across configs because a changed prompt or tool set yields a new key, and ignored where unsupported because a server that does not know the field drops it and returns 200.

The bridge had a second gap.
The flat-rate codex bridge in `tomo-labs/cmd/lab/bridge_wire.go` rebuilt each chat request from a fixed field set and dropped `prompt_cache_key` entirely, so a benchmark run could never carry the hint to the backend even though the Responses API keys its prompt cache on exactly that field.
The bridge now forwards the key when present and omits it when absent.

The two-arm A/B ran the same task and model back-to-back, differing only in the key, with the off arm selected by an environment flag:

    /tmp/lab bridge --port 8790 --model gpt-5.6-luna --effort high &
    /tmp/cache_key_bridge_ab.sh   # off arm then on arm, prints hit% per arm

## The result

| Arm | prompt_cache_key | Rounds | Input | Cached | Hit | Passed |
|---|---|---|---|---|---|---|
| off | via TOMO_PROMPT_CACHE_KEY_OFF=1 | 70 | 4,335,966 | 3,760,384 | 86.7% | False |
| on | sent | 71 | 6,000,139 | 5,253,120 | 87.5% | False |

Both arms are the same task, `dynaconf-1225`, the same model, `gpt-5.6-luna` at effort high, on one bridge.

## The hit rate did not move on signal

The hit rate moved 0.8 points with the key on, and that is not signal.
The on arm walked a 38 percent longer token path, 6.00M versus 4.34M input, from ordinary trajectory divergence at n=1, yet its hit rate barely changed.
The structural cache-hit ratio is roughly flat across both arms regardless of the key.

The mechanism is consistent with the earlier caveat.
The bridge is one forwarding process in front of a codex backend whose automatic prompt caching already lands around 87 percent without any hint.
There is no routing scatter for the key to fix, so the key has nothing to do.
Both arms failed dynaconf, which is the expected coin-flip outcome on that task and unrelated to caching.

## What the null does and does not say

The null does not retire the change.
The hint stays byte-lossless, cannot alter a single output token, and is a strict no-op wherever the backend ignores it or already pins one machine, so shipping it costs nothing.

The null does retire the claim that this closes the 84-to-97 gap on tomo's benchmark paths.
On the codex bridge the gap is not routing scatter, since a single backend has none.
It is cache TTL and granularity, which are provider-side and which a routing key cannot touch, so the 469K excess-fresh number is real on the trace but is not recoverable by this lever on this path.

The routing benefit remains a genuine property of true multi-machine OpenAI fleets, where per-machine caching does scatter a run's rounds.
That regime is not reproducible on the single-process bridge or on the deepseek proxy, so it stays a plausible-but-unmeasured win there, not a benchmark result.
The free-tier deepseek runner in `tomo-labs/scripts/cache_key_ab.sh` stayed blocked on 429 FreeUsageLimitError and was never the load-bearing read, so the paid bridge gave the actual answer.

## Lessons

- The bridge hole is closed. `bridge_wire.go` now forwards `prompt_cache_key`, verified by `TestChatRequestForwardsPromptCacheKey`, so a bridge measurement can no longer falsely read zero because it stripped the field. That makes the live check meaningful rather than pre-broken, even though the check itself came back null.
- The lever is correct and null at once. `TestPromptCacheKey` and `TestOpenAISendsPromptCacheKey` prove the hash is stable, sensitive, empty when there is nothing to key, carries no conversation content, and reaches the wire verbatim across rounds. All of that can be true and the A/B can still show no cache-hit gain, because on this path the backend already caches keylessly.
- A paper gap is not a benchmark result. The 84-to-97 gap is real on the trace, but on the codex bridge it comes from TTL and granularity, not routing scatter, so the routing key cannot recover it here. Keep the number as a trace fact, not as a win the lever banks.
- Keep the no-regret change and label it honestly. The hint is lossless and a strict no-op where unsupported, so it ships, but it ships as a no-op on tomo's benchmark path with the multi-machine win left explicitly unmeasured.

## Reproduce

The two-arm bridge runner produced the null and is a back-to-back A/B on the paid path.

1. Build the lab against the local tomo checkout so the bridge carries the forwarding fix.
2. Start the bridge first: `/tmp/lab bridge --port 8790 --model gpt-5.6-luna --effort high &`. The bridge drives the user's own subscription and must be run one consumer at a time, since two callers on the same subscription rate-limit each other.
3. Run `/tmp/cache_key_bridge_ab.sh`, which runs the off arm with `TOMO_PROMPT_CACHE_KEY_OFF=1` and then the on arm, and prints the hit percent per arm.
4. Read the two arms as an equal hit rate, which means the backend already caches without the hint and the change is a safe no-op on this path. A higher with-key hit rate would have been a routing win, and it did not appear.
5. The free-tier deepseek runner `tomo-labs/scripts/cache_key_ab.sh` exists but stays blocked on 429 FreeUsageLimitError, so it is not the load-bearing read and the multi-machine routing regime stays unmeasured here.
