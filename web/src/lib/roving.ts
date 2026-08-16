/**
 * Roving tabindex for the list primitive: a list is ONE tab stop, arrow keys
 * move within it (ARIA APG, DESIGN-DIRECTION §11).
 *
 * This is a port of the mockup's proven implementation (docs/design/mockups/
 * usarr.js) with one behavioural change, which is the reason it is a separate
 * module rather than an inline effect:
 *
 *   THE TAB STOP IS KEYED TO A ROW IDENTITY, NEVER TO A POSITION. The mockup
 *   re-derives the roved row by scanning for whichever row currently holds
 *   `tabIndex === 0`, which is identity-stable only for as long as the DOM node
 *   survives. A list that reorders — a sort, a filter, an SSE update arriving
 *   mid-scroll — can legitimately replace nodes, and a positional fallback then
 *   silently teleports the keyboard user to a different row. On a screen whose
 *   row action is an irreversible grab that is the sharpest failure available,
 *   so the caller supplies `data-key` on every row and this module reads it.
 *
 * Four invariants, all asserted by scripts/list-bench.mjs:
 *   1. exactly one row at tabindex="0", at all times;
 *   2. every focusable descendant of a row at tabindex="-1", so the row is the
 *      only stop rather than an extra one on top of the links already in it;
 *   3. the assignment is idempotent and re-runs after every append;
 *   4. an arrow key walks `nextElementSibling`/`previousElementSibling` and
 *      never rescans the list — measured at 55.12 ms per keypress at 25,000
 *      rows for the rescan, which is 2.25 s of main thread for one second of
 *      key repeat.
 */

/** Anything inside a row that would otherwise be its own tab stop. */
const FOCUSABLE = 'a[href], button, input, select, textarea, [tabindex]';

/**
 * Which controls own their own arrow keys, and therefore where the handler must
 * bail out before the key switch. A text input owns Left/Right and Home/End for
 * its caret, a `<select>` owns Up/Down for its value, and a radio owns Up/Down
 * for its group — SC 2.1.1, Level A.
 *
 * A checkbox owns none of them in any browser, and including it made the
 * bail-out too broad in exactly the place it hurts most: where a row's first
 * control is a row-select checkbox, arrowing into one left no way to arrow
 * onward and no Escape to get back out — only Tab. Push buttons are excluded
 * for the same reason.
 */
const FORM_TARGET =
	'select, textarea, [contenteditable], ' +
	'input:not([type=checkbox]):not([type=button]):not([type=submit])';

export interface RovingParams {
	/**
	 * The identity of the row that should hold the single tab stop, or null to
	 * let the first row have it. Read from `data-key`.
	 */
	activeKey?: string | null;
	/** Called with the new `data-key` whenever the tab stop moves. */
	onmove?: (key: string) => void;
	/**
	 * Anything that changes when the rendered rows change. Svelte re-runs an
	 * action's `update` when its parameter object changes, so passing the row
	 * count (or any per-render value) here is what makes invariant 3 hold
	 * across a "Load more" append.
	 */
	revision?: number;
	/**
	 * Whether `j`/`k` are live. They are single-character shortcuts and SC 2.1.4
	 * is Level A; the Settings toggle is what satisfies "turn off" for all of
	 * them at once. Arrow keys are unaffected — they are not letter keys.
	 */
	letterKeys?: boolean;
	/** Called on Enter or Space with the row's `data-key`. */
	onactivate?: (key: string) => void;
	/**
	 * False for a list that declares `data-roving-optout="reason"`. The action
	 * is still attached — `use:` cannot be applied conditionally — so the flag
	 * is what makes it inert, and the list is then a normal sequence of tab
	 * stops. Opting out is a decision that gets written down in the attribute,
	 * which is what check 7 in docs/design/check.mjs reads.
	 */
	enabled?: boolean;
}

/** A row is visible if neither it nor an ancestor carries `hidden`. */
function visible(el: Element): boolean {
	return !(el as HTMLElement).hidden && el.closest('[hidden]') === null;
}

function isFormTarget(el: EventTarget | null): boolean {
	return el instanceof Element && el.closest(FORM_TARGET) !== null;
}

/**
 * Set `tabindex` only when it differs. The read is free (neither getAttribute
 * nor tabIndex forces layout), the write is not: an unconditional assignment on
 * every row of every append is O(loaded rows) of attribute churn, and appending
 * is the primary interaction.
 *
 * IT COMPARES THE ATTRIBUTE, NOT THE `tabIndex` PROPERTY, and that is a bug fix
 * rather than a style choice. `el.tabIndex` returns -1 for an element that has
 * no `tabindex` attribute and is not focusable by default — which is every
 * `<tr>` in a fresh list. Comparing against the property therefore concluded
 * "already -1, nothing to do" and never wrote the attribute at all, leaving the
 * rows unfocusable: `row.focus()` silently did nothing, so Escape from inside a
 * row's controls had nowhere to go. Caught by the Escape assertion in
 * scripts/list-bench.mjs, which is exactly the kind of thing that survives a
 * reading of the code and does not survive a measurement.
 */
function setTab(el: HTMLElement, value: number): void {
	const want = String(value);
	if (el.getAttribute('tabindex') !== want) el.setAttribute('tabindex', want);
}

