---
title: "faker: the fix that let tomo apply its own answer"
linkTitle: "faker --yolo fix"
description: "The follow-up to the faker lockout. tomo gains a --yolo mode that runs it fully autonomous, the same way every rival already runs. The exact task tomo had solved but could not write now passes, and passes leaner: 40 percent fewer tokens and half the model calls."
date: 2026-07-13T00:50:00+07:00
weight: 998
---

This is the second half of a story.
In the [first report](/experiments/2026/07/13-faker-iban-untrusted-lock/), tomo worked out the exactly correct fix for a Belgian IBAN bug and then could not write a single character of it, because a web page it fetched flipped a safety switch that, running headless, nothing could flip back.
This report is the fix, and the rerun that confirms it.
The same task, the same model, a newer tomo: it now passes.

## Reproducibility

Everything you need to run this exact experiment again.

| | |
|---|---|
| Run captured | 2026-07-13 00:50 (GMT+7) |
| Tool | tomo, commit `4c13896f0233` (pseudo-version `v0.2.5-0.20260712174430`), the commit now pinned in the tomo image |
| Model | `hy3-free`, which resolves to `tencent/hy3-20260706:free` on the OpenCode Zen free tier |
| Harness | tomo-labs on the `tomo-yolo-adapter` branch, run at `LAB_CONCURRENCY=1` |
| Task | `joke2k__faker-2142`, the faker repo at commit `6edfdbf6`, graded in a Python 3.12 venv on the host |
| Verdict | PASS, "fail_to_pass green, in-file pass_to_pass stable". 109,481 tokens, 11 model calls, 104.0 MB peak memory, +50 KB disk |

```bash
go run ./cmd/lab build tomo
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo joke2k__faker-2142 --suite swebench-live
```

## What changed in tomo

The first report ended with three things tomo could fix.
This report acts on the one that flips the run: tomo needed a way to run fully autonomous when no human is watching.

tomo now has a `--yolo` flag.
With it on, every action the policy gate would stop to ask about is auto-approved, so tomo acts on its own instead of waiting for a tap that never comes.
It is off by default, so tomo stays fully protected out of the box, exactly as before.
It widens ask to allow, but it never overrides an explicit deny, so a rule you set to forbid something still forbids it.
When it is on, tomo prints a one-line warning to stderr, so even a piped run leaves a record that protection was off.

Two design choices are worth stating plainly.

First, this is opt-in, not automatic.
tomo could have quietly relaxed its own guard whenever it noticed no human was around.
That would be the wrong default: it weakens the safety feature for everyone running headless, silently.
An explicit flag keeps the safe behaviour as the default and makes autonomy a choice the operator makes for a sandbox they trust.

Second, it makes the comparison fair.
tomo was the only agent in this lab still stopping to ask mid-run.
Every rival already runs unattended by default here: aider with `--yes-always`, claude-code with `--dangerously-skip-permissions`, copilot with `--allow`, gemini-cli and hermes with their own `--yolo`, kilocode and opencode with `--auto`.
tomo running `--yolo` is not a special favour, it is the equal footing the others already had.
For anyone with the muscle memory, `--dangerously-skip-permissions` works too, as a hidden alias.

The harness now runs tomo with `--yolo`, and the tomo image is pinned to the commit that adds it.

## The rerun

Same task, same model, same grader.
This time the fetch still happens and the session is still tainted, but the gate no longer stops tomo: the escalated edit is auto-approved, tomo writes the fix it had already worked out, and the hidden test goes green.

```
PASS  tomo  joke2k__faker-2142  tokens=109481 reqs=11 rss=104.0MB disk=+50KB
PASS: fail_to_pass green, in-file pass_to_pass stable
```

## Before and after

| | before | after |
|---|---|---|
| tomo commit | `60584e7ffb4d` | `4c13896f0233` |
| Verdict | FAIL | PASS |
| Tokens | 184,329 | 109,481 |
| Model calls | 24 | 11 |
| Source file written | no | yes |

The win is not only that it passed, it is that it passed leaner: 40 percent fewer tokens and under half the model calls.
That is not a coincidence.
The failing run spent most of its budget retrying edits into a wall of declines and then narrating the fix it could not apply.
Remove the wall and the run does the work once and stops.

The peak memory is higher, 104 MB against the old 20 MB, and that is the run doing its job rather than a regression.
The failing run bailed early and never built the Python probes or wrote the file; this one did both.

## What is still open

`--yolo` is the fix that flips this run, but the first report named two more, and they still stand.

- **Do not fetch what you can compute.**
  tomo reached for a reference page it did not need; the answer was pure arithmetic it was already deriving.
  A leaner tomo never fetches here, saves the tokens, and never trips the taint in the first place.
- **Stop when every action is being refused.**
  With `--yolo` on there are no refusals to stop for, but the underlying instinct, treat a wall of declines as no-progress and quit fast, is still worth having for the runs where the gate is on.

Neither is needed to make this task pass, and neither changes the result here.
They would make tomo cheaper and more robust, and they are the next things to fix.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo joke2k__faker-2142 --suite swebench-live

# read the run turn by turn, including the fetch and the write that now lands
go run ./cmd/lab inspect tomo joke2k__faker-2142 --suite swebench-live
```

This report captures the run after the fix.
It does not overwrite the [first report](/experiments/2026/07/13-faker-iban-untrusted-lock/), which stays as the dated record of the build that failed.
Read the two together and you have the whole arc: a tool that solved a task, could not apply its answer, and then was given the one thing it needed to finish the job it had already done.
