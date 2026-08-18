/**
 * The two things a scroll container has to do that the DOCUMENT used to do for
 * free, and stopped doing when the shell became a `100dvh` grid whose second
 * row scrolls (app.css §2.2): respond to the scroll keys, and hold a position
 * across a back-navigation.
 *
 * Both are browser defaults that were lost, not features being invented, and
 * both are restored here rather than by moving focus. FOCUS IS NOT A SCROLLING
 * MECHANISM: the skip link is the first tab stop on every screen (SC 2.4.1,
 * Level A) and auto-focusing the content region to make PageDown work would buy
 * a scroll by spending the one promise the shell makes about focus order.
 *
 * ── 1. THE KEYS ─────────────────────────────────────────────────────────────
 *
 * The browser hands a scroll key to the nearest scrollable ancestor OF THE
 * FOCUSED ELEMENT. On the very first paint focus is on `<body>`, and `<body>`
 * no longer has a scrollable ancestor, so PageDown and the arrows did nothing
 * at all until the first click, Tab or navigation moved focus into the region.
 * `scrollDeltaFor` supplies the missing ancestor by computing what the platform
 * WOULD have scrolled, and the caller applies it to the region.
 *
 * It is deliberately the narrowest handler that closes the gap:
 *
 *   · it runs only while focus is on the body, so an input, a button, a link
 *     or a scroller of its own is never second-guessed — forwarding a key that
 *     something else would have handled is a worse bug than the one being
 *     fixed, and by construction there is nothing else to handle it here;
 *   · Ctrl, Alt and Meta combinations are passed straight through, so no
 *     browser shortcut is swallowed;
 *   · it stops mattering the moment the problem stops existing, because after
 *     the first navigation focus is on the h1 inside the region and the browser
 *     is doing this itself again.
 *
 * THE AMOUNTS ARE MEASURED, NOT ASSUMED, and `pnpm probe:shell` re-measures
 * them against a document that really does scroll, in the same engine, every
 * time it runs. Chromium 141, viewport heights 240 / 300 / 400 / 500 / 600 /
 * 800: an arrow key is 40px on both axes at every height, and a page key is
 * `trunc(0.875 × clientHeight)` — 210, 262, 350, 437, 525, 700 — which is the
 * fraction and not `height - 40`, since the two disagree at every height
 * measured and only the fraction matches. A nested `overflow: auto` box at
 * clientHeight 500 stepped by the same 437, so the rule is the scrollport's and
 * not the viewport's.
 *
 * One deliberate divergence, in animation only: Chromium animates a keyboard
 * scroll in the compositor, and this scrolls instantly. The direction and the
 * amount are the platform's; the motion is not, because the shell declares
 * `scroll-behavior: auto` and the project does not ship motion that buys
 * nothing (DESIGN-DIRECTION: no flair that costs render time).
 *
 * ── 2. THE POSITION ─────────────────────────────────────────────────────────
 *
 * SvelteKit remembers a scroll position per history entry and restores it by
 * calling `scrollTo` on the WINDOW. The window no longer moves, so every
 * back-navigation landed silently at the top — measured at 1280x400: /services
 * scrolled to 500, forward to /requests, back, restored offset 0.
 *
 * `createScrollMemory` keeps the same record for the region that does scroll,
 * keyed the same way SvelteKit keys its own — by the history index it stamps
 * on `history.state` — and persisted to sessionStorage so a reload or a
 * restored tab is not a special case. See the note at HISTORY_INDEX for what
 * happens if that key ever moves.
 */

/** One arrow key. 40px, measured on both axes at six viewport heights. */
const LINE_STEP = 40;

/**
 * One page key, as a fraction of the SCROLLPORT's height. Blink's own constant
 * is `kMinFractionToStepWhenPaging`; what matters here is that it is what the
 * engine actually did at every height probed, and that the result truncates.
 */
const PAGE_FRACTION = 0.875;

/** Everything `scrollDeltaFor` needs, so it is a pure function and testable. */
export interface RegionMetrics {
	scrollTop: number;
	scrollLeft: number;
	clientHeight: number;
	scrollHeight: number;
	scrollWidth: number;
	clientWidth: number;
}

/** Reads the six numbers off a live element. */
export function metricsOf(region: Element): RegionMetrics {
	return {
		scrollTop: region.scrollTop,
		scrollLeft: region.scrollLeft,
		clientHeight: region.clientHeight,
		scrollHeight: region.scrollHeight,
		scrollWidth: region.scrollWidth,
		clientWidth: region.clientWidth
	};
}

function pageStep(clientHeight: number): number {
	return Math.max(1, Math.trunc(clientHeight * PAGE_FRACTION));
}

