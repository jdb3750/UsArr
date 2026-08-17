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
 * job. §9.1 applies the split-slot treatment to SIZE COLUMNS ONLY, and names Age
 * as still carrying the one-string treatment on purpose: the reserved box costs
 * column width and `Age` in the release tables is a declared 68 px track — 24 px
 * of padding leaves 44 px, and the widest unit alone is 43 px, so the figure has
 * nothing left and the cell wraps. Measured at +18 px per cell over 224 cells.
 * Widening a declared track is its own decision on the densest tables in the
 * product, so there is deliberately no pair-returning function here to reach
 * for. Publishing one would make the exclusion a matter of discipline; not
 * publishing one makes it structural.
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
