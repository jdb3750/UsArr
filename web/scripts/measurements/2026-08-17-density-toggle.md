# The section-5 density gate: what it measures, and why it stopped asserting a budget

Raw output from `web/scripts/list-bench.mjs`'s section-5 gate and from probes built on the same
machinery, captured because these figures existed only in a live run's stdout and decisions were
taken against them.

**Every figure in this file is a measurement on the machine class named in §1 and nowhere else.**
Nothing here is a figure for any other hardware, and none of it may be quoted against a budget
derived from one. Where a Pi-class number appears it is marked **INFERRED** and is never set beside
a measured number as though the two were like for like — DESIGN-DIRECTION §13.

## 1. The machine, which is part of every number here

|                 |                                                                                                               |
| --------------- | ------------------------------------------------------------------------------------------------------------- |
| CPU             | Intel Xeon @ 2.80 GHz, **x86-64**, 4 vCPU (containerised, shared host)                                        |
| Memory          | 15 GiB total                                                                                                  |
| Class           | **x86-64 desktop/server class.** A proxy for a ThinkCentre-under-Proxmox box, **not** for a Pi 5.             |
| Node            | v22.22.2                                                                                                      |
| Chromium        | 141.0.7390.37 (`chromium_headless_shell-1194`, `PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers`)                   |
| playwright-core | 1.56.1                                                                                                        |
| Viewport        | 1440×900                                                                                                      |
| Corpus          | `scripts/harness`, the rich release row — chips, a button, a checkbox, a `<select>`                           |
| Tree            | `claude/hearth-thread-d247f2-bench` at `0d70dd4`, merged with `origin/main` @ `8415911`                       |
| Code path       | `densityToggleCostShipped` in `web/scripts/list-bench.mjs` — `prefs.setDensity` → `flushSync` → forced layout |
| Date            | 2026-08-17                                                                                                    |

A shared-vCPU container is a noisy instrument, and §4 quantifies exactly how noisy. That noise is
the reason this gate no longer asserts a budget.

📌 **Every figure below names the code path it was taken on, not only the machine.** DEVELOPMENT §11
rule 5 — _"name the surface, not just the value"_ — was promoted at `8415911` from this file's own
§2 defect: two individually correct numbers, ~75.7 ms and ~18 ms, subtracted as though they were
about the same operation. A figure here without its path is not a measurement.

## 2. The gate was measuring the wrong operation — found, and fixed here

⚠️ **This is the most important thing in this file.** `toggleCost` measured a density change by
writing `data-density` onto the `<table>` with `setAttribute`. `List.svelte`'s §7.4 invalidation
hangs off an `$effect` reading `prefs.density`, so **the attribute write never fired it**: the rows
restyled and relaid out, the number looked plausible, and the forced re-measurement that §7.4 makes
mandatory was skipped entirely.

Probed at 200 rows, fresh renderer per arm, four changes per arm, `MutationObserver` on the class.
**Machine class: x86-64 container, 4 vCPU, shared host.**

| arm           | code path                                                            | mean (ms) | `.tbl--remeasure` applied |
| ------------- | -------------------------------------------------------------------- | --------- | ------------------------- |
| old gate site | `toggleCost` — `tbl.setAttribute('data-density', v)` + forced layout | **18.8**  | **never**                 |
| shipped path  | `prefs.setDensity(v)` → `flushSync` → forced layout                  | **64.9**  | yes                       |

**Roughly 3.5×, and the gate was reporting the cheaper wrong one.** This was corrected as a
**correctness fix, not an improvement**: the attribute arm was not a cheaper approximation of the
density toggle, it was a different operation wearing its name.

The fix is guarded by a **hard assertion, not a log line** — `densityToggleCostShipped` counts the
measured windows in which `.tbl--remeasure` was present and the bench fails unless it is present in
all of them. The `setAttribute` form is faster, shorter and looks equivalent, so the next person
optimising this bench will reach for it; the positive control is what makes that revert loud.

## 3. Why no budget is asserted — the numbers the decision rested on

§7.2 gained a **Controls** category at `8c62fa0`: a user-initiated change to presentation that
fetches nothing, **target < 100 ms, hard fail 400 ms**. Dividing the hard fail by ADR-0029's
pessimistic 5× Pi multiplier gives a **derived desktop floor of 80 ms** — the shape of assertion
this gate used to carry, at the old 100 ms / 20 ms values.

