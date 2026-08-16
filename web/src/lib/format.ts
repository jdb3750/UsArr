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

/** Protocol drives a colour token, so it has to be a known value or nothing. */
export function protocolClass(protocol: string | undefined): string {
	if (protocol === 'torrent') return 'protocol-torrent';
	if (protocol === 'usenet') return 'protocol-usenet';
	return '';
}
