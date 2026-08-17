# Density-change invalidation: mechanism, choice of fix, and what complying costs

DESIGN-DIRECTION §7.4 requires that a density change invalidate every mounted row's remembered
intrinsic size, by rebuilding the row nodes or by forcing re-measurement, and requires that any
measurement of the density toggle report **both** the cost and the scrollbar error it leaves.
This is that measurement, and the record of which option shipped.

## The instrument, and the machine

**Machine class: x86-64 container, 4 vCPU (Intel Xeon @ 2.80 GHz), 15 GB RAM, shared host.**
This is a reasonable proxy for a ThinkCentre under Proxmox. **It is not a proxy for a Pi 5**, and
no figure below has been scaled to one.

|                           |                                                                                     |
| ------------------------- | ----------------------------------------------------------------------------------- |
| Tree measured             | `3ff8151` plus the change this record accompanies                                   |
| Browser                   | Chromium 141.0.7390.37, headless, via `playwright-core` 1.56.1                      |
| Node                      | v22.22.2                                                                            |
| Viewport                  | 1440×900                                                                            |
| Corpus                    | `scripts/harness`, the rich release row — chips, a button, a checkbox, a `<select>` |
| Rich-row mean content box | compact 45 px · standard 49 px · relaxed 53 px                                      |

Measured on the harness **after** `3ff8151` fixed its `minmax(max-content, auto)` actions track. An
earlier pass of this same work ran on the pre-fix harness; every figure here was re-taken.

The two arms are the same build. The "fix off" arm neutralises the shipped rule in CSS
(`.tbl--remeasure tbody tr { content-visibility: auto !important }`) rather than reverting the
code, so both arms run the same component down the same path and the delta is exactly the forced
re-measurement.

## 1. The recorded failure reproduces exactly

§7.4's pair, re-taken under its own conditions — 5,000 rich rows, `rowIntrinsic` set to the
measured mean per density, scrolled through once at compact and then switched to relaxed:

| arm                           | scrollHeight at load → after a full scroll | drift      | §7.4 records |
| ----------------------------- | ------------------------------------------ | ---------- | ------------ |
| reused nodes, no invalidation | 232,254 px → 271,870 px                    | **14.57%** | 14.57%       |
| rows rebuilt                  | 270,120 px → 271,870 px                    | **0.64%**  | 0.65%        |
| **reused nodes, this fix**    | **271,870 px → 271,870 px**                | **0.00%**  | —            |

Both recorded numbers reproduce. The instrument is therefore trusted for the rest of this file.

## 2. The mechanism, per row rather than per document

A document-level drift figure cannot separate "the estimate was wrong" from "the memory is
stale". This one does: 5,000 rows, only the **first 25%** scrolled, so some rows have rendered and
the rest never have, then compact → relaxed.

|                                          | row #600 (rendered, then off screen) | row #3000 (never rendered) | scrollHeight |
| ---------------------------------------- | ------------------------------------ | -------------------------- | ------------ |
| before, compact, `auto 45px`             | 45 px                                | 46 px                      | —            |
| **after, relaxed, `auto 53px`, fix off** | **45 px**                            | 54 px                      | 260,803 px   |
| after, relaxed, `auto 53px`, fix on      | 53 px                                | 57 px                      | 271,870 px   |
| ground truth at relaxed                  | 53 px                                | 57 px                      | 271,870 px   |

**Confirmed, and this is the mechanism rather than an observation with a plausible story
attached.** The declared length tracks the density — the never-rendered row moves 46 → 54 px as
`--row-ci` goes 45 → 53 px. The _remembered_ size does not: the row that had been rendered once
stays at its compact 45 px while the truth is 53 px. So after a density change the placeholder
heights of already-seen off-screen rows are one density stale, and the scrollbar is built from
them. Fix on, every row matches ground truth.

## 3. Four of the six candidate mechanisms do not work

5,000 rich rows, compact → relaxed, same machine class. "Blocking" is main-thread work in the
toggle, including any restore step.

