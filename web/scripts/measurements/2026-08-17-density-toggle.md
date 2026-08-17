# Density-toggle cost at one "Load more" page — the run output behind the gate

Raw output from `web/scripts/list-bench.mjs`'s section-5 page-size gate and from a one-off probe
built on the same machinery, captured because these figures existed only in a live run's stdout and
a decision was being taken against them.

**Everything below is a measurement on the machine named in §1 and nowhere else.** Nothing here is a
figure for any other hardware class, and none of it should be quoted against a budget derived from
one.

## 1. The machine, which is part of every number here

|                 |                                                                                                                  |
| --------------- | ---------------------------------------------------------------------------------------------------------------- |
| CPU             | Intel Xeon @ 2.80GHz, **x86-64**, 4 vCPU (containerised, shared host)                                            |
| Memory          | 15 GiB total                                                                                                     |
| Class           | **x86-64 desktop/server class.** A proxy for a ThinkCentre-under-Proxmox box, **not** for a Pi 5 or any ARM SBC. |
| Node            | v22.22.2                                                                                                         |
| Chromium        | 141.0.7390.37 (`chromium_headless_shell-1194`, `PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers`)                      |
| playwright-core | 1.56.1                                                                                                           |
| Tree            | `claude/hearth-thread-d247f2-bench` merged with `origin/main` @ `4fb96e0`; probe run at `bc695a7`                |
| Date            | 2026-08-17                                                                                                       |

A shared-vCPU container is a noisy instrument. The spread in §2 is that noise, not a moving
primitive: the corpus is deterministic and the DOM is byte-identical between samples.

## 2. The gate figure — `atSize(LOAD_MORE_PAGE_SIZE)`, mean of four toggles, x86-64

The gate measures on a **fresh renderer** (`atSize` → `recyclePage`), which is the 200-row page a
user actually gets, cold. It previously measured `setRows(200)` on the page the 25,000-row sweep
left behind — a warmed renderer — which is why the figure moved without the primitive changing.

| Run | Tree                                | mean (ms) | Provenance                       |
| --- | ----------------------------------- | --------- | -------------------------------- |
| 1   | `origin/main` (warm `setRows` site) | 12.0      | prior full sweep, same container |
| 2   | `origin/main` (warm `setRows` site) | 12.0      | prior full sweep, same container |
| 3   | branch (fresh `atSize` site)        | 17.0      | prior full sweep, same container |
| 4   | branch                              | 18.0      | prior full sweep, same container |
| 5   | branch                              | 18.9      | prior full sweep, same container |
| 6   | branch                              | 19.5      | prior full sweep, same container |
| 7   | branch                              | 20.3      | prior full sweep, same container |
| 8   | branch                              | 21.2      | prior full sweep, same container |
| 9   | branch + this commit's gate fixes   | 17.5      | `--quick`, this session          |

Branch median ≈ 19.2 ms; min 17.0, max 21.2. **The two `main` figures are not comparable with the
six branch ones** — they were taken at a different measurement site, not on a different primitive.

## 3. What a density toggle actually does — mechanism, measured

`prefs.setDensity` writes `data-density` on `<html>`; `List.svelte` re-stamps its own
`data-density` on the `<table>` and writes `--row-ci` through the CSSOM. The `{#each}` is keyed on
row identity, which a density change does not touch.

Checked at 200 rows on a fresh renderer, one toggle compact → relaxed:

```
rowsBefore 200   rowsAfter 200
sameObjects true            (every <tr> is the same object as before the toggle)
stampsAlive 200/200         (JS expandos on all 200 rows survived; a rebuilt node cannot carry one)
firstStillAttached true     lastStillAttached true
keys unchanged              (g0-rel-0 … g0-rel-4 before and after)
--row-h  28px → 36px        --row-ci 27px → 35px        row height 45px → 53px
```

CDP `Performance.getMetrics` delta across the evaluate that contained that toggle (the toggle itself
cost 22.4 ms; the window is wider than the toggle, so read the **proportions**, not the totals — the
recalc/layout split and the zero script time are the finding, not the 30.4 ms):