export function roving(container: HTMLElement, initial: RovingParams = {}) {
	let params: RovingParams = initial;
	const ROWS = '[role="row"][data-key]';

	function allRows(): HTMLElement[] {
		return Array.from(container.querySelectorAll<HTMLElement>(ROWS)).filter(visible);
	}

	/**
	 * Idempotent by construction: it computes the row that should hold the stop
	 * and then writes only the attributes that are wrong. Running it twice in a
	 * row does nothing the second time.
	 */
	function assign(): void {
		if (params.enabled === false) return;
		const rows = allRows();
		if (rows.length === 0) return;

		// Identity first. A positional fallback is what this module exists to
		// avoid, so the scan for an existing tabindex=0 row is a SECOND choice
		// and the first row is only the third.
		let chosen: HTMLElement | null = null;
		if (params.activeKey != null) {
			chosen = rows.find((r) => r.dataset.key === params.activeKey) ?? null;
		}
		if (!chosen) chosen = rows.find((r) => r.tabIndex === 0) ?? null;
		if (!chosen) chosen = rows[0];

		for (const row of rows) {
			setTab(row, row === chosen ? 0 : -1);
			// Invariant 2. Without this the row is an EXTRA tab stop on top of
			// the links already inside it, so a seven-row list is eight stops
			// rather than one, and arrowing from a focused link jumps to the
			// next row rather than to the next control.
			for (const inner of row.querySelectorAll<HTMLElement>(FOCUSABLE)) setTab(inner, -1);
		}
	}

	/** The adjacent row, by sibling walk. Never a rescan. See invariant 4. */
	function step(from: HTMLElement, dir: 1 | -1): HTMLElement | null {
		let el: Element | null = from;
		while (el) {
			el = dir === 1 ? el.nextElementSibling : el.previousElementSibling;
			if (el && el.matches(ROWS) && visible(el)) return el as HTMLElement;
		}
		return null;
	}

	function move(from: HTMLElement, to: HTMLElement | null): void {
		if (!to || to === from) return;
		setTab(from, -1);
		setTab(to, 0);
		to.focus();
		const key = to.dataset.key;
		if (key !== undefined) params.onmove?.(key);
	}

	function onKeydown(event: KeyboardEvent): void {
		if (params.enabled === false) return;
		const target = event.target;
		if (!(target instanceof Element)) return;
		const row = target.closest<HTMLElement>(ROWS);
		if (!row || !container.contains(row)) return;

		// ESCAPE IS HANDLED BEFORE THE FORM-CONTROL BAIL-OUT, and that order is
		// the whole point. The bail-out below is correct for arrows, Home and
		// End — a caret owns those — and it also swallows Escape, which removes
		// the documented way out of a row's controls exactly where it is needed.
		if (event.key === 'Escape') {
			if (target !== row) {
				event.preventDefault();
				row.focus();
			}
			return;
		}

		// A caret in a text field owns its own arrow keys, Home and End.
		if (isFormTarget(target)) return;

		switch (event.key) {
			case 'ArrowDown':
				event.preventDefault();
				move(row, step(row, 1));
				return;
			case 'ArrowUp':
				event.preventDefault();
				move(row, step(row, -1));
				return;
			case 'j':
			case 'k':
				// Letter keys only from the row itself, and only while the
				// Settings toggle has them on.
				if (!params.letterKeys || target !== row) return;
				event.preventDefault();
				move(row, step(row, event.key === 'j' ? 1 : -1));
				return;
			case 'ArrowRight':
			case 'ArrowLeft': {
				// Left/Right walks the row's own controls, which is what makes
				// "the row is one tab stop" compatible with the row having
				// actions in it at all.
				const controls = Array.from(row.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(visible);
				if (controls.length === 0) return;
				const at = controls.indexOf(target as HTMLElement);
				const to = event.key === 'ArrowRight' ? at + 1 : at - 1;
				event.preventDefault();
				if (to < 0) {
					row.focus();
					return;
				}
				if (to >= controls.length) return;
				controls[to].focus();
				return;
			}
			case 'Home':
			case 'End': {
				// Only from the row itself: Home inside a caret means column zero.
				if (target !== row) return;
				// The full scan survives here and only here. Home and End do not
				// repeat, so O(rows) once is not the cost an arrow key is.
				const rows = allRows();
				if (rows.length === 0) return;
				event.preventDefault();
				move(row, event.key === 'Home' ? rows[0] : rows[rows.length - 1]);
				return;
			}
			case 'Enter':
			case ' ': {
				if (target !== row) return;
				const key = row.dataset.key;
				if (key === undefined) return;
				event.preventDefault();
				params.onactivate?.(key);
				return;
			}
			default:
		}
	}

	/**
	 * Clicking or tabbing into a row makes it the tab stop, so Tab out and back
	 * returns to where the user was rather than to row one.
	 */
	function onFocusin(event: FocusEvent): void {
		if (params.enabled === false) return;
		const target = event.target;
		if (!(target instanceof Element)) return;
		const row = target.closest<HTMLElement>(ROWS);
		if (!row) return;
		const key = row.dataset.key;
		if (key === undefined || key === params.activeKey) return;
		params.onmove?.(key);
	}

	container.addEventListener('keydown', onKeydown);
	container.addEventListener('focusin', onFocusin);

	// Belt and braces for invariant 3. `update` below already re-runs the
	// assignment whenever the caller's parameters change, which covers every
	// append Svelte drives; the observer covers an append that anything else
	// drives, which is the case the mockup found the hard way (a row cloned by
	// a state switcher inherited tabindex="0" and became a second tab stop).
	// Coalesced to one run per microtask so a 200-row append is one assignment.
	let queued = false;
	const observer = new MutationObserver((records) => {
		if (queued) return;
		for (const record of records) {
			if (record.addedNodes.length > 0) {
				queued = true;
				queueMicrotask(() => {
					queued = false;
					assign();
				});
				return;
			}
		}
	});
	observer.observe(container, { childList: true, subtree: true });

	assign();

	return {
		update(next: RovingParams) {
			params = next;
			assign();
		},
		destroy() {
			observer.disconnect();
			container.removeEventListener('keydown', onKeydown);
			container.removeEventListener('focusin', onFocusin);
		}
	};
}