Against the shipped path's recorded cost at 200 rows (75.7 ms median,
`2026-08-17-density-invalidation.md` §6 at `dff20fd`) that leaves **4.3 ms of margin, 5.4%**. The
same record's five raw samples at 200 rows are **65.2, 77.1, 69.9, 75.7, 87.2** — sd **8.30 ms**,
range **22.0 ms**, and **87.2 already exceeds 80**.

Three candidate shapes were costed against those raw samples before choosing. All figures x86-64,
4 vCPU, shared host.

| candidate                       | statistic     | threshold     | margin       | margin / sd | margin / range |
| ------------------------------- | ------------- | ------------- | ------------ | ----------- | -------------- |
| absolute total at 200 rows      | 75.7 ms       | 80 ms         | 4.3 ms       | **0.52**    | **0.20**       |
| per-row, naive (`total / N`)    | 0.3785 ms/row | 0.4000 ms/row | 0.0215       | **0.52**    | **0.20**       |
| per-row, **slope**              | 0.4128 ms/row | 0.4000 ms/row | **negative** | —           | —              |
| median of k = 5, fresh renderer | 75.7 ms       | 80 ms         | 4.3 ms       | **0.92**    | —              |

- **Naive per-row is the same assertion in different units.** `N` divides out of both sides, so the
  relative margin, the sd ratio and the range ratio are all identical to the total's. It fixes
  "needs a new literal whenever the page size moves" and does nothing whatever about the noise,
  because the fixed overhead is still in the numerator.
- **The slope fails three times over.** It is the form that genuinely cancels fixed overhead —
  `(cost(N₂) − cost(N₁)) / (N₂ − N₁)` — and here the 100→200 slope is **0.4128 ms/row against a
  most-generous 0.4000 ceiling** (80/200, crediting zero fixed cost), so the margin is already
  negative before noise is considered. Its propagated sd is **0.097 ms/row, 23.5% relative** against
  the total's 11.1%, because differencing two noisy quantities adds their variances while shrinking
  the signal. And the cost curve is **convex, not linear**: bracket slopes are **0.265, 0.298 and
  0.660 ms/row** across 100→120→160→200 (implied intercept −7.5 ms), so there is no single slope to
  assert and the choice of bracket would decide the verdict.
- **A spread-stable statistic is the only one that improves with effort, and not enough.** The
  standard error of a median is 1.2533·sd/√k, so k = 5 gives 0.92 and k = 25 gives 2.07;
  **margin/se ≥ 3 needs k = 53** fresh-renderer samples at ~76 ms plus a page recycle each.

🚩 **And the deciding figure is one that no within-run sampling touches.** The same quantity,
measured on the **same tree on this same machine minutes apart**, came out at **19.8 ms** and
**14.2 ms** — **39% of between-run drift**, which is host load rather than sampling error. A
7-run series at the same site spanned 17.0–21.2 ms, 22% of its own median. **An instrument that
disagrees with itself by 39% cannot police a 5% margin, whatever statistic is computed on top of
it.**

The two measurement records already refuse to let this hardware decide a page size in their own
words — _"no single adjacent pair on it supports a page-size decision on this instrument"_, _"a
decision that needs to separate 100 from 120 needs a quieter machine, not more samples here"_. The
old gate re-ran exactly that decision on every invocation.

## 4. What is asserted instead

Two assertions, and neither is a budget.

1. **The positive control** — `.tbl--remeasure` present in all four measured windows. This is a
   correctness assertion about the measurement, not about the code under test.
2. **A regression tripwire** — the measured x86 figure against **§7.2's own 400 ms Control hard
   fail, applied directly with no Pi divisor**. That comparison is like for like: a ThinkCentre-class
   proxy measured against a budget that is a claim about the user's patience rather than about
   anyone's hardware, so §13 is satisfied because no Pi-derived number enters it. On the raw samples
   above, margin/sd is **39** and margin/range is **15**.

🔍 **Calibrated against the honest quantity, and that mattered.** A tripwire tuned while the gate
still read 18.8 ms would have sat ~5× clear of it — and the same threshold against the corrected
~65 ms cold figure is only ~1.5× clear, straight back into flapping. Correcting the measurement is
what gives the tripwire defensible headroom rather than what costs it.