```
RecalcStyleCount     2
RecalcStyleDuration  30.4 ms
LayoutCount          2
LayoutDuration        9.9 ms
ScriptDuration        0    ms
TaskDuration         70.1 ms
```

**No rows are rebuilt. The cost is style recalculation across the subtree, with layout second and
script at zero.** The isolation run below confirms it from the other side: pin the three geometry
custom properties so the attribute change still matches every density rule but can no longer change
a box, and the same toggle costs **0.0–0.1 ms**. So essentially the whole figure is the recalc and
relayout that follow the row-height change, and there is no re-render to remove.

## 4. Variants at 200 rows, x86-64, three samples each, fresh renderer per sample

| Variant                                                  | means (ms)         | min / median / max |
| -------------------------------------------------------- | ------------------ | ------------------ |
| gate: `setAttribute`, container-scoped, containment live | 19.8 / 19.8 / 20.8 | 19.8 / 19.8 / 20.8 |
| the app's own path: `prefs.setDensity` + `flushSync`     | 23.7 / 15.5 / 13.3 | 13.3 / 15.5 / 23.7 |
| geometry pinned (recalc only, no box change)             | 0.1 / 0.0 / 0.1    | 0.0 / 0.1 / 0.1    |
| containment forced off                                   | 57.3 / 45.3 / 43.9 | 43.9 / 45.3 / 57.3 |
| warm (same page, no recycle) — passes 2 and 3            | 11.8 / 12.5        | —                  |

Two things worth keeping:

- **`content-visibility` is paying for itself here**: with containment forced off the same toggle is
  43.9–57.3 ms against 19.8–20.8 ms, i.e. roughly 2.5×.
- **Cold vs warm is about 1.5×** on this machine: 19.8–20.8 fresh against 11.8–12.5 on a page that
  has already toggled once. That gap, not a regression, is the whole of the difference between the
  `main` figures and the branch figures in §2.

## 5. Candidate page sizes — MEASURED, BUT NOT A BASIS FOR A DECISION

Gate-identical measurement, fresh renderer per sample, three samples each, x86-64.

| rows | means (ms)         | min / median / max |
| ---- | ------------------ | ------------------ |
| 200  | 15.6 / 14.2 / 12.5 | 12.5 / 14.2 / 15.6 |
| 160  | 12.9 / 12.8 / 14.1 | 12.8 / 12.9 / 14.1 |
| 120  | 10.5 / 11.6 / 10.7 | 10.5 / 10.7 / 11.6 |
| 100  | 13.4 / 9.8 / 15.7  | 9.8 / 13.4 / 15.7  |

⚠️ **This table is recorded, not actionable, and it is the run-to-run spread that says so**: 100
rows produced a higher median than 120, and 200 here is lower than 200 in §4 taken minutes earlier
on the same tree. On a shared-vCPU container the size signal over this range is inside the noise.

⚠️ **And the curve is about to move.** DESIGN-DIRECTION §7.4 requires — "required rather than
advisory" — that a density change either rebuild the affected row nodes or force re-measurement of
every mounted row before the next paint. Neither is implemented: the `{#each}` keys on row identity
alone and `web/src/` contains no `{#key` at all, so §3's "no rows are rebuilt" is a measurement of a
configuration the spec forbids shipping. Complying will make the toggle **more** expensive by an
amount nobody has measured. Any page size chosen against this table is chosen against a curve that
has not been drawn yet.

## 6. The gate reads the real constant — fired, not assumed

`LOAD_MORE_PAGE_SIZE` in `src/lib/list.ts` was temporarily set to 137 and the bench re-run. It
reported:

```
ok one page (LOAD_MORE_PAGE_SIZE = 137 rows): density toggle 16.5 ms on this desktop, inside the
   20 ms budget (100 ms Tier-0 hard fail ÷ ADR-0029's 5x Pi 5 multiplier) and 6x under the hard
   fail itself
```

Before this commit the gate carried its own `const PAGE_SIZE = 200` and would have kept measuring
200 rows while its failure message named the constant that had moved. The constant was restored to
200 in the same working tree; only the gate's two fixes are committed.