/**
 * What the platform would have scrolled for this key press, as a delta, or
 * null if this key press is none of its business.
 *
 * Home and End come back as deltas too — the caller has exactly one way to
 * apply a result, so there is no second path to get wrong.
 */
export function scrollDeltaFor(
	event: Pick<
		KeyboardEvent,
		'key' | 'shiftKey' | 'ctrlKey' | 'altKey' | 'metaKey' | 'defaultPrevented'
	> & {
		isComposing?: boolean;
	},
	m: RegionMetrics
): { top: number; left: number } | null {
	// Something ahead of this already claimed the key.
	if (event.defaultPrevented) return null;
	// A modifier makes this a browser or OS shortcut, never a scroll.
	if (event.ctrlKey || event.altKey || event.metaKey) return null;
	// Mid-IME. The key is the composition's, whatever it looks like.
	if (event.isComposing) return null;
	// Shift is meaningful on exactly one scroll key and is a selection modifier
	// everywhere else, so Shift+anything-but-Space is left alone.
	if (event.shiftKey && event.key !== ' ' && event.key !== 'Spacebar') return null;

	const maxTop = Math.max(0, m.scrollHeight - m.clientHeight);
	const maxLeft = Math.max(0, m.scrollWidth - m.clientWidth);

	switch (event.key) {
		case 'ArrowDown':
			return { top: LINE_STEP, left: 0 };
		case 'ArrowUp':
			return { top: -LINE_STEP, left: 0 };
		case 'ArrowRight':
			return { top: 0, left: LINE_STEP };
		case 'ArrowLeft':
			return { top: 0, left: -LINE_STEP };
		case 'PageDown':
			return { top: pageStep(m.clientHeight), left: 0 };
		case 'PageUp':
			return { top: -pageStep(m.clientHeight), left: 0 };
		// `Spacebar` is the legacy name; harmless to accept and free to add.
		case ' ':
		case 'Spacebar':
			return {
				top: event.shiftKey ? -pageStep(m.clientHeight) : pageStep(m.clientHeight),
				left: 0
			};
		case 'Home':
			// `|| 0` normalises the negative zero that `-0` produces when the
			// region is already at the top; it is the same delta, spelled once.
			return { top: -m.scrollTop || 0, left: -m.scrollLeft || 0 };
		case 'End':
			return { top: maxTop - m.scrollTop, left: maxLeft - m.scrollLeft };
		default:
			return null;
	}
}

/**
 * True only while focus is still where the browser put it on first paint.
 * `null` and `<html>` are the same case as `<body>` — no element has taken
 * focus — and every other value means something real owns the keyboard.
 */
export function focusIsUnclaimed(doc: Document): boolean {
	const active = doc.activeElement;
	return active === null || active === doc.body || active === doc.documentElement;
}

/**
 * The whole keyboard half, for the shell's existing window keydown handler.
 * Returns true if the key was consumed, which is only ever the case when focus
 * is unclaimed AND the key is a scroll key AND the region can move that way.
 */
export function forwardScrollKey(event: KeyboardEvent, region: HTMLElement): boolean {
	if (!focusIsUnclaimed(region.ownerDocument)) return false;
	const m = metricsOf(region);
	const delta = scrollDeltaFor(event, m);
	if (delta === null) return false;
	// A key that cannot move this region is not this handler's key. Left and
	// Right on a region with no horizontal overflow are the ordinary case —
	// measured on /services at 1280x800, where `.main` has none — and claiming
	// them would swallow a key press to produce nothing.
	const top = clamp(
		delta.top,
		-m.scrollTop,
		Math.max(0, m.scrollHeight - m.clientHeight) - m.scrollTop
	);
	const left = clamp(
		delta.left,
		-m.scrollLeft,
		Math.max(0, m.scrollWidth - m.clientWidth) - m.scrollLeft
	);
	if (top === 0 && left === 0) return false;
	event.preventDefault();
	region.scrollBy({ top, left });
	return true;
}

function clamp(value: number, low: number, high: number): number {
	return Math.min(Math.max(value, low), high);
}

/* ────────────────────────────────────────────────────────────────────────── */

/**
 * ⚠️ SVELTEKIT'S OWN HISTORY KEY, read rather than re-derived.
 *
 * SvelteKit stamps a monotonic index on `history.state` under this key and uses
 * it as the key of its own scroll record (`sveltekit:scroll` in sessionStorage).
 * Keying the region's record the same way is what makes the two agree about
 * which entry is which, including across a replaceState navigation, where a
 * locally-counted index would drift.
 *
 * It is an internal constant, and the cost of it moving is bounded on purpose:
 * `readIndex` returns null, nothing is recorded, nothing is restored, and the
 * shell behaves exactly as it did before this module existed. `pnpm probe:shell`
 * measures the restored offset end to end, so the day it moves the probe says
 * so rather than the behaviour quietly degrading.
 */
