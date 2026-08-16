/**
 * A stand-in for SvelteKit's `$app/environment`, which does not exist outside a
 * SvelteKit build. `src/lib/prefs.svelte.ts` is the only thing under test that
 * imports it, and it uses `browser` to guard localStorage and the document.
 *
 * The harness always runs in a browser, so this is `true` — not a stub that
 * changes behaviour, which would make the measurement describe a different
 * component from the one that ships.
 */
export const browser = true;
export const dev = true;
export const building = false;
export const version = 'harness';
