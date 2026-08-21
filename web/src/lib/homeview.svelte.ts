/**
 * BLOCK C'S VIEW TOGGLE, HELD ACROSS RELOADS.
 *
 * WHY THE CHOICE IS DEVICE-LOCAL. It describes how this browser draws one
 * region, not anything about the library, so it belongs in localStorage rather
 * than in a round trip — and reading it from the API would put a network call
 * on the render path, which Principle 1 forbids. That is `$lib/prefs.svelte`'s
 * argument for theme and density, and this is the same kind of value.
 *
 * THE IDIOM IS THE ONE ALREADY IN THE TREE, deliberately: a frozen key, a
 * parser that falls back rather than throwing, and `read`/`write` behind
 * try/catch because localStorage THROWS rather than returning null in a
 * browser with site data blocked. `$lib/prefs.svelte` and
 * `$lib/indexerscope.svelte` each carry their own copy of those two functions;
 * this is the third, and a fourth spelling of them would be the drift worth
 * avoiding, not the duplication.
 *
 * WHY THE KEY AND THE PARSER LIVE IN `$lib/home` AND NOT HERE. This file is a
 * `.svelte.ts` and carries a rune, so `vitest.config.ts` — `environment:
 * 'node'`, no Svelte plugin — cannot import it at all. Everything a test needs
 * to hold is therefore next door in a plain module, and what is left here is
 * the reactive holder and the two storage calls.
 */
import { browser } from '$app/environment';
import { HOME_VIEW_KEY, parseHomeView, type HomeView } from '$lib/home';

function read(key: string): string | null {
	if (!browser) return null;
	try {
		return window.localStorage.getItem(key);
	} catch {
		return null;
	}
}

function write(key: string, value: string): void {
	if (!browser) return;
	try {
		window.localStorage.setItem(key, value);
	} catch {
		// Site data is blocked. The choice applies for this page view and is not
		// remembered; nothing else about the screen changes.
	}
}

/**
 * The view Block C is drawing, and the setter that remembers it.
 *
 * A FACTORY RATHER THAN A MODULE SINGLETON, which is `$lib/indexerscope`'s
 * shape and not `$lib/prefs`'s. The difference is scope: theme and density are
 * chrome the whole application reads, so one instance is correct for them;
 * this belongs to one region of one screen, and a singleton would outlive the
 * component that owns it for no benefit.
 */
export function createHomeView() {
	let view = $state<HomeView>(parseHomeView(read(HOME_VIEW_KEY)));

	return {
		get current(): HomeView {
			return view;
		},

		set(value: HomeView): void {
			view = value;
			write(HOME_VIEW_KEY, value);
		}
	};
}