const HISTORY_INDEX = 'sveltekit:history';

const STORE_KEY = 'usarr:shell-scroll';

/** How long to keep re-applying a restored position while content arrives. */
const RESTORE_WINDOW_MS = 2000;

function readIndex(): number | null {
	const state: unknown = history.state;
	if (typeof state !== 'object' || state === null) return null;
	const index = (state as Record<string, unknown>)[HISTORY_INDEX];
	return typeof index === 'number' ? index : null;
}

function load(): Record<number, number> {
	try {
		const raw = sessionStorage.getItem(STORE_KEY);
		if (!raw) return {};
		const parsed: unknown = JSON.parse(raw);
		return typeof parsed === 'object' && parsed !== null ? (parsed as Record<number, number>) : {};
	} catch {
		// Private-mode sessionStorage throws on read as well as on write. A
		// browser that will not remember is a browser that lands at the top, which
		// is what happens without this module at all.
		return {};
	}
}

export interface ScrollMemory {
	/** Record where the region is, against the entry currently being left. */
	remember(): void;
	/**
	 * Apply whatever this navigation should land on: the remembered offset for a
	 * back or forward step, and the top of the screen for anything else.
	 */
	settle(isPop: boolean): void;
	dispose(): void;
}

/**
 * Keeps the region's scroll position per history entry.
 *
 * `getRegion` and `getContent` are getters rather than elements because the
 * shell binds them after this is constructed. `getContent` is the box whose
 * height changes as a screen's data arrives, and it is what makes restoration
 * work at all on this application: every screen fetches in `onMount`, so at the
 * moment a back-navigation completes the region is still one screen tall and
 * `scrollTop = 500` clamps to 0. The position is re-applied as the content
 * grows, for a bounded window, and abandoned the instant the user scrolls
 * themselves — which is the same thing a browser does with its own restoration.
 */
export function createScrollMemory(
	getRegion: () => HTMLElement | null,
	getContent: () => HTMLElement | null
): ScrollMemory {
	const positions = load();
	// The entry being LEFT. It cannot be read from `history.state` at that
	// moment: a popstate has already swapped the state in by the time any
	// navigation callback runs, which is the same reason SvelteKit tracks its
	// own `previous_history_index`.
	let current = readIndex();

	let observer: ResizeObserver | null = null;
	let timer: ReturnType<typeof setTimeout> | null = null;
	let abandon: (() => void) | null = null;

	function stopRestoring() {
		observer?.disconnect();
		observer = null;
		if (timer !== null) clearTimeout(timer);
		timer = null;
		abandon?.();
		abandon = null;
	}

	function persist() {
		try {
			sessionStorage.setItem(STORE_KEY, JSON.stringify(positions));
		} catch {
			// Quota or private mode. The in-memory record still serves this tab.
		}
	}

	function apply(region: HTMLElement, target: number): boolean {
		region.scrollTop = target;
		return Math.abs(region.scrollTop - target) <= 1;
	}

	return {
		remember() {
			const region = getRegion();
			if (region === null || current === null) return;
			positions[current] = region.scrollTop;
			persist();
		},

		settle(isPop: boolean) {
			stopRestoring();
			const region = getRegion();
			current = readIndex();
			if (region === null) return;

			if (!isPop) {
				// A new screen starts at its top. The region persists across a route
				// change — only its contents are replaced — so without this it keeps
				// whatever offset the previous screen left behind.
				region.scrollTop = 0;
				return;
			}

			const target = current === null ? 0 : (positions[current] ?? 0);
			if (apply(region, target)) return;

			const content = getContent();
			if (content === null) return;

			// The screen's data has not arrived yet. Keep re-applying while the
			// content box grows, and give up the moment the user takes over.
			//
			// The key listener is on the WINDOW rather than on the region, because
			// the shell's own key forwarding scrolls the region from a keydown whose
			// target is `<body>` — that event never passes through the region, so a
			// region-bound listener would let the two fight over the offset.
			const giveUp = () => stopRestoring();
			region.addEventListener('wheel', giveUp, { passive: true });
			region.addEventListener('pointerdown', giveUp, { passive: true });
			window.addEventListener('keydown', giveUp, { passive: true });
			abandon = () => {
				region.removeEventListener('wheel', giveUp);
				region.removeEventListener('pointerdown', giveUp);
				window.removeEventListener('keydown', giveUp);
			};
			observer = new ResizeObserver(() => {
				if (apply(region, target)) stopRestoring();
			});
			observer.observe(content);
			timer = setTimeout(stopRestoring, RESTORE_WINDOW_MS);
		},

		dispose() {
			stopRestoring();
		}
	};
}
