---
title: "Compacting the cx transcript is a mirage under prefix caching"
linkTitle: "cx compaction mirage under prefix cache"
description: "Tomo's cx engine rebuilds every request as the whole conversation plus the running turn, so an offline replay of six recorded runs showed a 76 to 86 percent cut in wire bytes once older tool results are stubbed. That headline did not survive the live check. Against a provider that caches the prefix, the shed bytes were already cheap cache reads, so on dynaconf-1225 the compacted arm cut total input tokens only 2.3 percent and fresh uncached input only 10.2 percent. It bought no pass, took more rounds, and hit fewer gold files. Compaction stays off by default, and the real token lever is fewer fresh tokens per round times round count, not shedding cached history."
date: 2026-07-16T02:59:00+07:00
---

This is a negative result, written down honestly because the paper number was so good.

The cx turn loop rebuilds every request as the whole conversation plus the running turn.
A file read captured twenty rounds ago goes back on the wire on every later call, so the carried mass grows without bound and the late rounds dominate the bill.
The obvious lever is to stop re-sending that old mass.
An offline replay said the win was huge.
The live A/B said it was almost nothing.

## Setup

The change is `pkg/engine/cx/compact.go`, a compaction pass for the cx loop.
`compactSend` keeps the recent window whole and, once the transcript grows past a budget, sheds the content of large older tool results and leaves a stub that names the tool and the path or command it acted on, like "read src/main.go", so the model can re-fetch precisely.
It is gated off by default: all three fields default to zero, and a zero-value engine returns the transcript untouched, so a plain build is byte-for-byte unchanged.

The operable mode, once turned on, is unconditional tail-3: stub every large older result outside the three most recent rounds, no budget gate.
The env `CompactFromEnv()` reads `TOMO_COMPACT_TAIL`, `TOMO_COMPACT_MIN_BYTES`, and `TOMO_COMPACT_BUDGET_TOKENS`, so an A/B arm selects the gate without a rebuild.

First the deterministic replay, which needs no live call.

    # replay a recorded probe trace through compactSend and sum wire bytes per round
    TOMO_TRACE_REPLAY=1 go test ./pkg/engine/cx/ -run TestTraceReplay -v

Then the live A/B on the real task, one arm with compaction off and one with tail-3 on.

    # off arm, full transcript re-sent as before
    lab probe dynaconf__dynaconf-1225 \
      --engine cx-offline --prep-env --grade --out /tmp/off

    # compacted arm, same harness, tail-3 selected through the env
    TOMO_COMPACT_TAIL=3 lab probe dynaconf__dynaconf-1225 \
      --engine cx-offline --prep-env --grade --out /tmp/tail3

## The result

The live A/B on dynaconf-1225, off arm against the tail-3 arm.

| Arm | Total input tok | Fresh uncached tok | Rounds | Gold files hit | Graded |
|---|---|---|---|---|---|
| off (full re-send) | 3,696,858 | 581,850 | 55 | 6 | 4 pass, 5 fail |
| tail-3 compaction | 3,611,286 | 522,646 | 62 | 4 | 4 pass, 5 fail |

Total input fell 2.3 percent.
Fresh uncached input fell 10.2 percent.
The pass result was identical, and the compacted arm took more rounds and hit fewer gold files.

## The paper number that misled

The replay is where the 76 to 86 percent came from.
`tracereplay_test.go` replays each recorded trace through `compactSend` and sums the wire bytes per round with and without compaction, which is the exact quadratic re-send bill the loop paid, read off the real bytes with no live call.
Over the six swebench-live traces from the 0045 sweep, unconditional tail-3 cuts the wire-byte bill hard.

| task | baseline tok | tail-3 tok | saved |
|---|---|---|---|
| conan-17123 | 453570 | 107409 | 76.3% |
| fonttools-3682 | 1461425 | 197264 | 86.5% |
| dspy-1651 | 1107383 | 169162 | 84.7% |
| instructlab-2540 | 682907 | 109042 | 84.0% |
| mesa-2394 | 335885 | 80560 | 76.0% |
| sqllineage-661 | 546795 | 122671 | 77.6% |

Every one of those numbers is real.
The mistake was reading wire bytes as the bill.
Wire bytes are what the loop puts on the wire, not what the provider charges.

## Why the cut is a mirage

Against the real provider the prefix is cached.
On the off-arm trace of dynaconf-1225, total input was 3.70M over 55 rounds and 84 percent of it was served from the provider's cache, so only 582K was fresh.
The re-sent history that compaction sheds is exactly the cached part.
Shedding a cheap cache read does not lower the bill by much, which is why the 76 to 86 percent wire cut collapsed to a 2.3 percent total cut and a 10.2 percent fresh cut once it met a caching provider.

The budget modes tell the same story from the other side.
The 64k and 32k budget gates barely fire, because the token mass is spread across many mid-size rounds, each individually under the per-round ceiling, even while the summed bill is enormous.
So the only mode that does anything is the unconditional one, and the only thing it sheds is cached history.

## Where the real fresh tokens go

Tracing the off-arm run, the fresh mass is not the re-sent history at all.
New content added per round is only about 2K tokens, roughly 1.8K of new tool results and 0.3K of new assistant tool_use.
No reasoning is carried: the assistant messages are 54 tool_use blocks, 17.6KB total, and nothing else.
The remaining fresh tokens per round are the provider's cache granularity, not a tomo defect, because the request is already cache-optimal: the system prompt is identical across rounds, the tool schema is byte-identical and same-order, and the message history is strictly append-only.

So the whole lossless-lever sweep came back null on this trace.
Re-send reduction is neutralized by caching.
There is no cache-defeating prefix volatility.
No carried reasoning is re-fed as input.
There are no redundant identical re-reads, each file is read once.
The cheap deterministic wins are spent.
What is left is fewer fresh tokens per round times fewer rounds, and both of those trade against quality and need a live sweep, not a paper estimate.

## Lessons

- Wire bytes are not the bill. The replay measured a 76 to 86 percent cut in bytes on the wire, but under prefix caching the shed bytes were 84 percent cached, so the live total cut was 2.3 percent and the fresh cut 10.2 percent. Always validate a token lever on live fresh-plus-cached tokens, never on wire bytes.
- The cut bought no pass and cost rounds. Both arms graded 4 pass and 5 fail, and the compacted arm took 62 rounds against 55 and hit 4 gold files against 6. A lossless-looking change that shifts nothing on pass and drifts the trajectory is not free.
- Compaction stays off by default. All three fields default to zero and a plain build is byte-for-byte unchanged. It is opt-in through the env, and the record here is the reason to leave it that way.
- The cx request is already cache-optimal. Identical system prompt, byte-identical tool schema, append-only history. There is no free efficiency win left in request construction, so the next real gain is correctness work that needs a multi-run live A/B, not a single safe diff.

## Reproduce

1. Build the lab against the local tomo checkout: `go build -o /tmp/lab ./cmd/lab`.
2. Run the offline replay first: `TOMO_TRACE_REPLAY=1 go test ./pkg/engine/cx/ -run TestTraceReplay -v` prints the per-round wire-byte bill with and without tail-3, and reproduces the 76 to 86 percent table with no live call.
3. Run the off arm: `lab probe dynaconf__dynaconf-1225 --engine cx-offline --prep-env --grade --out /tmp/off`.
4. Run the compacted arm with the same flags plus `TOMO_COMPACT_TAIL=3` in the environment, writing to a separate `--out`.
5. Read both back and compare total input, fresh uncached input, rounds, and gold files. The wire-byte win from step 2 will not show up here, because the provider already served the re-sent history from its prefix cache.
