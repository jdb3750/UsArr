/**
 * THE CATALOGUE SCREENS' VIEW TOGGLE, HELD ACROSS RELOADS AND PER MEDIA TYPE.
 *
 * WHY THE CHOICE IS DEVICE-LOCAL. It describes how this browser draws one
 * screen, not anything about the library, so it belongs in localStorage rather
 * than in a round trip — and reading it from the API would put a network call on
 * the render path, which Principle 1 forbids. That is `$lib/prefs.svelte`'s
 * argument for theme and density, and this is the same kind of value.
 *
 * THE IDIOM IS `$lib/homeview.svelte`'s, deliberately: a key and a parser in a
 * plain module next door, a factory rather than a module singleton, and
 * `read`/`write` behind try/catch because localStorage THROWS rather than
 * returning null in a browser with site data blocked.
 *
 * ⚠️ AND IT IS NOT A COPY OF THAT FILE'S STATE, WHICH IS THE ONE DIFFERENCE
 * WORTH READING BEFORE EDITING. Home holds one value under one key, because Home
 * is the all-types view and has no type to key on. These screens hold one value
 * PER KEY, and the key moves under the component: SvelteKit reuses
 * `routes/library/[type]/+page.svelte` across `/library/movies` →
 * `/library/tv`, so a value captured once at construction would follow the
 * reader from one media type to the next and then be written back under the
 * wrong one. The key is therefore a getter, read on every access.
 *
 * WHY THE KEY AND THE PARSER LIVE IN `$lib/librarygrid` AND NOT HERE. This file
 * is a `.svelte.ts` and carries runes, so `vitest.config.ts` — `environment:
 * 'node'`, no Svelte plugin — cannot import it at all. Everything a test needs
 * to hold is next door in a plain module, and what is left here is the reactive
 * holder and the two storage calls.
 */
import { browser } from '$app/environment';
import { parseLibraryView, type LibraryView } from '$lib/librarygrid';

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
 * The view a catalogue screen is drawing, and the setter that remembers it.
 *
 * A FACTORY RATHER THAN A MODULE SINGLETON, which is `$lib/homeview.svelte`'s
 * shape and `$lib/indexerscope.svelte`'s before it, and not `$lib/prefs`'s. The
 * difference is scope: theme and density are chrome the whole application reads,
 * so one instance is correct for them; this belongs to one screen, and a
 * singleton would outlive the component that owns it for no benefit.
 *
 * `key` IS A GETTER AND NOT A STRING. See the header: the per-type screen's key
 * changes without the component remounting.
 */
export function createLibraryView(key: () => string) {
	// The choice made in THIS page view, and the key it was made under. Storage
	// answers for every other key, so a reader who switches media type sees what
	// they last chose there rather than what they chose here.
	let chosenKey = $state<string | undefined>(undefined);
	let chosen = $state<LibraryView | undefined>(undefined);

	return {
		get current(): LibraryView {
			const k = key();
			if (k === chosenKey && chosen !== undefined) return chosen;
			return parseLibraryView(read(k));
		},

		set(value: LibraryView): void {
			const k = key();
			chosenKey = k;
			chosen = value;
			write(k, value);
		}
	};
}
