---
title: "The cleanest local pass: a 4-bit MoE fixes fonttools-3682 in five rounds"
linkTitle: "fonttools-3682, local MoE"
description: "Running tomo against a small quantized model on a single desktop GPU, fonttools-3682 was the fastest and leanest pass of the local run: five rounds, 18.1k tokens, 127 wall seconds. The model went grep, read, edit, done, with no wrong turns and no re-reads. The reason is the task itself. The issue report already named the file, the symbols, and even handed over a working patch, so a 4-bit Qwen3-30B-A3B on an RTX 4090 only had to locate the function and transcribe the fix. This writes up the turn-by-turn path, the one-block diff, and why a small, well-localized fix is cheap for a local MoE while a diffuse one is not."
date: 2026-07-19T22:39:00+07:00
---

Reproducibility header: tool=tomo, model=qwen3-30b-a3b (Qwen3-30B-A3B GGUF Q4_K_M via Ollama on an RTX 4090 behind the llmgw gateway), suite=swebench-live, task=fonttools__fonttools-3682.
Reproduce command:

    LABS_DIR=~/github/tamnd/tomo-labs TOKEN=<gateway token> MODELS=qwen3-30b-a3b TASKS=fonttools__fonttools-3682 scripts/solve-swebench.sh

This was the fastest and leanest pass in the local run.
Five rounds, 18.1k total tokens, 127 wall seconds, and a hidden FAIL_TO_PASS test that went green.
Worth writing down not because the model is clever, but because the task made it easy, and it is useful to know exactly what "easy" looks like for a small quantized model on one desktop GPU.

## Setup

The model is Qwen3-30B-A3B, a mixture-of-experts model with about 3B active parameters per token, quantized to GGUF Q4_K_M and served by Ollama on a single RTX 4090 with 24 GB of VRAM.
It sits behind the llmgw gateway, so tomo talks to it over the normal OpenAI chat-completions path.
tomo drives its default agent loop: grep to find code, read a narrow range, edit an exact snippet, and stop when the work is done.
The checkout is fonttools at the bug commit with future history stripped, and the task's own check.sh grades the change against a hidden test suite.

## What the task asked

The bug is in `reorderGlyphs`, the helper that rearranges a font's glyphs into a new order.
When you reorder the glyphs, every table that refers to glyphs by index has to be rewritten to match.
The function did this for the usual OpenType tables, but it never touched the `CFF ` table.
So after a reorder the CFF `charset` and the `CharStrings` array still pointed at the old order, and the saved font was invalid.

The issue report is unusually generous.
It names the exact attributes, `CFF.cff.topDictIndex[0].charset` and `CFF.cff.topDictIndex[0].CharStrings.charStrings`, and it hands over a working patch:

    if "CFF " in font:
        cff_table = font["CFF "]
        charstrings = cff_table.cff.topDictIndex[0].CharStrings.charStrings
        cff_table.cff.topDictIndex[0].charset = new_glyph_order
        cff_table.cff.topDictIndex[0].CharStrings.charStrings = {
            k: charstrings.get(k) for k in new_glyph_order
        }

It even adds the `.notdef` rule: whatever the requested order, `.notdef` should be moved to the front.
So the model was not asked to design a fix.
It was asked to find where the fix goes and drop it in without breaking anything.

## What tomo did

The path was clean.
Five requests, and the first one does not count against the model.

The gateway's first hit was a bare `GET /` health probe that returned "404 page not found", which is the harness warming up the connection, not a model turn.
After that there are four model calls and three tool calls, and every one lands.

Round one, the model reads the issue and goes straight for the definition:

    grep {"pattern":"def reorderGlyphs"}

The reasoning trace shows it talking itself out of guessing.
It first supposes the code might be in `ttLib/tables/CFF.py` or `ttLib/macros.py`, then stops and says to grep instead of assuming.
The grep returns two hits and the useful one is `Lib/fontTools/ttLib/reorderGlyphs.py:246`.

Round two, it reads a twenty-line window at exactly that spot:

    read {"limit":20,"path":"Lib/fontTools/ttLib/reorderGlyphs.py","offset":245}

That window is enough to see the whole function head: the length check, the same-set check, and the `font.ensureDecompiled()` call with its comment about tables that use glyph indexes.
The model now has an anchor line to insert after.

Round three, it makes one edit, replacing the single line `    font.ensureDecompiled()` with that same line plus the CFF block:

    # Handle CFF table
    if "CFF " in font:
        cff_table = font["CFF "]
        top_dict = cff_table.cff.topDictIndex[0]
        if ".notdef" in new_glyph_order:
            new_glyph_order.remove(".notdef")
            new_glyph_order.insert(0, ".notdef")
        top_dict.charset = new_glyph_order
        charstrings = top_dict.CharStrings.charStrings
        top_dict.CharStrings.charStrings = {k: charstrings.get(k) for k in new_glyph_order}