⚠️ **Its limits, stated so nobody over-reads a green.** It trips on roughly an order-of-magnitude
regression, or on `LOAD_MORE_PAGE_SIZE` rising past about 700 rows. **It does not police the shipped
200-row page against a Pi-class floor**, and does not claim to; this instrument cannot.

**The `PI5_DERIVED_DESKTOP_FLOOR_MS` value (80 ms) is printed with its provenance and never
asserted against.** ADR-0029's 5× is a design floor about hardware nobody on this project runs.

## 5. The gate's own figure, and what it is not

The gate measures one page, four changes, on a fresh renderer, via
`densityToggleCostShipped` — `prefs.setDensity` → `flushSync` → forced layout, the same path the
product takes. **The first change is cold and the other three land on a renderer that has just
toggled**, so the mean is a blend. Observed at 200 rows, x86-64 4 vCPU shared host:

```
full run    38.4 ms mean of 4   (samples 41.7, 50.7, 28.8, 32.4)   -> tripwire 10.4x clear
--quick run 42.8 ms mean of 4   (samples 71.9, 28.5, 39.1, 31.8)   -> tripwire  9.3x clear
```

Two runs of the same code on the same tree and machine, minutes apart, differing by 4.4 ms on the
mean and by 43.4 ms on the widest single sample. That is §3's between-run drift showing up in the
gate's own output rather than only in the archive, and it is the clearest possible statement of why
a 4.3 ms budget margin is not assertable here.

⚠️ **A first draft of this function pre-toggled to a known density before measuring**, so that every
measured value would be a real change. That silently paid the expensive cold change outside the
measured window and produced **32.4 ms** — about half the honest figure — from four warm repeat
toggles. The sequence is now rotated against the current density instead, which guarantees four real
changes without spending the cold one first.

**This mean is fit for the order-of-magnitude tripwire it feeds and for nothing else.** The cost of
a density toggle as a user experiences it is the cold figure: **75.7 ms median at 200 rows**, five
samples, fresh renderer per sample, in `2026-08-17-density-invalidation.md` §6. Do not quote the
blended mean as that number.

## 6. Both guards were fired deliberately

A guard nobody has watched fail is indistinguishable from no guard, and this gate exists because of
exactly that failure mode. Two switches exist for firing them, and both were exercised.

**`USARR_BENCH_FORCE_ATTR_PATH=1`** measures via the old `setAttribute` site — reproducing the real
defect rather than simulating it, so the drill and the bug are the same event. The run exits 1:

```
FAIL positive control FAILED: `.tbl--remeasure` was applied in only 0 of 4 measured
     windows. The measurement is NOT going through the shipped density path, so the
     13.2 ms above is not the toggle a user performs and the tripwire below is judging
     the wrong quantity.
```

🚩 **Note what the tripwire did on that same run**: it passed, reporting _"13.2 ms … 30.3x under"_.
That is precisely the failure mode the positive control exists to catch — **a comfortable margin
around the wrong quantity**, which never goes red and so is never questioned.

**`USARR_BENCH_FORCE_TOGGLE_MS=500`** substitutes that figure for the measured one. The run exits 1:

```
ok   positive control: `.tbl--remeasure` was applied inside all 4 measured windows …
FAIL one page (LOAD_MORE_PAGE_SIZE = 200 rows): density toggle 500 ms on the shipped path,
     OVER §7.2's 400 ms Control hard fail on x86-64. This is not a thin-margin flap: the
     tripwire sits about 6x above the ~65 ms this path measures …
```

## 7. The gate reads the real page-size constant — fired, not assumed

`LOAD_MORE_PAGE_SIZE` in `src/lib/list.ts` was temporarily set to 137 and the bench re-run. It
reported the moved value rather than a hard-coded 200, confirming the gate can see its own subject.
The constant was restored to 200 in the same working tree.

## 8. What this file is not

Every number above is a measurement on the machine class in §1. §7.2's budgets and any figure
divided by ADR-0029's Pi multiplier are **design floors about hardware nobody on this project runs**,
and this file does not put the two in one column. `LOAD_MORE_PAGE_SIZE` is **not changed** by this
work, and neither is any threshold in any design document.
