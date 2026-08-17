/**
 * A figure and its unit are two slots, not one string — DESIGN-DIRECTION §9.1.
 *
 * The caller emits `value` and `unit` into separate elements so the unit can be
 * given a fixed-width box: right-aligning `4.8 GiB` over `820 MiB` as one string
 * aligns the `B`, which puts the digits — the one thing being compared down the
 * column — at two different x-positions, and `tabular-nums` cannot help because
 * the misalignment is caused by the word, not by the glyph widths.
 */
export type Measure = { value: string; unit: string };

const SIZE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];

/**
 * Binary units, because that is what every indexer and every *Arr reports.
 *
 * Returns `null` rather than an empty pair for an absent or nonsensical byte
 * count, so a caller cannot render an empty unit box where the design calls for
 * §9.1's `—`. A pair with `value: ''` would still reserve 3ch of nothing.
 */
export function sizeParts(bytes: number | undefined): Measure | null {
	if (bytes === undefined || !Number.isFinite(bytes) || bytes < 0) return null;
	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < SIZE_UNITS.length - 1) {
		value /= 1024;
		unit += 1;
	}
	const digits = unit === 0 || value >= 100 ? 0 : 1;
	return { value: value.toFixed(digits), unit: SIZE_UNITS[unit] };
}

/**
 * The one-string form, kept because it is what a `title`, an `aria-label` or a
 * plain-text export wants — nothing there has two slots to emit into. Rendering
 * a size column through this instead of through `sizeParts` is the thing §9.1
 * measured and rejected. Re-measured here on this app's own binary units rather
 * than quoted: over `68.4 GiB` / `820 MiB` / `4 B`, the figures' right edges
 * spread 14.00 px through this function and 0.00 px through `sizeParts` into a
 * reserved unit box.
 */
export function formatSize(bytes: number | undefined): string {
	const parts = sizeParts(bytes);
	return parts ? `${parts.value} ${parts.unit}` : '';
}

/**
 * Release age. The server sends `age_days` as a float (internal/httpapi/search.go),
 * so this is the one place that turns it into something a human reads.
 *
 * ⚠️ NO `ageParts`, and the omission is the decision rather than an unfinished
 * job. It rests on §9.1's ruling and on nothing this comment measures. §9.1
 * applies the split-slot treatment COLUMN BY COLUMN, and in the PRODUCT tree it
 * has reached the size columns only: `app.css` carries exactly one `.unit`
 * modifier, `.unit--size`, and no `.unit--age`. §9.1's `Age` result was measured
 * on the MOCKUP tree, whose Age track is 80 px; the product's `Age` in the
 * release tables is a declared 68 px track (`COLUMNS` in
 * `routes/requests/+page.svelte`). Until §9.1 rules the box onto the product's
 * column, Age keeps the one-string treatment, and widening a declared track is
 * its own decision on the densest tables in the product. So there is
 * deliberately no pair-returning function here to reach for. Publishing one
 * would make the exclusion a matter of discipline; not publishing one makes it
 * structural.
 *
 * ⚠️ IT USED TO REST ON ARITHMETIC AS WELL, AND THE ARITHMETIC WAS FALSE. The
 * clause read "24 px of padding leaves 44 px, and the widest unit alone is 43 px,
 * so the figure has nothing left and the cell wraps", costed at +18 px per cell
 * over 224 cells. The 68 px track and its 24 px of padding are right; 43 px is
 * not a unit this function emits. It is the width of the string `months` in the
 * MOCKUPS' Age-cell type stack, and `formatAge` returns only ` h` and ` d`.
 * Re-measured in the PRODUCT's own Age cell with IBM Plex served — HARNESS
 * numbers, `web/scripts/harness` re-seeded with this table's `COLUMNS`, because
 * no rig renders `/requests` — `h` is 7 px and `d` is 8 px, while `months`
 * reproduces at 43 px, which is what identifies the mockup string as the source.
 * A 44 px content box is not short of room for an 8 px unit, and the +18 px cost
 * went with the number it was derived from. Both are deleted rather than
 * corrected in place: a demonstration whose measurement has been falsified is
 * not repaired by adjusting its inputs, and the verdict above never needed it.
 *
 * ℹ️ What is tight in the product is the FIGURE, not the unit — `1095 d` is
 * 43 px in that same 44 px box (PRODUCT tree, Plex served) and 45.47 px on the
 * fallback face, wider than the box. That is a fact about this track rather than
 * a second reason for the exclusion, which is §9.1's alone.
 *
 * ⚠️ AND THIS IS THE THIRD ARTIFACT THE 43 px REACHED, after
 * `docs/design/mockups/usarr.css` and DESIGN-DIRECTION §9.1, both since
 * corrected. A width that travels between trees with its surface left off is how
 * one column's number comes to be quoted for another's, which is why every
 * figure above names the tree it was measured in.
 */
export function formatAge(days: number | undefined): string {
	if (days === undefined || !Number.isFinite(days) || days < 0) return '';
	if (days < 1) return `${Math.max(1, Math.round(days * 24))} h`;
	return `${Math.round(days)} d`;
}

/*
 * `protocolClass` is deliberately gone, along with `.protocol-torrent` and
 * `.protocol-usenet` in app.css.
 *
 * It handed out a class name for a rule that had already been emptied of
 * everything except `color: var(--fg-muted)`: the two colour tokens behind it
 * were WITHDRAWN by DESIGN-DIRECTION §3.3, which makes the protocol swatch
 * achromatic because a torrent green one column from a status green is the one
 * collision this ramp cannot afford. The shipping rendering is `.proto` with the
 * words `torrent` and `usenet` carrying the distinction, and a function handing
 * out a class that styles nothing is a trap for whoever reads it next.
 */