The tool reports `replaced 1 occurrence(s)`.

Round four, the model writes one sentence and stops:

    The change has been applied to reorder the CFF table charset and CharStrings correctly, moving ".notdef" to the front if present.

No re-read, no second edit, no thrash.

One honest note about the trace: most of the model's tokens are in the `reasoning` field, not `content`.
The round-three reasoning is long and circular, re-deriving the same insert three or four times and second-guessing how the edit tool matches strings.
That is the model being a small quantized model, not the loop being inefficient.
The loop itself made exactly the three tool calls it needed.

## The fix

The final diff is one hunk in one source file:

    --- a/Lib/fontTools/ttLib/reorderGlyphs.py
    +++ b/Lib/fontTools/ttLib/reorderGlyphs.py
    @@ -262,6 +262,17 @@ def reorderGlyphs(font: ttLib.TTFont, new_glyph_order: List[str]):
         font.ensureDecompiled()
    +
    +    # Handle CFF table
    +    if "CFF " in font:
    +        cff_table = font["CFF "]
    +        top_dict = cff_table.cff.topDictIndex[0]
    +        if ".notdef" in new_glyph_order:
    +            new_glyph_order.remove(".notdef")
    +            new_glyph_order.insert(0, ".notdef")
    +        top_dict.charset = new_glyph_order
    +        charstrings = top_dict.CharStrings.charStrings
    +        top_dict.CharStrings.charStrings = {k: charstrings.get(k) for k in new_glyph_order}

It is placed right after `font.ensureDecompiled()`, which is correct.
The decompile guarantees the CFF table is loaded before its `charset` and `CharStrings` get rebuilt.
The block sets `charset` to the new order and rebuilds `charStrings` as a fresh dict keyed in the new order, so both structures now line up with the reordered glyphs.

## Why it passed

The hidden test the grader applies reverses a CFF font's glyph order and asserts both structures follow:

    def test_reorder_glyphs_cff():
        font = TTFont(str(DATA_DIR / "TestVGID-Regular.otf"))
        ga = list(reversed(font.getGlyphOrder()))
        reorderGlyphs(font, ga)
        assert list(font["CFF "].cff.topDictIndex[0].CharStrings.charStrings.keys()) == ga
        assert font["CFF "].cff.topDictIndex[0].charset == ga

Before the change, `reorderGlyphs` left `charset` and `charStrings` in their original order, so both asserts failed.
After the change, `charset` is the new order and `charStrings` is rebuilt in that order, so `.keys()` matches `ga` and `charset == ga`.
The check reports `PASS: fail_to_pass green, in-file pass_to_pass stable`, so nothing else in the file regressed.

There is one subtlety in the `.notdef` branch worth spelling out, because it looks like it should break the test but does not.
The test font's original order starts with `.notdef`, so the reversed `ga` ends with `.notdef`.
The model's block moves `.notdef` back to the front, which would seem to make `charset` disagree with the `ga` the test compares against.
It does not, because `ga` is the same list object the test passes in as `new_glyph_order`, and the block mutates that list in place with `remove` then `insert`.
So the move rewrites `ga` too, and `charset`, which is assigned that same object, stays equal to it.
The model copied the report's `.notdef` rule verbatim and did not reason about the aliasing, but the in-place mutation kept the test's own reference aligned, so both asserts hold.

## The lesson

This task is cheap for a local 4-bit MoE for one reason: the fix is small and its location was handed over.
The report named the file's key symbols, so grep found the function on the first pattern.
The fix is one contiguous block with a clear anchor line, so a single exact-string edit placed it.
And the patch was written out in the issue, so the model did not need to invent logic, only transcribe it into the right spot.
A small quantized model is good enough for find-the-spot-and-transcribe.
It reasons in circles, but the loop only spends a tool call when the reasoning resolves, so the circling costs completion tokens, not rounds.

The slower local passes are the opposite shape.
When the fix is diffuse, spread across two files or gated on behavior the report does not spell out, the model has to hold more of the codebase in its head and decide what the fix is, and that is where a 3B-active MoE starts guessing, re-reading, and burning rounds.
The takeaway for the local tier is to expect the win on well-localized bugs and to read the issue report first: a task that already tells you the file and the patch is a transcription job, and transcription is exactly what a cheap local model can do on one GPU.

Metrics: passed, 1 attempt, 5 rounds (4 model calls, 3 tool calls: grep, read, edit), 18055 total tokens (10261 prompt, 7794 completion), 127 wall seconds, 20.4 MB peak RSS, graded PASS on the hidden FAIL_TO_PASS with in-file PASS_TO_PASS stable.
