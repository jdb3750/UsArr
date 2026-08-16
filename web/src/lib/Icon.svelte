<script lang="ts">
	/**
	 * The icon vocabulary, and it is deliberately a closed set.
	 *
	 * DESIGN-DIRECTION §13 keeps an allowlist of icon ids and treats a new one as
	 * a PR discussion rather than a free choice, so the names here are exactly the
	 * ones the mockups' sprite carries — no more, and no emoji anywhere.
	 *
	 * Inline `<svg>` rather than a `<use href="#i-…">` sprite, for one reason
	 * worth writing down: a sprite needs the symbol document to exist somewhere in
	 * the DOM before the first reference renders, which on an SPA means either an
	 * inline block in `app.html` (nothing in the served body, by design) or a
	 * fetch. Six paths repeated a handful of times is cheaper than either, and it
	 * has no ordering hazard at all.
	 *
	 * EVERY ICON IS `aria-hidden`. §9.5 requires icon + text + colour in that
	 * order of importance and §11 forbids a status glyph with an empty accessible
	 * name, so the word is always beside it and the glyph is decoration. This
	 * component therefore never takes a label — a caller that wants one is
	 * building something this component is the wrong shape for.
	 */
	type Name = 'check' | 'alert' | 'x-circle' | 'dash-circle' | 'chevron-right' | 'chevron-down';

	interface Props {
		name: Name;
		/** `sm` is the 14px form used inside a row; the default is the 16px one. */
		size?: 'sm' | 'md';
	}

	let { name, size = 'md' }: Props = $props();

	/** Path data, lifted verbatim from docs/design/mockups/services.html's sprite. */
	const PATHS: Record<Name, { d: string; extra?: string; width?: string }> = {
		check: { d: 'M5 12.5l5 5L20 7', width: '2.25' },
		alert: {
			d: 'M10.3 4 2.6 17.4A2 2 0 0 0 4.3 20.4h15.4a2 2 0 0 0 1.7-3L13.7 4a2 2 0 0 0-3.4 0z',
			extra: 'M12 9v4.5M12 17.2h.01'
		},
		'x-circle': { d: 'M15 9l-6 6M9 9l6 6' },
		'dash-circle': { d: 'M8.5 12h7' },
		'chevron-right': { d: 'M9 6l6 6-6 6' },
		'chevron-down': { d: 'M6 9l6 6 6-6' }
	};

	const circled = $derived(name === 'x-circle' || name === 'dash-circle');
	const glyph = $derived(PATHS[name]);
</script>

<svg
	class="i"
	class:i--sm={size === 'sm'}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width={glyph.width ?? '2'}
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
	focusable="false"
>
	{#if circled}<circle cx="12" cy="12" r="9" />{/if}
	<path d={glyph.d} />
	{#if glyph.extra}<path d={glyph.extra} />{/if}
</svg>
