/**
 * Harness entry point for the freeze-while-aimed negative control.
 *
 * Everything the driver pokes hangs off `window.__freeze`. The two things worth
 * reading here:
 *
 *   · `inFlush` brackets `flushSync()`, so the diagnostic observers in
 *     Region.svelte can report whether the browser dispatched a boundary event
 *     from inside Svelte's flush or after it. That distinction is the entire
 *     mechanism of the bug this control covers, and a run where nothing fired
 *     during the flush is a run that proved nothing — so it is reported rather
 *     than inferred.
 *
 *   · `pageErrors` collects uncaught errors. `state_unsafe_mutation` surfaces as
 *     one, and the assignment that raised it is LOST. The assertions do not key
 *     on this — an aim flag can also be lost without a throw, and a test that
 *     asserts on the error message is asserting on Svelte's wording — but the
 *     count is printed so the failure is diagnosable rather than merely red.
 */
import '../../src/app.css';
import { mount, flushSync } from 'svelte';
import Region from './Region.svelte';
import { feed } from './state.svelte';

declare global {
	interface Window {
		__freeze: {
			feed: typeof feed;
			handle: ReturnType<typeof mountRegion>;
			/** True for the duration of a driver-initiated flush. Read by the
			 * diagnostic observers only. */
			inFlush: boolean;
			pageErrors: string[];
			/** Run `fn`, then apply its state changes synchronously, so an arrival
			 * lands in one flush exactly as an SSE leg's does. */
			flush: (fn?: () => void) => void;
			/** Rendered order, by key, straight off the DOM rather than off the
			 * engine's own array — the assertion is about what a person can point
			 * at, not about what the module believes. */
			renderedKeys: () => string[];
			/** The one place the row selector is written down. `$lib/List.svelte`
			 * owns the row markup, so the driver asks the harness rather than
			 * carrying its own copy of it. */
			rowSelector: string;
		};
	}
}

function mountRegion() {
	const instance = mount(Region, { target: document.getElementById('freeze-harness')! });
	return instance.handle();
}

/** A rendered row, as `$lib/List.svelte` emits it. The expand row it can also
 * emit carries no `data-key`, which is exactly what keeps it out of here. */
const ROW = '[data-testid="region"] tr[role="row"][data-key]';

const pageErrors: string[] = [];
window.addEventListener('error', (event) => {
	pageErrors.push(String(event.message ?? event.error));
});
window.addEventListener('unhandledrejection', (event) => {
	pageErrors.push(String(event.reason));
});

const handle = mountRegion();

window.__freeze = {
	feed,
	handle,
	inFlush: false,
	pageErrors,
	flush(fn?: () => void) {
		window.__freeze.inFlush = true;
		try {
			if (fn) fn();
			flushSync();
		} finally {
			window.__freeze.inFlush = false;
		}
	},
	renderedKeys() {
		return [...document.querySelectorAll(ROW)].map((el) => (el as HTMLElement).dataset.key ?? '');
	},
	rowSelector: ROW
};
