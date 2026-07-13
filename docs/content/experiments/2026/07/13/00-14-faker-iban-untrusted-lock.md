---
title: "faker: solved, then locked out by a web fetch"
linkTitle: "faker IBAN lock"
description: "tomo diagnoses a Belgian IBAN bug and writes the exactly correct fix, then cannot apply it. A reference URL it fetched tripped its own prompt-injection guard, which escalated every later edit to an approval that never comes in headless mode. A close read of a run tomo had already won."
date: 2026-07-13T00:14:00+07:00
---

This is one run: tomo, on a real GitHub issue from the [swebench-live](/evals/swebench-live/) tier.
It failed, but not for lack of skill.
tomo worked out the correct fix and even printed it, then could not write a single character to the file, because a web page it fetched partway through flipped a safety switch inside tomo that it could not flip back.

## Reproducibility

Everything you need to run this exact experiment again.

| | |
|---|---|
| Run captured | 2026-07-13 00:14 (GMT+7) |
| Tool | tomo, commit `60584e7ffb4d` (pseudo-version `v0.2.5-0.20260712152729`), the commit pinned in the tomo image |
| Model | `hy3-free`, which resolves to `tencent/hy3-20260706:free` on the OpenCode Zen free tier |
| Harness | tomo-labs commit `9ff26d1`, run at `LAB_CONCURRENCY=1` |
| Task | `joke2k__faker-2142`, the faker repo at commit `6edfdbf6`, graded in a Python 3.12 venv on the host |
| Verdict | FAIL, "hidden tests not satisfied". 184,329 tokens, 24 model calls, 20.0 MB peak memory, 345 s |

```bash
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo joke2k__faker-2142 --suite swebench-live
```

## The task

The `faker` library generates fake data for tests, and one of its providers builds a Belgian bank account number, an IBAN.
The bug report is simple: the generated IBAN is not valid, because the last part of the account number carries a wrong check number.
A real IBAN ends in check digits computed from the rest of the number with a fixed formula, mod 97, and faker was just emitting random digits there instead of computing them.

The fix the maintainers merged is a few lines of pure arithmetic.
Build the account number, compute `check = the number mod 97` (or 97 when that comes out zero), and glue the check on the end.
No network needed, no ambiguity: the hidden test simply recomputes that same formula and checks the last two digits match.

## What tomo did

tomo got it completely right.
It read the provider, reproduced the bug against a known-good IBAN, and worked out the exact formula.
Its final message printed the fix in full, and it is functionally identical to the one the maintainers shipped:

```python
def bban(self) -> str:
    bank = self.numerify("###")
    account = self.numerify("#######")
    check = int(f"{bank}{account}") % 97 or 97
    return f"{bank}{account}{check:02d}"
```

Then came the last line of the run:

> Once the sandbox permits writes, applying exactly that block will confirm the fix.

tomo had the answer and was asking, in effect, for permission to type it.
That permission never came.

## Why the sandbox never permitted writes

Earlier in the run, tomo followed a link from the bug report and fetched an external page, a reference implementation of the check-digit rule:

```
[fetch] https://raw.githubusercontent.com/arthurdejong/python-stdnum/.../be/iban.py
```

tomo has a safety feature for exactly this moment.
Once a session pulls in content from outside, tomo treats the session as tainted, on the theory that outside text might be trying to hijack the agent (a prompt-injection attack).
From that point on, every action that touches the system, running a command, editing a file, writing a file, is escalated: it stops and asks a human to approve it.

That is a sensible guard when a person is watching.
This run had no person watching.
It runs headless, through `tomo -p`, and the lab answers approval prompts from a small fixed budget that most tasks set to zero because they never fetch anything.
So every escalated action read no approval and was declined:

```
reason: exec escalated: session touched untrusted content
allow? [y/N]  [edit failed] the user declined to run edit
```

Even a harmless `echo hi` was declined.
tomo tried to write the fix, was refused, tried the edit a different way, was refused, and after ten refusals gave up and described the fix in words instead.
The whole detour cost 184,329 tokens and produced no change to the file it was asked to fix.

One detail to head off confusion: if you look at the work tree afterward you will see the test file changed.
That is the grader applying its hidden test, not tomo.
Every edit tomo attempted was to the source file, and every one was declined.

## Is this a fair task?

Yes, and it stays in the suite.
The task itself is clean: a small, well-scoped bug, a fix that is pure arithmetic, a hidden test with no ambiguity.
tomo even solved it.
The failure is not about the task, it is about how tomo behaves when it is running alone.

There is a fairness point worth stating plainly, and it does not let tomo off the hook.
None of the rival tools have this taint guard, so they edit freely, and a human sitting at tomo would have tapped `y` once and let it pass.
So this run is partly a measure of how tomo copes on its own, without a human to approve things.
That is worth measuring, because running alone is exactly how tomo is used here and in any automated setting.
A coding agent that reads a reference page and then cannot edit its own workspace is genuinely stuck, and pretending otherwise would hide a real limitation.

## The lesson for tomo

Three separate things went wrong, and each is a fix tomo can make.

- **Do not fetch what you can compute.**
  The answer was derivable from the repository and simple arithmetic, and tomo was already deriving it.
  The fetch added nothing but the taint that locked the run.
  A leaner tomo reaches for the network only when the repo genuinely does not hold the answer.
- **Give the safety guard a way to work headless.**
  The guard exists to stop a hijacked session from leaking data or running something destructive.
  Writing the one file the task asked for, inside a throwaway container, is neither.
  When there is no human to approve, tomo should fall back to the policy the operator already set for local edits, and keep only the network locked so a tainted session still cannot phone home.
  Refusing everything, including the edit that finishes the job, is the wrong default.
- **Stop when every action is being refused.**
  A run where ten calls in a row are declined is going nowhere, and tomo should recognise that and stop quickly rather than burn 184,329 tokens against a wall.

None of these needs a smarter model.
tomo had the answer in hand.
It needs to not talk itself out of using it.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_MODEL=hy3-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo joke2k__faker-2142 --suite swebench-live

# read the run turn by turn, including the fetch and the declines
go run ./cmd/lab inspect tomo joke2k__faker-2142 --suite swebench-live
```

This report captures the run before the fix.
The tomo changes it calls for are the subject of the next report, which reruns the same task to confirm they turn this FAIL into a PASS.
