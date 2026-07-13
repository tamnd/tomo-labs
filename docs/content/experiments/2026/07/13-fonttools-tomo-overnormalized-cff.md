---
title: "fonttools: tomo writes more than the fix and fails on one normalization"
linkTitle: "fonttools tomo over-normalized"
description: "The one swebench-live failure where tomo did everything right and still lost. On a fonttools glyph-reordering bug it found the exact file, wrote a fuller fix than the maintainers, and verified its work, then failed a single hidden test because it reused a variable that forces .notdef to the front. A close read of a correctness gap that is not a discipline problem."
date: 2026-07-13T08:19:00+07:00
weight: 992
---

This is a single run: tomo, on `fonttools__fonttools-3682`, a real GitHub issue from the [swebench-live](/evals/swebench-live/) tier.
It is the failure to read after the two runaways, because it is their opposite.
The [dynaconf](/experiments/2026/07/13-dynaconf-tomo-git-archaeology-runaway/) and [python-control](/experiments/2026/07/13-python-control-tomo-scratch-file-runaway/) runs failed on discipline, burning millions of tokens with no fix landing.
This run has no discipline problem at all.
tomo went straight to the right file, wrote a fix that is fuller than the one the maintainers shipped, checked its own work, and still failed, on a single subtle choice.
That makes it the cleanest correctness gap in the sweep, and the most useful kind of failure to read.

## Reproducibility

| | |
|---|---|
| Run captured | 2026-07-13 08:19 (GMT+7) |
| Tool | tomo, `--yolo`, pinned image on the swebench-live suite |
| Model | `north-mini-code-free` on the OpenCode Zen free tier |
| Harness | tomo-labs, run at `LAB_CONCURRENCY=1`, per-attempt wall ceiling 900s |
| Task | `fonttools__fonttools-3682`, the fonttools repo at its base commit, graded in a Python 3.12 venv on the host |
| Verdict | FAIL, one hidden test red. 1,315,505 tokens, 71 model calls |

```bash
export LAB_MODEL=north-mini-code-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo fonttools__fonttools-3682 --suite swebench-live --yolo
```

## The task, in one line

`reorderGlyphs` reorders a font's glyphs across its tables, but it never touched CFF-flavoured fonts, so an OpenType/CFF font came out with its glyph tables reordered and its CFF charset and charstrings left in the old order.
One hidden test, `test_reorder_glyphs_cff`, builds a CFF font, reverses its glyph order, calls `reorderGlyphs`, and asserts the CFF charset and charstrings now match that reversed order exactly.
The fix is local: add a CFF branch to `reorderGlyphs` that reorders the charset and the charstrings dictionary.

## What tomo did, and it was almost exactly right

tomo found the file on its third move and added a CFF branch at the end of `reorderGlyphs`.
Its version is, if anything, more thorough than the maintainers':

```python
# tomo's fix
if "CFF " in font:
    cff = font["CFF "].cff
    for fontName in cff.fontNames:
        topDict = cff[fontName]
        topDict.charset = adjusted_glyph_order
        if hasattr(topDict, "CharStrings"):
            charstrings = topDict.CharStrings.charStrings
            topDict.CharStrings.charStrings = {
                k: charstrings.get(k) for k in adjusted_glyph_order
            }
elif "CFF2" in font:
    ...  # the same, for CFF2
```

It handled CFF2 as well as CFF, iterated every font in the table rather than assuming one, and guarded the CharStrings access with `hasattr`.
The maintainers' shipped fix does none of that; it takes the first top dict and reorders it directly.
By any ordinary reading tomo wrote the better patch.

## The one line that failed it

The maintainers reorder to `new_glyph_order`, the exact list passed into the function.
tomo reordered to `adjusted_glyph_order`.
Those are not the same list, and the difference is the whole failure.
`adjusted_glyph_order` is built a few lines up:

```python
adjusted_glyph_order = new_glyph_order.copy()
if ".notdef" in adjusted_glyph_order:
    adjusted_glyph_order.remove(".notdef")
adjusted_glyph_order.insert(0, ".notdef")   # force .notdef to the front
```

It is `new_glyph_order` with `.notdef` forced to index 0, which is exactly right for the TrueType glyph order that `font.setGlyphOrder` needs, and tomo reasonably reused the variable that was already sitting there.
But the hidden test reverses the glyph order, so `.notdef` lands at the end, and then asserts the CFF charset equals that reversed list verbatim:

```python
ga = list(reversed(font.getGlyphOrder()))    # .notdef is now last
reorderGlyphs(font, ga)
assert font["CFF "].cff.topDictIndex[0].charset == ga
```

tomo's fix put `.notdef` back at the front of the CFF charset, so its result was `[.notdef, ...]` where the test wanted `[..., .notdef]`.
The CFF charset is a faithful record of the requested order, `.notdef` wherever the caller put it, not a normalized one.
Reusing the normalized list broke that contract, and the assertion failed on the ordering it was written to catch.

## Why this one is worth keeping

tomo did not lack effort or discipline here.
It read the right code, understood the CFF internals well enough to write a broader fix than the reference, and verified against everything it could see.
It lost on a single reasoning step: it reused a variable whose normalization was correct for one code path and silently wrong for another.

The lever is real and it is in [tomo](https://github.com/tamnd/tomo), but it is a coding-judgement lever, not a convergence one.
When a fix reuses an existing local like `adjusted_glyph_order`, the agent should check whether that variable's transformation, here forcing `.notdef` first, is actually wanted in the new context, or whether the raw input is what the contract expects.
The tell was available: the function already distinguishes the setGlyphOrder path, which wants `.notdef` first, from the raw request, and a careful read of what a CFF charset is supposed to contain points at `new_glyph_order`.
tomo also could not run the hidden test, so its self-check never exercised the reversed-order case; a fuller fix would reason about the exact-order contract rather than lean on a normalized list that happened to be in scope.

## The lesson

This is the failure that says tomo's ceiling on this tier is coding judgement, not habits.
Strip away the two runaways and their missing convergence policy, and what is left is a run that did the process right and got the answer subtly wrong.
The fix was local, readable, and reachable, so it points at tomo, and it is the honest measure of the gap: not that tomo cannot find the bug or commit to an edit, but that it can write a confident, broader-than-needed patch that misses one semantic detail a test pins down.
That is a harder gap to close than a budget policy, and a more important one.

## Reproduce it

```bash
go run ./cmd/lab build tomo
export LAB_MODEL=north-mini-code-free LAB_CONCURRENCY=1
go run ./cmd/lab run tomo fonttools__fonttools-3682 --suite swebench-live --yolo
go run ./cmd/lab inspect tomo fonttools__fonttools-3682 --suite swebench-live
```

The task, its grader, and the base commit are committed, so a rerun on the same commit and model lands on the same verdict, free-tier rate limits on the day permitting.
