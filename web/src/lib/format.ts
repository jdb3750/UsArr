/** Binary units, because that is what every indexer and every *Arr reports. */
export function formatSize(bytes: number | undefined): string {
	if (bytes === undefined || !Number.isFinite(bytes) || bytes < 0) return '';
	const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < units.length - 1) {
		value /= 1024;
		unit += 1;
	}
	const digits = unit === 0 || value >= 100 ? 0 : 1;
	return `${value.toFixed(digits)} ${units[unit]}`;
}

/**
 * Release age. The server sends `age_days` as a float (internal/httpapi/search.go),
 * so this is the one place that turns it into something a human reads.
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