| mechanism                                  | blocking (median of 3) | scrollbar error | verdict                      |
| ------------------------------------------ | ---------------------- | --------------- | ---------------------------- |
| nothing                                    | 208.7 ms               | 14.57%          | the defect                   |
| drop the `auto` keyword for a frame        | 332.9 ms               | 14.57%          | **no better than nothing**   |
| `content-visibility: hidden` for a frame † | 87.0 ms                | 14.09%          | **no better than nothing**   |
| `display: none` on the tbody for a frame   | 572.3 ms               | 14.59%          | **no better than nothing**   |
| detach the tbody for a frame †             | 71.5 ms                | 0.61%           | works; rejected, see below   |
| rebuild the rows                           | 3,913.1 ms             | 0.64%           | works (§7.4's first option)  |
| **force re-measurement**                   | 7,847.1 ms             | **0.00%**       | works (§7.4's second option) |

† measured at 1,000 rows, not 5,000; the column is not comparable across that line.

**The remembered size is not discarded by hiding the row, by taking its box away, or by removing
the keyword that reads it. Only laying the row out again replaces it.** Two of those three
non-working candidates were this author's own hypotheses, offered as cheap wins; they are recorded
because the reasoning that produced them is exactly the reasoning a future reader will repeat.

**Detaching the tbody works and is cheapest, and is still rejected**, on grounds that are not
cost: removing the rows from the document collapses the document height for a frame, so the
scroll offset is clamped and the user's position in a long list is destroyed, and focus on a
detached row is lost. §7.4 permits rebuild or re-measurement; this is neither.

## 4. Why forced re-measurement rather than rebuild

At 5,000 rows rebuild is the cheaper of the two. **At the sizes that actually ship it is not**, and
it is worse on the number §7.4 says decides it. Five samples, fresh renderer per sample, medians:

| rows | rebuild — blocking | rebuild — error | re-measure — blocking | re-measure — error |
| ---- | ------------------ | --------------- | --------------------- | ------------------ |
| 100  | 47.1 ms            | 0.11%           | **39.4 ms**           | **0.00%**          |
| 120  | 58.9 ms            | 0.27%           | **45.2 ms**           | **0.00%**          |
| 160  | 70.3 ms            | 0.43%           | **58.4 ms**           | **0.00%**          |
| 200  | 83.2 ms            | 0.60%           | **79.9 ms**           | **0.00%**          |

§7.4 makes rebuild the baseline that forced re-measurement must **match on scrollbar error**, not
merely beat on cost. It does not match it; it beats it, at every size, and it is also cheaper at
every size measured here. Three further properties decided the tie in advance of the numbers, and
none of them is a measurement:

- rebuild destroys every row node, so keyboard focus on the roved row is lost on what §7.4 calls
  "a first-class setting on every screen" — the same invariant ADR-0029 protects for "Load more";
- rebuild discards in-cell control state and text selection;
- re-measurement changes nothing about the steady state — one class, for two frames.

**Rejected, with reasons:** keying the each-block on `id + density` (rebuild, above);
`{#key density}` around the list body (rebuild, plus it rebuilds the tbody itself); dropping
`auto`, `content-visibility: hidden`, `display: none`, tbody detach (all above).

## 5. Two frames, not one — a fix that measured as no fix

Recorded because it is invisible in review and cost a full re-measurement pass to find.

A `requestAnimationFrame` callback scheduled inside an event handler runs in **that same frame's**
rendering steps, **before** style and layout. A single-rAF removal can therefore take the class off
before the frame that was supposed to record the new sizes ever renders. Measured on the shipped
primitive, 5,000 rich rows, compact → relaxed, single-rAF removal:

- synchronously after the forced layout, `document.scrollHeight` = **271,870 px** — correct;
- three frames later, **232,198 px** — a **14.59%** error, which is the 14.57% of doing nothing.

The forced synchronous layout gives correct **geometry** immediately. It does not reliably cause
the browser to **record** that geometry as the row's last remembered size; only a completed
rendering opportunity does. The shipped code therefore holds the class for two frames. The second
frame is nearly free — the rows laid out in the first and the layout is cached.

**This is also why the first run of this measurement disagreed with itself**: sections measuring
per-row heights showed a working fix while the document-level drift section showed none. The
disagreement was the bug, not noise, and it was resolved by instrumenting rather than by
re-running.

## 6. What complying costs — the curve

**One density change through the real product path** (`prefs.setDensity` → Svelte flush → forced
style and layout), fresh renderer per sample, five samples per cell, medians reported.
**Machine class: x86-64 container, 4 vCPU, shared host.**

| rows | toggle, no invalidation | scrollbar error | toggle, with invalidation | scrollbar error |
| ---- | ----------------------- | --------------- | ------------------------- | --------------- |
| 100  | 15.2 ms                 | 7.53%           | **32.1 ms**               | **0.00%**       |
| 120  | 16.9 ms                 | 8.70%           | **37.4 ms**               | **0.00%**       |
| 160  | 18.7 ms                 | 10.18%          | **49.3 ms**               | **0.00%**       |
| 200  | 19.4 ms                 | 11.07%          | **75.7 ms**               | **0.00%**       |

Per-sample values, with invalidation, in run order:

```
100 rows   42.6, 30.2, 32.2, 32.1, 31.6
120 rows   37.4, 37.9, 36.3, 36.1, 38.3
160 rows   47.7, 54.7, 50.6, 49.3, 47.6
200 rows   65.2, 77.1, 69.9, 75.7, 87.2
```

**Is the signal resolvable on this instrument?** Partly, and the parts differ — this runner is
noisy, and a previous page-size sweep on it was refused because 100 rows medianed _above_ 120.

- **The trend is resolvable.** The curve is monotone across all four sizes, and 100 → 200 is
  32.1 → 75.7 ms, a factor of 2.4 against per-sample spreads of 2–22 ms. That is a real slope.
- **Adjacent points are not all separable.** 100 and 120 medians differ by 5.3 ms while the
  100-row samples span 30.2–42.6 ms; those two sizes overlap and should not be read apart.
  160 → 200 (49.3 → 75.7 ms) is outside the spread and can be.

So: **the cost of complying rises with page size, roughly doubling between 100 and 200 rows, and
the shape is a curve rather than a step — but no single adjacent pair on it supports a page-size
decision on this instrument.** A decision that needs to separate 100 from 120 needs a quieter
machine, not more samples here.

**The scrollbar error, by contrast, is not a curve at all.** It is 0.00% at every size with
invalidation, and it _rises_ with page size without it — 7.53% at 100 rows to 11.07% at 200, all
of it above the 2% budget §7.4 holds this to. The defect is not a large-list-only problem: it is
already 3.8× over budget at the smallest page size measured.

## 7. What this is not

Every number above is a **measurement on the machine class named at the top**. §7.2's 100 ms
Tier-0 hard fail and the 20 ms figure derived from it via ADR-0029's pessimistic Pi-5 multiplier
are **design floors about hardware nobody on this project runs**. §13 forbids quoting a
Pi-derived figure as a measured one, and this file does not compare the two: they are different
kinds of object, and putting them in one column would imply an equivalence that does not exist.

`LOAD_MORE_PAGE_SIZE` is **not changed** by this work, and neither is any threshold. The curve is
produced here; the decision belongs to the owner and to design.
