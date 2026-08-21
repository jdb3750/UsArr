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
 * WHY THE KEY, THE PARSER AND ⚠️ THE DECISION ITSELF LIVE IN `$lib/librarygrid`
 * AND NOT HERE. This file is a `.svelte.ts` and carries runes, so
 * `vitest.config.ts` — `environment: 'node'`, no Svelte plugin — cannot import
 * it at all, and anything held in here is held by nothing. That was not a
 * hypothetical: the getter rule above was stated as a MUST in this very comment
 * and enforced by no test, so replacing it with a closure over a captured value
 * left the whole gate green. `readLibraryView` and `chooseLibraryView` are next
 * door and are unit-tested there; what is left here is the reactive holder and
 * the two `window.localStorage` calls, which is the whole of what a rune file
 * has to own.
 */
import { browser } from '$app/environment';
import {
	chooseLibraryView,
	readLibraryView,
	type LibraryView,
	type LibraryViewChoice,
	type LibraryViewStorage
} from '$lib/librarygrid';

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

/** The two calls, behind the port the rules next door are written against. */
const STORAGE: LibraryViewStorage = { read, write };

/**
 * The view a catalogue screen is drawing, and the setter that remembers it.
 *
 * A FACTORY RATHER THAN A MODULE SINGLETON, which is `$lib/homeview.svelte`'s
 * shape and `$lib/indexerscope.svelte`'s before it, and not `$lib/prefs`'s. The
 * difference is scope: theme and density are chrome the whole application reads,
 * so one instance is correct for them; this belongs to one screen, and a
 * singleton would outlive the component that owns it for no benefit.
 *
 * ⚠️ `key` IS A GETTER AND NOT A STRING, AND IT IS PASSED ON RATHER THAN
 * RESOLVED HERE. See the header: the per-type screen's key changes without the
 * component remounting, so resolving it once in this factory is the same defect
 * as a caller passing `() => capturedKey`. Both rules are `readLibraryView`'s
 * and `chooseLibraryView`'s, where a test can reach them.
 */
export function createLibraryView(key: () => string) {
	// The choice made in THIS page view, and the key it was made under, as one
	// `$state` object so the pair cannot be updated apart.
	const chosen = $state<LibraryViewChoice>({ key: undefined, view: undefined });

	return {
		get current(): LibraryView {
			return readLibraryView(key, chosen, STORAGE);
		},

		set(value: LibraryView): void {
			chooseLibraryView(value, key, chosen, STORAGE);
		}
	};
}
