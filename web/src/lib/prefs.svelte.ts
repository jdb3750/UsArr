/**
 * The three chrome preferences: theme, density and single-key shortcuts.
 *
 * All three are device-local rather than account-level. They describe how this
 * browser draws the application, not anything about the library, so they belong
 * in localStorage and not in a round trip to the API — and reading them from
 * the server would put a network call on the render path, which is the one
 * thing Principle 1 forbids.
 *
 * THE STORAGE KEYS ARE PART OF THE CONTRACT. `usarr.theme`, `usarr.density` and
 * `usarr.shortcuts` are what a browser that has already been used will have
 * written. Renaming one silently resets every existing install to the default,
 * so they are frozen.
 *
 * WHY THERE IS NO ANTI-FLASH INLINE SCRIPT. The obvious way to apply a stored
 * theme before first paint is a small synchronous <script> in app.html. The
 * server sends `script-src 'self'` with no 'unsafe-inline'
 * (internal/httpapi/middleware.go), and measured in Chromium against that exact
 * header an inline script is refused outright. So the attributes are written
 * from this module's top-level side effect instead, which runs when the root
 * layout imports it — before Svelte has rendered anything and, because this is
 * an SPA whose served document has an empty body, before there is anything to
 * paint. The cost is at worst one frame of the default theme, and in practice
 * none.
 *
 * READ AND WRITE BOTH GO THROUGH try/catch. localStorage throws rather than
 * returning null in a browser with site data blocked, and a preference is never
 * worth taking the application down for.
 */
import { browser } from '$app/environment';

export type Theme = 'light' | 'dark' | 'auto';
export type Density = 'compact' | 'standard' | 'relaxed';

export const THEME_KEY = 'usarr.theme';
export const DENSITY_KEY = 'usarr.density';
export const SHORTCUTS_KEY = 'usarr.shortcuts';

/** Auto follows the OS, which is Sonarr's shape and the documented default. */
const THEME_DEFAULT: Theme = 'auto';
/** Compact is what loads. Density is a feature, so the dense option is not the
 * middle one (ARCHITECTURE §17.1). */
const DENSITY_DEFAULT: Density = 'compact';
/** On by default, and the Settings toggle is what satisfies SC 2.1.4 for all
 * of the single-character shortcuts at once. */
const SHORTCUTS_DEFAULT = true;

const THEMES: readonly Theme[] = ['light', 'dark', 'auto'];
const DENSITIES: readonly Density[] = ['compact', 'standard', 'relaxed'];

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
		// Site data is blocked. The preference applies for this page view and is
		// not remembered; nothing else about the application changes.
	}
}

function readTheme(): Theme {
	const stored = read(THEME_KEY);
	return THEMES.includes(stored as Theme) ? (stored as Theme) : THEME_DEFAULT;
}

function readDensity(): Density {
	const stored = read(DENSITY_KEY);
	return DENSITIES.includes(stored as Density) ? (stored as Density) : DENSITY_DEFAULT;
}

function readShortcuts(): boolean {
	const stored = read(SHORTCUTS_KEY);
	if (stored === 'on') return true;
	if (stored === 'off') return false;
	return SHORTCUTS_DEFAULT;
}

/**
 * `auto` is the ABSENCE of the attribute, not a third value of it. app.css
 * resolves the three states as: bare :root is light, :root[data-theme="dark"]
 * is dark, and `@media (prefers-color-scheme: dark) :root:not([data-theme=
 * "light"])` is what makes auto follow the OS. Writing data-theme="auto" would
 * match neither override and pin the app to light.
 */
function applyTheme(value: Theme): void {
	if (!browser) return;
	const root = document.documentElement;
	if (value === 'auto') root.removeAttribute('data-theme');
	else root.setAttribute('data-theme', value);
}

function applyDensity(value: Density): void {
	if (!browser) return;
	document.documentElement.setAttribute('data-density', value);
}

/**
 * The stored values are read into plain consts FIRST, and the import-time apply
 * below consumes those rather than the `$state` variables. Both spellings put
 * the same string on `document.documentElement` — this bootstrap is a one-shot
 * that runs synchronously on the line after the state is created, and every
 * later change routes through setTheme/setDensity, which apply the new value
 * themselves. But reading a `$state` variable at module scope captures it by
 * value, so the compiler cannot tell a deliberate snapshot from the bug where
 * someone expects the call to re-run; it warns `state_referenced_locally` on
 * both. Taking the value from the const says which one this is, in the code
 * rather than in a suppression comment.
 */
const initialTheme = readTheme();
const initialDensity = readDensity();

let theme = $state<Theme>(initialTheme);
let density = $state<Density>(initialDensity);
let shortcuts = $state<boolean>(readShortcuts());

// Applied at import time, before the root layout renders. See the header.
applyTheme(initialTheme);
applyDensity(initialDensity);

export const prefs = {
	get theme(): Theme {
		return theme;
	},

	setTheme(value: Theme): void {
		if (!THEMES.includes(value)) return;
		theme = value;
		applyTheme(value);
		write(THEME_KEY, value);
	},

	get density(): Density {
		return density;
	},

	setDensity(value: Density): void {
		if (!DENSITIES.includes(value)) return;
		density = value;
		applyDensity(value);
		write(DENSITY_KEY, value);
	},

	/**
	 * Whether the single-character shortcuts (`/`, `j`, `k`, `l`, and the rest)
	 * are live. SC 2.1.4 is Level A and requires that a letter-key shortcut can
	 * be turned off, remapped, or confined to focus; this one control turns all
	 * of them off at once, which is the cheapest of the three routes.
	 *
	 * `?` is deliberately NOT governed by this, because the sheet it opens is
	 * where the toggle is discovered.
	 */
	get shortcuts(): boolean {
		return shortcuts;
	},

	setShortcuts(value: boolean): void {
		shortcuts = value;
		write(SHORTCUTS_KEY, value ? 'on' : 'off');
	}
};
